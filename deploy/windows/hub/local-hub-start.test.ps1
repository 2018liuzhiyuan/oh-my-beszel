param(
    [Parameter(Mandatory)]
    [string]$HubBinary,
    [string]$HubScript,
    [string]$MonitorBinary
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'local-ports.ps1')

$scratch = Join-Path ([IO.Path]::GetTempPath()) ('beszel-local-hub-start-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $scratch
$occupied = $null
$runner = $null
try {
    Copy-Item -LiteralPath $HubBinary -Destination (Join-Path $scratch 'beszel.exe')
    if ($MonitorBinary) {
        Copy-Item -LiteralPath $MonitorBinary -Destination (Join-Path $scratch 'Monitor.exe')
    } else {
        if (-not $HubScript) { throw 'Provide either HubScript or MonitorBinary' }
        Copy-Item -LiteralPath $HubScript -Destination (Join-Path $scratch 'run-hub.ps1')
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'local-ports.ps1') -Destination (Join-Path $scratch 'local-ports.ps1')
    }

    for ($candidate = 40000; $candidate -lt 45000; $candidate++) {
        if (-not (Test-LocalPortFree -Port $candidate) -or -not (Test-LocalPortFree -Port ($candidate + 1))) {
            continue
        }
        $occupied = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $candidate)
        $occupied.Start()
        break
    }
    if (-not $occupied) { throw 'Could not reserve a Hub port with a free successor' }
    $configPath = Join-Path $scratch 'config.json'
    [IO.File]::WriteAllText($configPath, (@{ host = '127.0.0.1'; port = $candidate; portRanges = @{ hub = @{ start = $candidate + 1; end = $candidate + 1 } } } | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = if ($MonitorBinary) { Join-Path $scratch 'Monitor.exe' } else { (Get-Command pwsh.exe).Source }
    $startInfo.WorkingDirectory = $scratch
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $arguments = if ($MonitorBinary) { @('hub') } else { @('-NoProfile', '-NonInteractive', '-File', (Join-Path $scratch 'run-hub.ps1')) }
    foreach ($argument in $arguments) {
        $startInfo.ArgumentList.Add($argument)
    }
    $runner = [Diagnostics.Process]::Start($startInfo)
    $stdout = $runner.StandardOutput.ReadToEndAsync()
    $stderr = $runner.StandardError.ReadToEndAsync()

    $updated = $false
    for ($attempt = 0; $attempt -lt 120; $attempt++) {
        if ($runner.HasExited) {
            $launcherLog = Join-Path $scratch 'launcher.log'
            $failureDetails = if (Test-Path -LiteralPath $launcherLog) { Get-Content -LiteralPath $launcherLog -Raw } else { '' }
            throw "Hub runner exited with code $($runner.ExitCode): $($stderr.GetAwaiter().GetResult()) $failureDetails"
        }
        $config = Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json
        if ($config.port -eq ($candidate + 1)) {
            try {
                $health = Invoke-RestMethod -NoProxy -Uri "http://127.0.0.1:$($config.port)/api/health" -TimeoutSec 2
                if ($health.code -eq 200) { $updated = $true; break }
            } catch {}
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $updated) {
        $hubLog = Join-Path $scratch 'hub.log'
        $details = if (Test-Path -LiteralPath $hubLog) { (Get-Content -LiteralPath $hubLog | Select-Object -Last 15) -join "`n" } else { '' }
        throw "Hub did not start on the automatically selected port (expected=$($candidate + 1), config.port=$($config.port)): $details"
    }
    Write-Output "Hub runner changed $candidate to $($candidate + 1), saved config.json, and served /api/health."
} finally {
    $ownedHubs = @(Get-Process -Name beszel -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq (Join-Path $scratch 'beszel.exe') })
    if ($runner) {
        if (-not $runner.HasExited) {
            $runner.Kill($true)
            $runner.WaitForExit()
        }
        $null = $stdout.GetAwaiter().GetResult()
        $null = $stderr.GetAwaiter().GetResult()
        $runner.Dispose()
    }
    foreach ($ownedHub in $ownedHubs) {
        if (-not $ownedHub.WaitForExit(5000)) { throw 'Scratch Hub process did not exit during cleanup' }
        $ownedHub.Dispose()
    }
    if ($occupied) { $occupied.Stop() }
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($scratch)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-local-hub-start-*') {
        throw "Refusing to clean unexpected test directory: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
