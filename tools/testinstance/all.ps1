# Builds the test exe, then runs every check in turn. Exits with the total
# number of failed checks.
$failed = 0
foreach ($step in 'build', 'launch', 'hotkey', 'coldstart', 'autorun', 'menu', 'slide') {
  Write-Host ''
  Write-Host "######## $step"
  try {
    & (Join-Path $PSScriptRoot "$step.ps1")
    $failed += $LASTEXITCODE
    if ($step -eq 'build' -and $LASTEXITCODE -ne 0) { break }
  }
  catch {
    Write-Host "$step stopped: $_"
    $failed++
    if ($step -eq 'build') { break }
  }
}
Write-Host ''
if ($failed -eq 0) { Write-Host 'everything passed' } else { Write-Host "$failed check(s) failed" }
exit $failed
