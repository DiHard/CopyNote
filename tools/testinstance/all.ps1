# Builds the test exe, then runs every check in turn. Exits with the total
# number of failed checks.

# Taken here, the one-run-at-a-time lock in common.ps1 is held from the build
# to the last check, not only while each script runs.
. (Join-Path $PSScriptRoot 'common.ps1')

# The one thing a test build could damage for good is the daily CopyNote's
# autorun entry: with fresh settings it deletes the value it believes is its
# own. build.ps1 verifies the name is overridden before anything is launched;
# this is the same question asked of the registry, before and after.
$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
function Get-DailyAutorun { return (Get-ItemProperty -Path $runKey -ErrorAction SilentlyContinue).CopyNote }
$autorunBefore = Get-DailyAutorun

if (-not $Interactive) {
    Write-Host 'The session is locked: checks that need an active window or typed keys are skipped, not failed.'
}

$failed = 0
$env:COPYNOTE_TEST_SKIPPED = '0' # counted by Skip in common.ps1
foreach ($step in 'build', 'launch', 'hotkey', 'coldstart', 'autorun', 'menu', 'slide', 'preparation') {
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
if ((Get-DailyAutorun) -ne $autorunBefore) {
    Write-Host "[FAIL] the daily CopyNote's autorun entry changed during the run. It was: $autorunBefore"
    $failed++
}
$skipped = [int]$env:COPYNOTE_TEST_SKIPPED
$verdict = if ($failed -gt 0) { "$failed check(s) failed" } elseif ($skipped -gt 0) { 'nothing failed' } else { 'everything passed' }
if ($skipped -gt 0) { $verdict += "; $skipped skipped - see the [skip] lines above for what was not checked and why" }
Write-Host $verdict
exit $failed
