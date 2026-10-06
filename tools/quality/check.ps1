# The same checks run locally and in Windows CI. Fix mode only formats files.
#
# What is checked is what git tracks. A scratch file lying in the checkout is
# neither judged nor, in Fix mode, rewritten; a new file joins once it is staged.
param(
    [switch]$Fix,
    [switch]$SkipVulnerabilities,
    [switch]$Fuzz
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$toolRoot = Join-Path $repoRoot '.quality'
$bin = Join-Path $toolRoot 'bin'
$versions = Get-Content (Join-Path $PSScriptRoot 'versions.json') -Raw | ConvertFrom-Json
$analyzer = Join-Path $toolRoot "psmodules/PSScriptAnalyzer/$($versions.PSScriptAnalyzer)/PSScriptAnalyzer.psd1"
$golangci = Join-Path $bin 'golangci-lint.exe'
if (-not (Test-Path $analyzer) -or -not (Test-Path $golangci)) {
    throw 'Run pwsh tools/quality/setup.ps1 first.'
}
Import-Module $analyzer -Force
# Go's module and build caches and npm's cache stay where they normally are:
# what one build fetched or compiled the next one reuses, and so does CI.
$env:GOTOOLCHAIN = (Select-String -LiteralPath (Join-Path $repoRoot 'go.mod') -Pattern '^toolchain ').Line.Split(' ')[1]
$env:GOLANGCI_LINT_CACHE = Join-Path $toolRoot 'golangci-cache'

function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Command failed with exit code $LASTEXITCODE" }
}

# A long list goes in several calls, so it never outgrows the command line.
function Invoke-OnFiles([string]$Command, [string[]]$Arguments, [string[]]$Files) {
    for ($start = 0; $start -lt $Files.Count; $start += 200) {
        $end = [Math]::Min($start + 200, $Files.Count) - 1
        Invoke-Checked $Command ($Arguments + $Files[$start..$end])
    }
}

Push-Location $repoRoot
try {
    # One line per tracked file: "i/lf w/crlf attr/text=auto eol=lf<TAB>path".
    $listing = @(& git ls-files --eol)
    if ($LASTEXITCODE -ne 0) { throw 'git ls-files failed' }
    $tracked = @($listing | ForEach-Object {
            $state, $path = $_ -split "`t", 2
            [pscustomobject]@{ State = $state; Path = $path }
        })
    $quoted = @($tracked.Path -like '"*')
    if ($quoted.Count -gt 0) { throw "Tracked paths that git has to quote are not supported: $($quoted -join ', ')" }
    # A file deleted but not yet staged is still listed.
    $tracked = @($tracked | Where-Object { Test-Path -LiteralPath $_.Path -PathType Leaf })
    $untracked = @(& git ls-files --others --exclude-standard --directory)
    $skipped = if ($untracked.Count -gt 0) { "Not tracked by git, so left out of formatting and linting: $($untracked -join ', ')" }

    # .gitattributes pins LF, but git applies it only to the files it writes
    # out: a checkout older than that file keeps the CRLF copies core.autocrlf
    # made. Every formatter below rejects those, and under third_party, where
    # conversion is off, they would be committed as they are.
    $stale = @($tracked | Where-Object { $_.State -match '^i/lf\s+w/(crlf|mixed)\s' })
    if ($stale.Count -gt 0) {
        if (-not $Fix) {
            throw "$($stale.Count) tracked file(s) have CRLF line endings here and LF in git, for example $($stale[0].Path). Run tools/quality/check.ps1 -Fix once to convert them."
        }
        # Latin-1 maps every byte to one character and back, whatever the file's encoding.
        $latin1 = [Text.Encoding]::Latin1
        foreach ($file in $stale) {
            $path = Join-Path $repoRoot $file.Path
            $text = $latin1.GetString([IO.File]::ReadAllBytes($path))
            [IO.File]::WriteAllBytes($path, $latin1.GetBytes($text.Replace("`r`n", "`n")))
        }
        Write-Host "Converted $($stale.Count) file(s) to LF line endings."
    }

    $files = @($tracked.Path -notlike 'third_party/*')
    $goFiles = @($files -like '*.go')
    $psFiles = @($files -match '\.psd?1$')
    $prettier = @('web/node_modules/prettier/bin/prettier.cjs', '--ignore-unknown')

    # golangci-lint's formatters are gofmt and goimports, configured in
    # .golangci.yml. Given the files rather than packages, it also covers the
    # ones a build constraint keeps out of every package.
    if ($Fix) { Invoke-OnFiles $golangci @('fmt') $goFiles }
    else { Invoke-OnFiles $golangci @('fmt', '--diff') $goFiles }
    foreach ($file in $psFiles) {
        $source = Get-Content -LiteralPath $file -Raw
        $formatted = (Invoke-Formatter -ScriptDefinition $source.Replace("`r`n", "`n") -Settings CodeFormatting).Replace("`r`n", "`n").TrimEnd() + "`n"
        if ($source -ceq $formatted) { continue }
        if (-not $Fix) { throw "PowerShell formatting: $file" }
        $path = Join-Path $repoRoot $file
        [IO.File]::WriteAllText("$path.format.tmp", $formatted)
        [IO.File]::Move("$path.format.tmp", $path, $true)
    }
    if ($Fix) {
        Invoke-OnFiles 'node' ($prettier + '--write', '--list-different') $files
        if ($skipped) { Write-Host $skipped }
        return
    }
    Invoke-OnFiles 'node' ($prettier + '--check') $files

    $diagnostics = @($psFiles | Invoke-ScriptAnalyzer -Settings (Join-Path $PSScriptRoot 'PSScriptAnalyzerSettings.psd1'))
    if ($diagnostics.Count -gt 0) {
        $diagnostics | Format-Table ScriptName, Line, RuleName, Message -AutoSize -Wrap
        throw "PSScriptAnalyzer: $($diagnostics.Count) diagnostics"
    }
    Invoke-Checked (Join-Path $bin 'actionlint.exe') @('-shellcheck=', '-pyflakes=')

    # Packages with a tracked source file. "./..." would also take in a scratch
    # package and the Go code some npm packages carry under web/node_modules.
    $sourceDirs = @($goFiles | ForEach-Object { ('./' + (Split-Path -Parent $_).Replace('\', '/')).TrimEnd('/') } | Sort-Object -Unique)
    $goModule = & go list -m
    $packages = @(& go list ./... | ForEach-Object { '.' + $_.Substring($goModule.Length) } | Where-Object { $_ -in $sourceDirs })
    if ($LASTEXITCODE -ne 0 -or $packages.Count -eq 0) { throw 'go list failed' }
    # govet is one of its linters, so there is no separate go vet.
    Invoke-Checked $golangci @('config', 'verify')
    Invoke-Checked $golangci (@('run') + $packages)

    $toolScripts = @($files -match '^tools/.*\.[cm]?js$' | ForEach-Object { $_.Substring('tools/'.Length) })
    Push-Location 'tools'
    try { Invoke-OnFiles 'node' @('../web/node_modules/eslint/bin/eslint.js', '--max-warnings', '0') $toolScripts }
    finally { Pop-Location }
    Push-Location 'web'
    try {
        foreach ($script in 'lint', 'lint:deps', 'lint:unused', 'check') { Invoke-Checked 'npm.cmd' @('run', $script) }
        if (-not $SkipVulnerabilities) { Invoke-Checked 'npm.cmd' @('audit', '--audit-level=moderate') }
        Invoke-Checked 'npm.cmd' @('test')
        Invoke-Checked 'npm.cmd' @('run', 'build')
    }
    finally { Pop-Location }

    # Keep test executables in the checkout: endpoint policies may reject executables under TEMP.
    New-Item -ItemType Directory -Force '.quality/test-bin' | Out-Null
    Invoke-Checked 'go' (@('test', '-o', '.quality/test-bin/') + $packages)
    Invoke-Checked 'go' @('build', '-ldflags=-H=windowsgui -s -w', '-o', 'build/copynote.exe', '.')
    if (-not $SkipVulnerabilities) {
        $govulncheck = Join-Path $bin 'govulncheck.exe'
        if (-not (Test-Path $govulncheck)) {
            throw 'govulncheck is not installed. Rerun tools/quality/setup.ps1, or pass -SkipVulnerabilities.'
        }
        Invoke-Checked $govulncheck $packages
    }
    if ($Fuzz) {
        Invoke-Checked 'go' @('test', '-o', '.quality/test-bin/parser-fuzz.exe', './internal/hotkey', '-fuzz=FuzzResolve', '-fuzztime=10s', '-parallel=2')
        Invoke-Checked 'go' @('test', '-o', '.quality/test-bin/storage-fuzz.exe', './internal/storage', '-fuzz=FuzzDecode', '-fuzztime=10s', '-parallel=2')
    }
    if ($skipped) { Write-Host $skipped }
    Write-Host 'All quality checks passed.'
}
finally { Pop-Location }
