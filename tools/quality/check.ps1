# The same checks run locally and in Windows CI. Fix mode only formats files.
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
$module = Join-Path $toolRoot "psmodules/PSScriptAnalyzer/$($versions.PSScriptAnalyzer)/PSScriptAnalyzer.psd1"
if (-not (Test-Path $module) -or -not (Test-Path (Join-Path $bin 'golangci-lint.exe'))) {
    throw 'Run pwsh tools/quality/setup.ps1 first.'
}
Import-Module $module -Force
$env:GOCACHE = Join-Path $toolRoot 'go-build'
$env:GOPATH = Join-Path $toolRoot 'go'
$env:GOMODCACHE = Join-Path $toolRoot 'go-mod'
$env:GOTOOLCHAIN = (Select-String -LiteralPath (Join-Path $repoRoot 'go.mod') -Pattern '^toolchain ').Line.Split(' ')[1]
$env:GOLANGCI_LINT_CACHE = Join-Path $toolRoot 'golangci-cache'
$env:npm_config_cache = Join-Path $toolRoot 'npm-cache'

function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Command failed with exit code $LASTEXITCODE" }
}

Push-Location $repoRoot
try {
    $goFiles = @(
        Get-ChildItem -Path '*.go' -File
        Get-ChildItem -Path 'internal', 'tools' -Filter '*.go' -Recurse -File
    ).FullName
    if ($Fix) {
        Invoke-Checked (Join-Path $bin 'goimports.exe') (@('-w', '-local', 'copynote') + $goFiles)
    }
    else {
        $unformatted = @(& gofmt -l @goFiles)
        if ($LASTEXITCODE -ne 0 -or $unformatted.Count -gt 0) { throw "gofmt: $($unformatted -join ', ')" }
        $unformatted = @(& (Join-Path $bin 'goimports.exe') -l -local copynote @goFiles)
        if ($LASTEXITCODE -ne 0 -or $unformatted.Count -gt 0) { throw "goimports: $($unformatted -join ', ')" }
    }
    $psFiles = Get-ChildItem 'tools' -Recurse -File -Include '*.ps1', '*.psd1'
    foreach ($file in $psFiles) {
        $source = Get-Content -LiteralPath $file.FullName -Raw
        $formatted = (Invoke-Formatter -ScriptDefinition $source.Replace("`r`n", "`n") -Settings CodeFormatting).Replace("`r`n", "`n").TrimEnd() + "`n"
        if ($Fix -and $source -cne $formatted) {
            $temporary = $file.FullName + '.format.tmp'
            [IO.File]::WriteAllText($temporary, $formatted)
            [IO.File]::Move($temporary, $file.FullName, $true)
        }
        elseif (-not $Fix -and $source -cne $formatted) { throw "PowerShell formatting: $($file.FullName)" }
    }
    if ($Fix) {
        Invoke-Checked 'node' @('web/node_modules/prettier/bin/prettier.cjs', '--write', '.')
        return
    }
    Invoke-Checked 'node' @('web/node_modules/prettier/bin/prettier.cjs', '--check', '.')
    $diagnostics = @($psFiles | ForEach-Object { Invoke-ScriptAnalyzer -Path $_.FullName -Settings (Join-Path $PSScriptRoot 'PSScriptAnalyzerSettings.psd1') })
    if ($diagnostics.Count -gt 0) {
        $diagnostics | Format-Table ScriptName, Line, RuleName, Message -AutoSize -Wrap
        throw "PSScriptAnalyzer: $($diagnostics.Count) diagnostics"
    }
    Invoke-Checked (Join-Path $bin 'actionlint.exe') @('-shellcheck=', '-pyflakes=')
    Invoke-Checked (Join-Path $bin 'golangci-lint.exe') @('config', 'verify')
    Invoke-Checked (Join-Path $bin 'golangci-lint.exe') @('run', './...')
    Push-Location 'web'
    try {
        Invoke-Checked 'npm.cmd' @('run', 'quality')
        if (-not $SkipVulnerabilities) { Invoke-Checked 'npm.cmd' @('audit', '--audit-level=moderate') }
        Invoke-Checked 'npm.cmd' @('test')
        Invoke-Checked 'npm.cmd' @('run', 'build')
    }
    finally { Pop-Location }
    # Keep test executables in the checkout: endpoint policies may reject executables under TEMP.
    New-Item -ItemType Directory -Force '.quality/test-bin' | Out-Null
    Invoke-Checked 'go' @('test', '-o', '.quality/test-bin/', './...')
    Invoke-Checked 'go' @('vet', './...')
    Invoke-Checked 'go' @('build', '-ldflags=-H=windowsgui -s -w', '-o', 'build/copynote.exe', '.')
    if (-not $SkipVulnerabilities) {
        Invoke-Checked (Join-Path $bin 'govulncheck.exe') @('./...')
    }
    if ($Fuzz) {
        Invoke-Checked 'go' @('test', '-o', '.quality/test-bin/parser-fuzz.exe', './internal/hotkey', '-fuzz=FuzzResolve', '-fuzztime=10s', '-parallel=2')
        Invoke-Checked 'go' @('test', '-o', '.quality/test-bin/storage-fuzz.exe', './internal/storage', '-fuzz=FuzzDecode', '-fuzztime=10s', '-parallel=2')
    }
    Write-Host 'All quality checks passed.'
}
finally { Pop-Location }
