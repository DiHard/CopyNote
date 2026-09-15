# Launch behaviour: a launch by hand brings the window up by itself with the
# caret in the search box; an --autostart launch - what a Windows sign-in
# runs - stays out of sight until the tray icon is clicked.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

Say '== Launched by hand'
$launched = [CopyNoteProbe]::Now()
$p = Start-TestInstance
try {
  $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 60000)
  Check ($shown -ge 0) 'the window comes up by itself' ('{0} ms after the launch' -f ($shown - $launched))
  if ($shown -ge 0) {
    Check (Wait-PageReady) 'the page loads'
    Check ([CopyNoteProbe]::StaysOnScreen($p.Id, 1500)) 'the window stays up'
    $focused = Invoke-Page 'document.activeElement && document.activeElement.id'
    Check ($focused -eq '"entry-search"') 'the search box has focus' $focused
  }
}
finally { Stop-TestInstance $p }

Say '== Launched with --autostart'
$p = Start-TestInstance @('--autostart')
try {
  Check (Wait-PageReady) 'the page loads'
  Check ([CopyNoteProbe]::StaysParked($p.Id, 3000)) 'the window stays out of sight'
  $clicked = [CopyNoteProbe]::Now()
  [void][CopyNoteProbe]::Post([CopyNoteProbe]::Tray($TrayClass), $TRAY_CALLBACK, 0, $WM_LBUTTONUP)
  $shown = [CopyNoteProbe]::WaitOnScreen($p.Id, 5000)
  Check ($shown -ge 0) 'a click on the tray icon brings it up' ('after {0} ms' -f ($shown - $clicked))
}
finally { Stop-TestInstance $p }

Complete-Checks
