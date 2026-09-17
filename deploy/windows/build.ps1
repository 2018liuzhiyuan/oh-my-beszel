# Builds the Windows one-click package into build\windows, ready to zip and
# distribute. Users only see Monitor.exe and their config at the root; the hub
# (beszel.exe, agents, data, logs) lives inside the app\ folder.
#
# Requirements: Go 1.26+ in PATH (https://golang.google.cn/dl/ or https://go.dev/dl/).
# Node.js is only needed if internal/site/dist is missing (or -BuildWebUi is given).
param(
    [string]$GoProxy = 'https://goproxy.cn,direct',
    [switch]$BuildWebUi
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$siteDir = Join-Path $repoRoot 'internal\site'
$distIndex = Join-Path $siteDir 'dist\index.html'
$outDir = Join-Path $repoRoot 'build\windows'
$hubDir = Join-Path $outDir 'app'
$agentOutDir = Join-Path $hubDir 'agents'
$goBuildCache = Join-Path $repoRoot '.tmp\go-build-cache'
$previousGoExperiment = $env:GOEXPERIMENT

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory)] [string]$Output,
        [Parameter(Mandatory)] [string]$Package,
        [Parameter(Mandatory)] [string]$LdFlags
    )

    $nativeArgs = @('build', '-trimpath', '-buildvcs=false', '-ldflags', $LdFlags, '-o', $Output, $Package)
    & go @nativeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed for $Package with exit code $LASTEXITCODE"
    }
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go not found in PATH. Install Go 1.26+ from https://golang.google.cn/dl/'
}

if ($BuildWebUi -or -not (Test-Path $distIndex)) {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        throw 'internal/site/dist is missing and npm was not found. Install Node.js to build the web UI.'
    }
    Push-Location $siteDir
    if (-not (Test-Path 'src\locales\en\en.ts')) { npm run sync }
    npm install
    npm run build
    Pop-Location
}

$env:GOPROXY = $GoProxy
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:GOCACHE = $goBuildCache
# PocketBase 0.36 recursively re-enters Collection.UnmarshalJSON when the
# Go 1.27 json/v2 implementation initializes a fresh database.
$env:GOEXPERIMENT = 'nojsonv2'

New-Item -ItemType Directory -Force -Path $outDir | Out-Null
New-Item -ItemType Directory -Force -Path $hubDir | Out-Null

foreach ($legacyPayload in @(
    (Join-Path $outDir 'configure-example.ps1'),
    (Join-Path $hubDir 'run-hub.ps1')
)) {
    Remove-Item -LiteralPath $legacyPayload -Force -ErrorAction SilentlyContinue
}

# Refuse to package runtime state: a stale database or log would ship a
# pre-migrated hub to every user (no first-boot account creation, and
# possibly someone else's data). The hub's runtime files land in app\.
foreach ($runtimeRoot in @($outDir, $hubDir)) {
    foreach ($runtimeArtifact in @('beszel_data', 'hub.log', 'hub.log.1', 'launcher.log')) {
        $runtimePath = Join-Path $runtimeRoot $runtimeArtifact
        if (Test-Path -LiteralPath $runtimePath) {
            throw "Runtime artifact '$runtimeArtifact' exists in $runtimeRoot. Move or delete it before building; never ship it in a package."
        }
    }
}

New-Item -ItemType Directory -Force -Path $agentOutDir | Out-Null
New-Item -ItemType Directory -Force -Path $goBuildCache | Out-Null

Push-Location $repoRoot
try {
    Invoke-GoBuild -Output (Join-Path $hubDir 'beszel.exe') -Package './internal/cmd/hub' -LdFlags '-s -w'
    Invoke-GoBuild -Output (Join-Path $outDir 'Monitor.exe') -Package './internal/cmd/monitor' -LdFlags '-s -w -H windowsgui'
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    Invoke-GoBuild -Output (Join-Path $agentOutDir 'beszel-agent_linux_amd64') -Package './internal/cmd/agent' -LdFlags '-s -w'
    $env:GOARCH = 'arm64'
    Invoke-GoBuild -Output (Join-Path $agentOutDir 'beszel-agent_linux_arm64') -Package './internal/cmd/agent' -LdFlags '-s -w'
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    if ($null -eq $previousGoExperiment) {
        Remove-Item Env:GOEXPERIMENT -ErrorAction SilentlyContinue
    } else {
        $env:GOEXPERIMENT = $previousGoExperiment
    }
    Pop-Location
}

Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'app\config.example.json') -Destination $outDir -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'app\readme.md') -Destination $outDir -Force
if (-not (Test-Path (Join-Path $outDir 'config.json'))) {
    Copy-Item (Join-Path $outDir 'config.example.json') (Join-Path $outDir 'config.json')
}

$packagedPowerShell = @(Get-ChildItem -LiteralPath $outDir -Recurse -File -Filter '*.ps1')
if ($packagedPowerShell.Count -gt 0) {
    throw "Windows release must not contain PowerShell scripts: $($packagedPowerShell.FullName -join ', ')"
}

Write-Host "Package ready: $outDir"
Write-Host 'Next: double-click Monitor.exe - zero config, autostart registers itself on first run.'
