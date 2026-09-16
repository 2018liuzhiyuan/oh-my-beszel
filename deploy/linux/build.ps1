# Builds the Linux (amd64) release tarball from Windows via Go
# cross-compilation. No WSL needed: everything is CGO-free (the NVML/glibc
# agent variant is just the `glibc` build tag; NVML is loaded at runtime).
#
# The tar is written by Python's tarfile so the executable bit survives:
# packing NTFS-built files with a naive tar ships 644 binaries that a Linux
# user cannot run after extraction (the om.3 launch shipped exactly that).
#
# Usage: pwsh -File deploy/linux/build.ps1 [-Tag v0.18.8-om.4]
# Output: build\oh-my-beszel_<Tag>_linux_amd64.tar.gz (+ staging in build\linux)

[CmdletBinding()]
param(
    [string]$Tag = 'dev',
    [string]$GoProxy = 'https://goproxy.cn,direct'
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$stageDir = Join-Path $repoRoot 'build\linux'
$tarPath = Join-Path $repoRoot "build\oh-my-beszel_${Tag}_linux_amd64.tar.gz"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go not found in PATH'
}
if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    throw 'python not found in PATH (needed to write the tar with exec bits)'
}

$env:GOPROXY = $GoProxy
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:GOCACHE = Join-Path $repoRoot '.tmp\go-build-cache'
# PocketBase 0.36 recursively re-enters Collection.UnmarshalJSON under the
# Go 1.27 json/v2 implementation
$env:GOEXPERIMENT = 'nojsonv2'

New-Item -ItemType Directory -Force -Path $stageDir | Out-Null

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory)] [string]$Output,
        [Parameter(Mandatory)] [string]$Package,
        [string[]]$ExtraArgs = @()
    )
    $goArgs = @('build', '-trimpath', '-buildvcs=false') + $ExtraArgs +
        @('-ldflags', '-s -w', '-o', $Output, $Package)
    & go @goArgs
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $Package" }
}

Push-Location $repoRoot
try {
    Invoke-GoBuild -Output (Join-Path $stageDir 'beszel') -Package './internal/cmd/hub'
    Invoke-GoBuild -Output (Join-Path $stageDir 'beszel-agent') -Package './internal/cmd/agent'
    Invoke-GoBuild -Output (Join-Path $stageDir 'beszel-agent-glibc') -Package './internal/cmd/agent' -ExtraArgs @('-tags', 'glibc')
}
finally {
    Pop-Location
    Remove-Item Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
}

Copy-Item (Join-Path $PSScriptRoot 'README.txt') (Join-Path $stageDir 'README.txt') -Force

$goVersion = (& go version) -join ''
@(
    "run_id=cross-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
    "built_at=$([DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'))"
    'distribution=cross-compile from Windows (GOOS=linux GOARCH=amd64 CGO_ENABLED=0)'
    $goVersion
    'goos=linux'
    'goarch=amd64'
    "tag=$Tag"
) | Set-Content -LiteralPath (Join-Path $stageDir 'build-info.txt') -Encoding ascii

$binaries = @('beszel', 'beszel-agent', 'beszel-agent-glibc')
$sums = foreach ($name in $binaries) {
    $hash = (Get-FileHash -LiteralPath (Join-Path $stageDir $name) -Algorithm SHA256).Hash.ToLower()
    "$hash  $name"
}
$sums | Set-Content -LiteralPath (Join-Path $stageDir 'sha256sums.txt') -Encoding ascii

# tar via python tarfile: binaries 0755, texts 0644, root ownership
$pythonScript = @"
import tarfile, os
stage = r'$stageDir'
out = r'$tarPath'
top = 'oh-my-beszel-linux-amd64'
binaries = {'beszel', 'beszel-agent', 'beszel-agent-glibc'}
def norm(info):
    info.uid = info.gid = 0
    info.uname = info.gname = 'root'
    base = os.path.basename(info.name)
    info.mode = 0o755 if base in binaries else 0o644
    return info
with tarfile.open(out, 'w:gz') as tar:
    for name in sorted(os.listdir(stage)):
        tar.add(os.path.join(stage, name), arcname=f'{top}/{name}', filter=norm)
with tarfile.open(out) as tar:
    for member in tar.getmembers():
        print(oct(member.mode), member.name)
"@
& python -c $pythonScript
if ($LASTEXITCODE -ne 0) { throw 'tar packaging failed' }

Write-Host "Linux package ready: $tarPath"
