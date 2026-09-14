# Autorun under the test name: turning it on in the test build writes
# Run\CopyNoteTest, turning it off deletes it, and the daily installation's
# Run\CopyNote is never touched.
. (Join-Path $PSScriptRoot 'common.ps1')
Assert-Built
Reset-TestData

$dailyBefore = Get-RunValue 'CopyNote'
$setAutorun = '(async (on) => { const s = await window.getSettings(); await window.saveSettings({ ...s, autorun: on }); return (await window.getSettings()).autorun })'
$p = Start-TestInstance @('--autostart')
try {
  Check (Wait-PageReady) 'the page loads'
  $on = Invoke-Page "$setAutorun(true)"
  $written = Get-RunValue $RunValue
  Check (($on -eq 'true') -and ($null -ne $written) -and $written.Contains((Split-Path $Exe -Leaf))) "turning autorun on writes Run\$RunValue for the test build" $written
  $off = Invoke-Page "$setAutorun(false)"
  Check (($off -eq 'false') -and ($null -eq (Get-RunValue $RunValue))) "turning it off deletes Run\$RunValue"
}
finally {
  Stop-TestInstance $p
  # A failed run must not leave Windows starting the test build at sign-in.
  if ($null -ne (Get-RunValue $RunValue)) {
    Remove-ItemProperty -Path $RunKey -Name $RunValue
    Say "removed a leftover Run\$RunValue"
  }
}

$dailyAfter = Get-RunValue 'CopyNote'
$detail = if ($null -eq $dailyBefore) { 'absent before and after' } else { $dailyBefore }
Check ($dailyAfter -eq $dailyBefore) 'Run\CopyNote is untouched' $detail
Complete-Checks
