# The cold start: a request that arrives before the UI can render waits, then
# brings the window up once - no blank window, no lost click. A second launch
# reaches this build's tray and never the daily CopyNote's.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

function Test-EarlyRequest([string]$What, [uint32]$Message, [long]$WParam, [long]$LParam, [int]$Count) {
  Say "== $What during the cold start"
  # --autostart, so nothing but the request can bring the window up.
  $p = Start-TestInstance @('--autostart')
  try {
    $posted = [CopyNoteProbe]::PostWhenTrayAppears($TrayClass, $Message, $WParam, $LParam, $Count, 60000)
    if ($posted -lt 0) { Check $false 'the tray window appears'; return }
    $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 60000)
    [void](Wait-PageReady)
    $loaded = Get-PageLoadedAt
    Check ($posted -lt $loaded) 'the request arrives before the page has loaded' ('{0} ms before DOMContentLoaded' -f ($loaded - $posted))
    Check ($shown -ge 0) 'the window comes up' ('{0} ms after the request' -f ($shown - $posted))
    # 20 ms of slack: the page and this script read the clock separately.
    Check (($shown -ge 0) -and ($shown + 20 -ge $loaded)) 'but not before the page has loaded'
    Check ([CopyNoteProbe]::StaysOnScreen($p.Id, 1000)) 'and it stays up'
  }
  finally { Stop-TestInstance $p }
}

Test-EarlyRequest 'Two clicks on the tray icon' $TRAY_CALLBACK 0 $WM_LBUTTONUP 2
Test-EarlyRequest 'The global hotkey' $WM_HOTKEY 1 (($VK_N -shl 16) -bor $MOD_CONTROL -bor $MOD_ALT) 1

Say '== A second launch during the cold start'
$daily = Get-DailyCopyNote
$dailyBefore = if ($daily) { [CopyNoteProbe]::Where([CopyNoteProbe]::MainWindow($daily.Id)) } else { $null }
$first = Start-TestInstance @('--autostart')
try {
  Start-Sleep -Milliseconds 300 # the first copy has to hold the mutex before the second starts
  $second = Start-TestInstance
  $handedOver = $second.WaitForExit(30000)
  Check ($handedOver -and $second.ExitCode -eq 0) 'the second copy hands the request over and exits'
  Check ([CopyNoteProbe]::WaitOnScreen($first.Id, 60000) -ge 0) "the first copy's window comes up"
  if ($daily) {
    $dailyAfter = [CopyNoteProbe]::Where([CopyNoteProbe]::MainWindow($daily.Id))
    Check ($dailyAfter -eq $dailyBefore) "the daily CopyNote's window is left alone" $dailyAfter
  }
  else { Say 'no daily CopyNote is running, so there is nothing to leave alone' }
}
finally { Stop-TestInstance $first }

Complete-Checks
