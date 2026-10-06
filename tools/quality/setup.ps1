# Install pinned tools inside this checkout, without changing global tools.
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$toolRoot = Join-Path $repoRoot '.quality'
$versions = Get-Content (Join-Path $PSScriptRoot 'versions.json') -Raw | ConvertFrom-Json
$bin = Join-Path $toolRoot 'bin'
$installed = Join-Path $toolRoot 'installed'
New-Item -ItemType Directory -Force $bin, $installed | Out-Null

# A tool is in place when its marker names the pinned version, so a second run
# - or a CI run with .quality restored from its cache - downloads nothing.
function Test-Installed([string]$Tool, [string]$Version) {
    $marker = Join-Path $installed $Tool
    return (Test-Path -LiteralPath $marker) -and ((Get-Content -LiteralPath $marker -Raw).Trim() -eq $Version)
}

function Set-Installed([string]$Tool, [string]$Version) {
    [IO.File]::WriteAllText((Join-Path $installed $Tool), $Version)
}

# Downloads an archive, checks it against the SHA-256 pinned for it in
# versions.json and unpacks it. No pinned hash, no download: after changing a
# version, add the hash of the new archive - checked against the publisher's
# own - to versions.json.
function Get-ToolArchive([string]$Url, [string]$Name) {
    $expected = $versions.sha256.$Name
    if (-not $expected) { throw "tools/quality/versions.json has no sha256 for $Name" }
    $archive = Join-Path $toolRoot "downloads/$Name"
    & node (Join-Path $PSScriptRoot 'download.mjs') $Url $archive $expected
    if ($LASTEXITCODE -ne 0) { throw "Download failed: $Name" }
    $destination = Join-Path $toolRoot "downloads/$Name.contents"
    Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force
    return $destination
}

$lintVersion = $versions.'golangci-lint'
if (-not (Test-Installed 'golangci-lint' $lintVersion)) {
    $name = "golangci-lint-$lintVersion-windows-amd64.zip"
    $expanded = Get-ToolArchive "https://github.com/golangci/golangci-lint/releases/download/v$lintVersion/$name" $name
    Copy-Item -LiteralPath (Join-Path $expanded "golangci-lint-$lintVersion-windows-amd64/golangci-lint.exe") -Destination $bin -Force
    Set-Installed 'golangci-lint' $lintVersion
}

$actionVersion = $versions.actionlint
if (-not (Test-Installed 'actionlint' $actionVersion)) {
    $name = "actionlint_${actionVersion}_windows_amd64.zip"
    $expanded = Get-ToolArchive "https://github.com/rhysd/actionlint/releases/download/v$actionVersion/$name" $name
    Copy-Item -LiteralPath (Join-Path $expanded 'actionlint.exe') -Destination $bin -Force
    Set-Installed 'actionlint' $actionVersion
}

$analyzerVersion = $versions.PSScriptAnalyzer
if (-not (Test-Installed 'PSScriptAnalyzer' $analyzerVersion)) {
    $modulePath = Join-Path $toolRoot "psmodules/PSScriptAnalyzer/$analyzerVersion"
    $expanded = Get-ToolArchive "https://www.powershellgallery.com/api/v2/package/PSScriptAnalyzer/$analyzerVersion" "PSScriptAnalyzer.$analyzerVersion.zip"
    New-Item -ItemType Directory -Force $modulePath | Out-Null
    Copy-Item -Path (Join-Path $expanded '*') -Destination $modulePath -Recurse -Force
    Set-Installed 'PSScriptAnalyzer' $analyzerVersion
}

# Built from source by Go, which checks the modules against its checksum
# database. Only the binary lands in this checkout; the module and build
# caches are Go's own, shared with every other build on the machine.
$vulnVersion = $versions.govulncheck
if (-not (Test-Installed 'govulncheck' $vulnVersion)) {
    $env:GOBIN = $bin
    $env:GOTOOLCHAIN = (Select-String -LiteralPath (Join-Path $repoRoot 'go.mod') -Pattern '^toolchain ').Line.Split(' ')[1]
    & go install "golang.org/x/vuln/cmd/govulncheck@$vulnVersion"
    if ($LASTEXITCODE -eq 0) { Set-Installed 'govulncheck' $vulnVersion }
    else {
        # Not fatal: on a network that cannot reach proxy.golang.org nothing
        # else here depends on it, and CI runs the vulnerability checks.
        Write-Warning 'govulncheck could not be installed. Run check.ps1 with -SkipVulnerabilities on this machine.'
    }
}

Push-Location (Join-Path $repoRoot 'web')
try {
    & npm.cmd ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
}
finally { Pop-Location }
Write-Host 'Quality tools installed. Run pwsh tools/quality/check.ps1.'
