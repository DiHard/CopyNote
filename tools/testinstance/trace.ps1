# Diagnostic, not a check: prints every change of the test window's position
# and focus for a few seconds after a launch by hand - for a window that shows
# up and then vanishes.
param(
  [int]$Seconds = 6,
  # A new WebView2 profile, as on a first run.
  [switch]$FreshProfile,
  # Launch after this shell has earned the right to hand the foreground on,
  # the position Explorer is in on a double click.
  [switch]$WithForegroundRights
)
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built

$localAppData = if ($FreshProfile) { 'localappdata-' + [CopyNoteProbe]::Now() } else { 'localappdata' }
if ($WithForegroundRights) { Say ('foreground rights handed on: ' + [CopyNoteProbe]::GainForegroundRights()) }
$launched = [CopyNoteProbe]::Now()
$p = Start-TestInstance -LocalAppData $localAppData
try {
  $trace = [CopyNoteProbe]::Trace($p.Id, $launched, $Seconds * 1000)
  $trace -split "`n" | Where-Object { $_ } | ForEach-Object { Say $_ }
  if (Wait-PageReady 10) { Say ('DOMContentLoaded at +{0} ms' -f ((Get-PageLoadedAt) - $launched)) }
}
finally {
  Stop-TestInstance $p
  if ($FreshProfile) { Remove-Item -Recurse -Force (Join-Path $WorkDir $localAppData) -ErrorAction SilentlyContinue }
}
