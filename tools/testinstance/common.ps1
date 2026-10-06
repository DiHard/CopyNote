# Shared by the scripts in this folder: the names the test build runs under,
# where it keeps its files, and helpers to start, inspect and stop it.

$ErrorActionPreference = 'Stop'

$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
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
$script:RunValue = $Names['copynote/internal/service.autorunValueName']
$RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'

$script:MOD_ALT = 1
$script:MOD_CONTROL = 2
$script:VK_N = 0x4E
$script:VK_M = 0x4D
$WM_QUIT = 0x0012
$script:WM_HOTKEY = 0x0312
$script:WM_LBUTTONUP = 0x0202
$script:TRAY_CALLBACK = 0x8001 # WM_APP + 1, trayCallbackMsg in internal/tray
$ERROR_HOTKEY_ALREADY_REGISTERED = 1409

if (-not ('CopyNoteProbe' -as [type])) {
    Add-Type -TypeDefinition (Get-Content (Join-Path $PSScriptRoot 'probe.cs') -Raw)
}

# One run at a time. Two runs start instances under the same names and each
# then watches the other's window: a launch check has failed that way with
# nothing wrong in the build under test. Assert-NotRunning cannot see it, since
# the other run's instance has no tray window yet during its first moments.
# The mutex is held until the process exits; the scripts all.ps1 runs one
# after another share its thread and take it again without waiting.
$script:RunLock = [Threading.Mutex]::new($false, 'Local\dev.copynote.testinstance.run')
try { $script:RunLockTaken = $script:RunLock.WaitOne(0) }
catch [Threading.AbandonedMutexException] { $script:RunLockTaken = $true } # the last holder was killed
if (-not $script:RunLockTaken) { throw 'Another run of the test instance checks is in progress. Wait for it to finish.' }

$script:Failures = 0
$script:Skipped = 0

function Say([string]$Text) { Write-Host ('{0:HH:mm:ss.fff}  {1}' -f (Get-Date), $Text) }

# One verdict line. A check script exits with its number of failures.
function Check([bool]$Ok, [string]$What, [string]$Detail = '') {
    if (-not $Ok) { $script:Failures++ }
    $mark = if ($Ok) { 'ok  ' } else { 'FAIL' }
    $suffix = if ($Detail) { " ($Detail)" } else { '' }
    Say "[$mark] $What$suffix"
}

# A check that cannot mean anything in this session. Said out loud instead of
# being passed or failed: both would be wrong. The count also goes into the
# environment, which is how all.ps1 learns of it: a script's exit code is its
# number of failures.
function Skip([string]$What, [string]$Why) {
    $script:Skipped++
    $env:COPYNOTE_TEST_SKIPPED = [string]([int]$env:COPYNOTE_TEST_SKIPPED + 1)
    Say "[skip] $What ($Why)"
}

function Complete-Checks {
    $verdict = if ($script:Failures -gt 0) { "$($script:Failures) check(s) failed" }
    elseif ($script:Skipped -gt 0) { 'nothing failed' }
    else { 'all checks passed' }
    if ($script:Skipped -gt 0) { $verdict += "; $($script:Skipped) skipped" }
    Say $verdict
    exit $script:Failures
}

# Whether this session can give a window the foreground and take keystrokes.
# With the lock screen up - someone is following from another device -
# SetForegroundWindow and keybd_event both fail. The app then rightly treats
# its window as not the active one, and every check about activation would
# fail, or pass for the wrong reason.
$script:Interactive = [CopyNoteProbe]::Interactive()
$script:NotInteractive = 'the session is locked, so no window can become the active one'

# Check, for a result that depends on which window is active.
function CheckActive([bool]$Ok, [string]$What, [string]$Detail = '') {
    if ($Interactive) { Check $Ok $What $Detail } else { Skip $What $NotInteractive }
}

# The foreground a click into the window would give it. Windows does not count
# DevTools input as input, so without this the window never has the activation
# its menu takes or a click on the tray icon takes away.
function Set-WindowForeground([IntPtr]$Hwnd) {
    [void][CopyNoteProbe]::ForceForeground($Hwnd)
    Start-Sleep -Milliseconds 150
    return ([CopyNoteProbe]::Foreground() -eq $Hwnd)
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
    $root = [IO.Path]::GetFullPath($WorkDir)
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
    if (-not $root.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        -not [IO.Path]::GetFileName($root).StartsWith('copynote-testinstance', [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clear test data outside the test directory: $root"
    }
    $dir = [IO.Path]::GetFullPath((Join-Path $root 'appdata'))
    if (-not $dir.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clear test data outside $root"
    }
    if (Test-Path -LiteralPath $dir) { Remove-Item -LiteralPath $dir -Recurse -Force }
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

# Quits the test instance by ending its tray's message loop - WM_QUIT on the
# tray window, which is all the tray's own Quit comes down to - then waits for
# it and for its WebView2 processes, which would otherwise keep the DevTools
# port from the next launch.
function Stop-TestInstance($Process) {
    if ($null -eq $Process) { return }
    if (-not $Process.HasExited) {
        $tray = [CopyNoteProbe]::Tray($TrayClass)
        if ($tray -ne [IntPtr]::Zero) { [void][CopyNoteProbe]::Post($tray, $WM_QUIT, 0, 0) }
        if (-not $Process.WaitForExit(15000)) {
            Say 'the test instance did not quit within 15 s; killing it'
            try { $Process.Kill() } catch { Write-Verbose "Test process exited before it could be killed: $_" }
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
