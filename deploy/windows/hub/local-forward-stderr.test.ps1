$ErrorActionPreference = 'Stop'
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('beszel-forward-stderr-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $scratch
$runner = $null
try {
    $sshProbe = Join-Path $scratch 'ssh-probe.ps1'
    [IO.File]::WriteAllText($sshProbe, @'
param([int]$ExitCode, [int]$ByteCount)
[Console]::Error.Write('E' * $ByteCount)
[Console]::Error.Flush()
exit $ExitCode
'@, [Text.UTF8Encoding]::new($false))
    $attemptProbe = Join-Path $scratch 'attempt-probe.ps1'
    [IO.File]::WriteAllText($attemptProbe, @'
param($PortScript, $DataDirectory, $SshProbe, $Name, [int]$ExitCode, [int]$ByteCount)
$ErrorActionPreference = 'Stop'
. $PortScript
function Test-LocalForwardOwned { return $true }
Invoke-LocalForwardAttempt -Name $Name -DefaultPort 47100 -DataDirectory $DataDirectory -Ssh (Get-Command pwsh.exe).Source -SshArgs @('-NoProfile', '-NonInteractive', '-File', $SshProbe, [string]$ExitCode, [string]$ByteCount) -Target 'test-only'
'@, [Text.UTF8Encoding]::new($false))
    foreach ($case in @(
        @{ Name = 'stderr-0'; ExitCode = 0; ByteCount = 200000 },
        @{ Name = 'stderr-7'; ExitCode = 7; ByteCount = 200000 },
        @{ Name = 'stderr-7'; ExitCode = 0; ByteCount = 17 },
        @{ Name = 'stderr-empty'; ExitCode = 0; ByteCount = 0 }
    )) {
        $name, $exitCode, $byteCount = $case.Name, $case.ExitCode, $case.ByteCount
        $startInfo = [Diagnostics.ProcessStartInfo]::new()
        $startInfo.FileName = (Get-Command pwsh.exe).Source
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        $startInfo.RedirectStandardOutput = $true
        $startInfo.RedirectStandardError = $true
        $startInfo.Environment['BESZEL_PORT_ALERT_TEST_MODE'] = '1'
        foreach ($argument in @('-NoProfile', '-NonInteractive', '-File', $attemptProbe, (Join-Path $PSScriptRoot 'local-ports.ps1'), $scratch, $sshProbe, $name, [string]$exitCode, [string]$byteCount)) {
            $startInfo.ArgumentList.Add($argument)
        }
        $runner = [Diagnostics.Process]::Start($startInfo)
        $stdout = $runner.StandardOutput.ReadToEndAsync()
        $stderr = $runner.StandardError.ReadToEndAsync()
        if (-not $runner.WaitForExit(15000)) { throw 'Tunnel attempt blocked while draining stderr' }
        $output = $stdout.GetAwaiter().GetResult()
        $errors = $stderr.GetAwaiter().GetResult()
        if ($runner.ExitCode -ne 0) { throw "Probe failed: $errors" }
        $logPath = Join-Path (Join-Path $scratch 'local-ports') "$name.stderr.log"
        if ((Get-Item -LiteralPath $logPath).Length -ne $byteCount -or [IO.File]::ReadAllText($logPath) -cne ('E' * $byteCount)) {
            throw 'SSH stderr was truncated or retained output from a previous attempt'
        }
        $assignment = Get-Content -Raw -LiteralPath (Join-Path (Split-Path -Parent $logPath) "$name.json") | ConvertFrom-Json
        if ($assignment.active) { throw 'Exited tunnel retained an active assignment' }
        if ($exitCode -eq 7 -and ($output -notlike '*SSH exited with code 7*' -or -not $output.Contains($logPath))) {
            throw 'SSH failure did not identify its diagnostic log'
        }
        if ($exitCode -eq 0 -and $output) { throw "Successful SSH process produced a warning: $output" }
        $runner.Dispose()
        $runner = $null
        Write-Output "${name}: SSH exit ${exitCode}, drained $byteCount stderr bytes and cleared active state."
    }
} finally {
    if ($runner) {
        if (-not $runner.HasExited) { $runner.Kill($true); $runner.WaitForExit() }
        $runner.Dispose()
    }
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($scratch)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-forward-stderr-*') {
        throw "Refusing to clean unexpected test directory: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
