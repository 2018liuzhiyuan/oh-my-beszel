$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'local-ports.ps1')

$testDirectory = Join-Path ([IO.Path]::GetTempPath()) ('beszel-local-ports-test-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $testDirectory
try {
    Add-Type @'
using System.IO;
using System.Threading;
using System.Threading.Tasks;
public static class PortAssignmentReadLock {
    public static Task ReleaseAfter(FileStream stream, int milliseconds) {
        return Task.Run(() => { Thread.Sleep(milliseconds); stream.Dispose(); });
    }
}
'@
    Set-LocalPortAssignment -Name 'read-lock' -Port 47100 -Active $false -DataDirectory $testDirectory
    $lockedPath = Get-LocalPortFile -Name 'read-lock' -DataDirectory $testDirectory
    $readLock = [IO.File]::Open($lockedPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    $release = [PortAssignmentReadLock]::ReleaseAfter($readLock, 350)
    try {
        Set-LocalPortAssignment -Name 'read-lock' -Port 47101 -Active $true -DataDirectory $testDirectory
    } finally { $null = $release.GetAwaiter().GetResult(); $readLock.Dispose() }
    $updated = Get-Content -Raw -LiteralPath $lockedPath | ConvertFrom-Json
    if ($updated.port -ne 47101 -or -not $updated.active) { throw 'Transient read lock prevented atomic port assignment update' }
    Write-Output 'Transient file lock: update retried and succeeded.'

    $readLock = [IO.File]::Open($lockedPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    $env:BESZEL_PORT_ALERT_TEST_MODE = '1'
    $timer = [Diagnostics.Stopwatch]::StartNew()
    try {
        $alert = Invoke-LocalForwardAttempt -Name 'read-lock' -DefaultPort 47101 -DataDirectory $testDirectory
        if ($alert -notlike '*文件告警*' -or $alert -notlike '*File warning*' -or $alert -notlike '*read-lock.json*' -or $alert -notlike '*click OK*' -or $alert -like '*portRanges*' -or $alert -like '*扩大范围*') {
            throw "Persistent file lock was misreported as port exhaustion: $alert"
        }
        if ($timer.Elapsed.TotalSeconds -gt 8) { throw 'File-lock retry was not bounded' }
        $unchanged = Get-Content -Raw -LiteralPath $lockedPath | ConvertFrom-Json
        if ($unchanged.port -ne 47101 -or -not $unchanged.active) { throw 'Failed write corrupted the previous assignment' }
        if (@(Get-ChildItem -LiteralPath (Split-Path -Parent $lockedPath) -Filter '*.tmp').Count) { throw 'Failed write left temporary files behind' }
    } finally { $readLock.Dispose(); Remove-Item Env:BESZEL_PORT_ALERT_TEST_MODE -ErrorAction SilentlyContinue }
    Set-LocalPortAssignment -Name 'read-lock' -Port 47101 -Active $false -DataDirectory $testDirectory
    Write-Output 'Persistent file lock: bounded retry, preserved state, accurate warning, and recovery passed.'

    $probeScript = Join-Path $testDirectory 'tunnel-process-probe.ps1'
    $probePidFile = Join-Path $testDirectory 'tunnel-process-probe.pid'
    [IO.File]::WriteAllText($probeScript, @'
param([string]$PidPath)
[IO.File]::WriteAllText($PidPath, [string]$PID)
Start-Sleep -Seconds 60
'@, [Text.UTF8Encoding]::new($false))
    $originalOwnershipCheck = (Get-Item Function:Test-LocalForwardOwned).ScriptBlock
    $script:publishReadLock = $null
    $env:BESZEL_PORT_ALERT_TEST_MODE = '1'
    try {
        Set-Item Function:Test-LocalForwardOwned -Value {
            param([int]$Port, [int]$ProcessId)
            if (-not (Test-Path -LiteralPath $probePidFile)) { return $false }
            $path = Get-LocalPortFile -Name 'publish-lock' -DataDirectory $testDirectory
            $script:publishReadLock = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
            return $true
        }
        $alert = Invoke-LocalForwardAttempt -Name 'publish-lock' -DefaultPort 47102 -DataDirectory $testDirectory -Ssh (Get-Command pwsh.exe).Source -SshArgs @('-NoProfile', '-NonInteractive', '-File', $probeScript, $probePidFile) -Target 'test-only'
        if ($alert -notlike '*File warning*' -or $alert -like '*portRanges*') { throw 'Active-state write failure was not reported as a file error' }
        $probeProcessId = [int](Get-Content -Raw -LiteralPath $probePidFile)
        if (Get-Process -Id $probeProcessId -ErrorAction SilentlyContinue) { throw 'State publication failure left its tunnel process running' }
        $assignment = Get-Content -Raw -LiteralPath (Get-LocalPortFile -Name 'publish-lock' -DataDirectory $testDirectory) | ConvertFrom-Json
        if ($assignment.active) { throw 'Failed state publication left an active assignment' }
    } finally {
        if ($script:publishReadLock) { $script:publishReadLock.Dispose() }
        Set-Item Function:Test-LocalForwardOwned -Value $originalOwnershipCheck
        Remove-Item Env:BESZEL_PORT_ALERT_TEST_MODE -ErrorAction SilentlyContinue
    }
    Write-Output 'State publication failure: tunnel process cleaned up before accurate warning.'

    $customHub = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $customHub.Start()
    $customHubPort = ([Net.IPEndPoint]$customHub.LocalEndpoint).Port
    $customForward = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $customForward.Start()
    $customForwardPort = ([Net.IPEndPoint]$customForward.LocalEndpoint).Port
    $configPath = Join-Path $testDirectory 'config.json'
    @{ portRanges = @{ hub = @{ start = $customHubPort; end = $customHubPort }; localTunnel = @{ start = $customForwardPort; end = $customForwardPort } } } |
        ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $configPath -Encoding utf8
    try {
        $hubRange = Get-ConfiguredPortRange -ConfigPath $configPath -Name 'hub' -DefaultStart 8090 -DefaultEnd 8190
        if ($hubRange.Start -ne $customHubPort -or $hubRange.End -ne $customHubPort) { throw 'Configured Hub range was not read correctly' }
        try {
            $null = Select-LocalForwardPort -Name 'custom-full' -DefaultPort $customForwardPort -DataDirectory $testDirectory -ConfigPath $configPath
            throw 'Expected occupied custom tunnel range to fail'
        } catch {
            if ($_.Exception.Message -notlike '*No free local tunnel port*') { throw }
        }
        $env:BESZEL_PORT_ALERT_TEST_MODE = '1'
        $alert = Show-PortRangeAlert -Kind 'localTunnel' -Details 'Range occupied' -ConfigPath $configPath -DataDirectory $testDirectory
        if ($alert -notlike '*portRanges.localTunnel.start/end*' -or $alert -notlike '*config.json*' -or $alert -notlike '*扩大范围*' -or $alert -notlike '*Expand the range*' -or $alert -notlike '*自动重试*' -or $alert -notlike '*click OK*retries automatically*') {
            throw 'Tunnel alert did not identify the configuration to change'
        }
        $hubAlert = Show-PortRangeAlert -Kind 'hub' -Details 'Range occupied' -ConfigPath $configPath -DataDirectory $testDirectory
        if ($hubAlert -notlike '*portRanges.hub.start/end*' -or $hubAlert -notlike '*重新启动 Hub*' -or $hubAlert -notlike '*save and restart Hub*') {
            throw 'Hub alert did not identify the configuration to change'
        }
    } finally {
        Remove-Item Env:BESZEL_PORT_ALERT_TEST_MODE -ErrorAction SilentlyContinue
        $customHub.Stop()
        $customForward.Stop()
    }

    foreach ($invalidRange in @(@{start=0; end=10}, @{start=10; end=1}, @{start=1; end=65536}, @{start=1.5; end=10}, @{start=1})) {
        @{ portRanges = @{ hub = $invalidRange } } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $configPath -Encoding utf8
        try {
            $null = Get-ConfiguredPortRange -ConfigPath $configPath -Name 'hub' -DefaultStart 8090 -DefaultEnd 8190
            throw 'Expected invalid range to fail'
        } catch {
            if ($_.Exception.Message -notlike '*Invalid portRanges.hub.start/end*') { throw }
        }
    }

    $originalPortCheck = (Get-Item Function:Test-LocalPortFree).ScriptBlock
    try {
        Set-Item Function:Test-LocalPortFree -Value { param([int]$Port) $Port -eq 47082 }
        @{ portRanges = @{ localTunnel = @{ start=47082; end=47082 } } } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $configPath -Encoding utf8
        $configuredPort = Select-LocalForwardPort -Name 'configured-range' -DefaultPort 40000 -DataDirectory $testDirectory -ConfigPath $configPath
        if ($configuredPort -ne 47082) { throw 'Configured range was restricted by legacy hardcoded port exclusions' }
    } finally {
        Set-Item Function:Test-LocalPortFree -Value $originalPortCheck
    }

    $forwardPort = 40000
    while ($forwardPort -lt 45000 -and (-not (Test-LocalPortFree -Port $forwardPort) -or -not (Test-LocalPortFree -Port ($forwardPort + 1)))) {
        $forwardPort++
    }
    if ($forwardPort -ge 45000) { throw 'Could not find two free consecutive test ports' }
    $occupiedPort = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $forwardPort)
    $occupiedPort.Start()
    @{ portRanges = @{ localTunnel = @{ start = $forwardPort + 1; end = $forwardPort + 1 } } } |
        ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $configPath -Encoding utf8
    try {
        $chosenForward = Select-LocalForwardPort -Name 'successor' -DefaultPort $forwardPort -DataDirectory $testDirectory -ConfigPath $configPath
        if ($chosenForward -ne ($forwardPort + 1)) {
            throw "Expected tunnel port $($forwardPort + 1), got $chosenForward"
        }
    } finally {
        $occupiedPort.Stop()
    }

    $occupiedForward = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $occupiedForward.Start()
    try {
        $preferred = ([Net.IPEndPoint]$occupiedForward.LocalEndpoint).Port
        $first = Select-LocalForwardPort -Name 'test-one' -DefaultPort $preferred -DataDirectory $testDirectory
        $second = Select-LocalForwardPort -Name 'test-two' -DefaultPort $preferred -DataDirectory $testDirectory
        if ($first -lt 47100 -or $first -gt 47999 -or $second -lt 47100 -or $second -gt 47999 -or $first -eq $second) {
            throw "Forward ports were not distinct values in 47100-47999: $first, $second"
        }
        Set-LocalPortAssignment -Name 'test-one' -Port $first -Active $true -DataDirectory $testDirectory
        $assignment = Get-Content -Raw -LiteralPath (Get-LocalPortFile -Name 'test-one' -DataDirectory $testDirectory) | ConvertFrom-Json
        if ($assignment.port -ne $first -or -not $assignment.active) {
            throw 'Published local port assignment is incorrect'
        }
    } finally {
        $occupiedForward.Stop()
    }
    Write-Output "Configured tunnel fallback: $forwardPort -> $chosenForward; default fallbacks: $first, $second"
} finally {
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($testDirectory)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-local-ports-test-*') {
        throw "Refusing to clean unexpected test directory: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
