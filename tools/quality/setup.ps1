# Install pinned tools inside this checkout, without changing global tools.
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$toolRoot = Join-Path $repoRoot '.quality'
$versions = Get-Content (Join-Path $PSScriptRoot 'versions.json') -Raw | ConvertFrom-Json
$bin = Join-Path $toolRoot 'bin'
New-Item -ItemType Directory -Force $bin | Out-Null

function Get-ToolArchive([string]$Url, [string]$Name, [string]$ChecksumsUrl = '') {
    $archive = Join-Path $toolRoot "downloads/$Name"
    & node (Join-Path $PSScriptRoot 'download.mjs') $Url $archive $ChecksumsUrl
    if ($LASTEXITCODE -ne 0) { throw "Download failed: $Name" }
    $destination = Join-Path $toolRoot "downloads/$Name.contents"
    Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force
    return $destination
}

$lintVersion = $versions.'golangci-lint'
$baseUrl = "https://github.com/golangci/golangci-lint/releases/download/v$lintVersion"
$name = "golangci-lint-$lintVersion-windows-amd64.zip"
$expanded = Get-ToolArchive "$baseUrl/$name" $name "$baseUrl/golangci-lint-$lintVersion-checksums.txt"
Copy-Item -LiteralPath (Join-Path $expanded "golangci-lint-$lintVersion-windows-amd64/golangci-lint.exe") -Destination $bin -Force

$actionVersion = $versions.actionlint
$baseUrl = "https://github.com/rhysd/actionlint/releases/download/v$actionVersion"
$name = "actionlint_${actionVersion}_windows_amd64.zip"
$expanded = Get-ToolArchive "$baseUrl/$name" $name "$baseUrl/actionlint_${actionVersion}_checksums.txt"
Copy-Item -LiteralPath (Join-Path $expanded 'actionlint.exe') -Destination $bin -Force

$analyzerVersion = $versions.PSScriptAnalyzer
$modulePath = Join-Path $toolRoot "psmodules/PSScriptAnalyzer/$analyzerVersion"
$installedMarker = Join-Path $modulePath '.installed'
if (-not (Test-Path $installedMarker)) {
    $expanded = Get-ToolArchive "https://www.powershellgallery.com/api/v2/package/PSScriptAnalyzer/$analyzerVersion" "PSScriptAnalyzer.$analyzerVersion.zip"
    New-Item -ItemType Directory -Force $modulePath | Out-Null
    Copy-Item -Path (Join-Path $expanded '*') -Destination $modulePath -Recurse -Force
    [IO.File]::WriteAllText($installedMarker, $analyzerVersion)
}

$env:GOPATH = Join-Path $toolRoot 'go'
$env:GOBIN = $bin
$env:GOMODCACHE = Join-Path $toolRoot 'go-mod'
$env:GOCACHE = Join-Path $toolRoot 'go-build'
$env:GOTOOLCHAIN = (Select-String -LiteralPath (Join-Path $repoRoot 'go.mod') -Pattern '^toolchain ').Line.Split(' ')[1]
& go install "golang.org/x/tools/cmd/goimports@$($versions.goimports)"
if ($LASTEXITCODE -ne 0) { throw 'goimports installation failed' }
& go install "golang.org/x/vuln/cmd/govulncheck@$($versions.govulncheck)"
if ($LASTEXITCODE -ne 0) { throw 'govulncheck installation failed' }

$env:npm_config_cache = Join-Path $toolRoot 'npm-cache'
Push-Location (Join-Path $repoRoot 'web')
try {
    & npm.cmd ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
}
finally { Pop-Location }
Write-Host 'Quality tools installed. Run pwsh tools/quality/check.ps1.'
