# Builds copynote-test.exe from the current sources under the test names.
# The frontend goes in as committed in web/dist: run `npm run build` in web/
# first if you changed it.
. (Join-Path $PSScriptRoot 'common.ps1')
# A running test instance keeps the exe locked.
Assert-NotRunning

$overrides = ($Names.GetEnumerator() | ForEach-Object { "-X $($_.Key)=$($_.Value)" }) -join ' '
New-Item -ItemType Directory -Force (Split-Path $Exe) | Out-Null
Push-Location $RepoRoot
try {
  & go build "-ldflags=-H=windowsgui -s -w $overrides" -o $Exe .
  if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
}
finally { Pop-Location }
Say "built $Exe"

# -X ignores a constant or a mistyped name without a word, so look for every
# name in the binary. Longest first, each removed once found, so a name that
# is the prefix of another is not credited with the other's bytes.
$text = [Text.Encoding]::ASCII.GetString([IO.File]::ReadAllBytes($Exe))
foreach ($entry in ($Names.GetEnumerator() | Sort-Object { $_.Value.Length } -Descending)) {
  Check ($text.Contains($entry.Value)) "$($entry.Key) is overridden" $entry.Value
  $text = $text.Replace($entry.Value, '')
}
Complete-Checks
