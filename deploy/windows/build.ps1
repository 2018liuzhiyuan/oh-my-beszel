# Builds the Windows one-click package (beszel.exe + Monitor.exe + app files)
# into build\windows, ready to zip and distribute.
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
$agentOutDir = Join-Path $outDir 'agents'
$goBuildCache = Join-Path $repoRoot '.tmp\go-build-cache'
$previousGoExperiment = $env:GOEXPERIMENT

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory)] [string]$Output,
        [Parameter(Mandatory)] [string]$Package,
        [Parameter(Mandatory)] [string]$LdFlags
    )

    $nativeArgs = @('build', '-trimpath', '-ldflags', $LdFlags, '-o', $Output, $Package)
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
New-Item -ItemType Directory -Force -Path $agentOutDir | Out-Null
New-Item -ItemType Directory -Force -Path $goBuildCache | Out-Null

Push-Location $repoRoot
try {
    Invoke-GoBuild -Output (Join-Path $outDir 'beszel.exe') -Package './internal/cmd/hub' -LdFlags '-s -w'
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

Copy-Item (Join-Path $PSScriptRoot 'app\*') $outDir -Force
if (-not (Test-Path (Join-Path $outDir 'config.json'))) {
    Copy-Item (Join-Path $outDir 'config.example.json') (Join-Path $outDir 'config.json')
}

Write-Host "Package ready: $outDir"
Write-Host 'Next: edit config.json, then run install-task.ps1 (autostart) or double-click Monitor.exe.'
