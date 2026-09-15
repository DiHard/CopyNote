# Shared by the scripts in this folder: the names the test build runs under,
# where it keeps its files, and helpers to start, inspect and stop it.

$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
# Fixed, under %TEMP%: Reset-TestData and trace.ps1 delete folders in here.
$WorkDir = Join-Path ([IO.Path]::GetTempPath()) 'copynote-testinstance'
$Exe = Join-Path $WorkDir 'bin\copynote-test.exe'
$CdpPort = 9223

# The four machine-wide names a second CopyNote would otherwise share with the
# daily one. CLAUDE.md, "Running a test instance beside the real app".
$Names = [ordered]@{
  'main.singletonName'                         = 'Local\dev.copynote.test.singleton'
  'copynote/internal/tray.trayClassName'       = 'CopyNoteTestTrayWnd'
  'copynote/internal/tray.showMessageName'     = 'dev.copynote.test.SHOW'
  'copynote/internal/service.autorunValueName' = 'CopyNoteTest'
}
$TrayClass = $Names['copynote/internal/tray.trayClassName']
$RunValue = $Names['copynote/internal/service.autorunValueName']
$RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'

$MOD_ALT = 1
$MOD_CONTROL = 2
$VK_N = 0x4E
$VK_M = 0x4D
$WM_QUIT = 0x0012
$WM_HOTKEY = 0x0312
$WM_LBUTTONUP = 0x0202
$TRAY_CALLBACK = 0x8001 # WM_APP + 1, trayCallbackMsg in internal/tray
$ERROR_HOTKEY_ALREADY_REGISTERED = 1409

if (-not ('CopyNoteProbe' -as [type])) {
  Add-Type -TypeDefinition (Get-Content (Join-Path $PSScriptRoot 'probe.cs') -Raw)
}

$script:Failures = 0

function Say([string]$Text) { Write-Host ('{0:HH:mm:ss.fff}  {1}' -f (Get-Date), $Text) }

# One verdict line. A check script exits with its number of failures.
function Check([bool]$Ok, [string]$What, [string]$Detail = '') {
  if (-not $Ok) { $script:Failures++ }
  $mark = if ($Ok) { 'ok  ' } else { 'FAIL' }
  $suffix = if ($Detail) { " ($Detail)" } else { '' }
  Say "[$mark] $What$suffix"
}

function Complete-Checks {
  if ($script:Failures -eq 0) { Say 'all checks passed' } else { Say "$($script:Failures) check(s) failed" }
  exit $script:Failures
}

function Assert-NotRunning {
  if ([CopyNoteProbe]::Tray($TrayClass) -ne [IntPtr]::Zero) {
    throw 'A test instance is already running (its tray window exists). Quit it first.'
  }
}

function Assert-Built {
  if (-not (Test-Path $Exe)) { throw "No test build at $Exe. Run build.ps1 first." }
  Assert-NotRunning
}

# Fresh settings and no entries for the next launch: autorun off, default hotkey.
function Reset-TestData {
  $dir = Join-Path $WorkDir 'appdata'
  if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
}

# Starts the test build with its own data, log and WebView2 profile, and
# DevTools on $CdpPort. The variables are set only around the launch: a
# `go build` from this shell must keep using the real module cache and settings.
function Start-TestInstance([string[]]$Arguments = @(), [string]$LocalAppData = 'localappdata') {
  $vars = @{
    APPDATA                               = Join-Path $WorkDir 'appdata'
    LOCALAPPDATA                          = Join-Path $WorkDir $LocalAppData
    WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=$CdpPort"
  }
  $saved = @{}
  foreach ($name in $vars.Keys) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name)
    Set-Item "env:$name" $vars[$name]
  }
  try {
    if ($Arguments.Count -gt 0) { return Start-Process -FilePath $Exe -ArgumentList $Arguments -PassThru }
    return Start-Process -FilePath $Exe -PassThru
  }
  finally {
    foreach ($name in $saved.Keys) {
      if ($null -eq $saved[$name]) { Remove-Item "env:$name" -ErrorAction SilentlyContinue }
      else { Set-Item "env:$name" $saved[$name] }
    }
  }
}

# Quits the test instance the way its tray does - WM_QUIT on the tray window -
# then waits for it and for its WebView2 processes, which would otherwise keep
# the DevTools port from the next launch.
function Stop-TestInstance($Process) {
  if ($null -eq $Process) { return }
  if (-not $Process.HasExited) {
    $tray = [CopyNoteProbe]::Tray($TrayClass)
    if ($tray -ne [IntPtr]::Zero) { [void][CopyNoteProbe]::Post($tray, $WM_QUIT, 0, 0) }
    if (-not $Process.WaitForExit(15000)) {
      Say 'the test instance did not quit within 15 s; killing it'
      try { $Process.Kill() } catch { }
      [void]$Process.WaitForExit(5000)
    }
  }
  $deadline = (Get-Date).AddSeconds(20)
  while ((Get-Date) -lt $deadline) {
    $left = @(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.CommandLine -like '*copynote-testinstance*' })
    if ($left.Count -eq 0) { return }
    Start-Sleep -Milliseconds 250
  }
}

# Evaluates a JavaScript expression in the test instance's page, awaiting a
# promise, and returns the result as JSON. The Go bridge is reachable the same
# way: window.applyHotkey('Ctrl+Alt+M').
function Invoke-Page([string]$Expression) {
  return ((& node (Join-Path $PSScriptRoot 'cdp.mjs') $CdpPort $Expression) -join "`n")
}

# Sends one raw DevTools command to the page. Input.dispatchMouseEvent and
# Input.dispatchKeyEvent arrive as trusted input without moving the pointer or
# typing into another window. Params travel base64-encoded, past any quoting.
function Send-PageCommand([string]$Method, [hashtable]$Params) {
  $json = $Params | ConvertTo-Json -Compress -Depth 5
  $encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json))
  return ((& node (Join-Path $PSScriptRoot 'cdp.mjs') $CdpPort '--send' $Method $encoded) -join "`n")
}

function Wait-PageReady([int]$TimeoutSec = 60) {
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    if ((Invoke-Page "document.readyState === 'complete' && !!document.querySelector('[data-shell]')") -eq 'true') { return $true }
    Start-Sleep -Milliseconds 200
  }
  return $false
}

# When the page's DOMContentLoaded finished, in Unix ms like CopyNoteProbe::Now.
function Get-PageLoadedAt {
  return [long](Invoke-Page "Math.round(performance.timeOrigin + performance.getEntriesByType('navigation')[0].domContentLoadedEventEnd)")
}

function Get-RunValue([string]$Name) {
  $item = Get-ItemProperty $RunKey
  if ($item.PSObject.Properties[$Name]) { return [string]$item.$Name }
  return $null
}

# Whether some program holds the combination. The probe registers it for an
# instant itself, so call it only once the app is done registering.
function Test-HotkeyHeld([uint32]$Mods, [uint32]$Vk) {
  return [CopyNoteProbe]::HotkeyState($Mods, $Vk) -eq $ERROR_HOTKEY_ALREADY_REGISTERED
}

function Get-DailyCopyNote {
  $daily = Get-Process -ErrorAction SilentlyContinue |
    Where-Object { $_.ProcessName -like 'copynote*' -and $_.ProcessName -ne 'copynote-test' } |
    Select-Object -First 1
  return $daily
}
