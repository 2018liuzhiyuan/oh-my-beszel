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

New-Item -ItemType Directory -Force -Path $outDir | Out-Null

Push-Location $repoRoot
go build -trimpath -ldflags '-s -w' -o (Join-Path $outDir 'beszel.exe') ./internal/cmd/hub
go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $outDir 'Monitor.exe') ./internal/cmd/monitor
Pop-Location

Copy-Item (Join-Path $PSScriptRoot 'app\*') $outDir -Force
if (-not (Test-Path (Join-Path $outDir 'config.json'))) {
    Copy-Item (Join-Path $outDir 'config.example.json') (Join-Path $outDir 'config.json')
}

Write-Host "Package ready: $outDir"
Write-Host 'Next: edit config.json, then run install-task.ps1 (autostart) or double-click Monitor.exe.'
