$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'local-ports.ps1')
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('beszel-forward-failures-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $scratch
$previousAlertMode = $env:BESZEL_PORT_ALERT_TEST_MODE
$logLock = $null
try {
    $env:BESZEL_PORT_ALERT_TEST_MODE = '1'
    $missingSsh = Join-Path $scratch 'missing-ssh.exe'
    $alert = Invoke-LocalForwardAttempt -Name 'missing-ssh' -DefaultPort 47100 -DataDirectory $scratch -Ssh $missingSsh -Target 'test-only'
    if ($alert -notlike '*Tunnel operation failed*' -or -not $alert.Contains($missingSsh) -or $alert -like '*portRanges*') {
        throw "Missing SSH executable was not reported as an operation failure: $alert"
    }
    $path = Get-LocalPortFile -Name 'missing-ssh' -DataDirectory $scratch
    $assignment = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json
    if ($assignment.active) { throw 'Failed process launch retained an active assignment' }
    $logPath = [IO.Path]::ChangeExtension($path, '.stderr.log')
    $exclusiveLog = [IO.File]::Open($logPath, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $exclusiveLog.Dispose()
    Write-Output 'Failed SSH launch: accurate warning, inactive state, and released log handle.'

    Set-LocalPortAssignment -Name 'locked-log' -Port 47101 -Active $false -DataDirectory $scratch
    $path = Get-LocalPortFile -Name 'locked-log' -DataDirectory $scratch
    $logPath = [IO.Path]::ChangeExtension($path, '.stderr.log')
    [IO.File]::WriteAllText($logPath, 'previous diagnostics')
    $logLock = [IO.File]::Open($logPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    $alert = Invoke-LocalForwardAttempt -Name 'locked-log' -DefaultPort 47101 -DataDirectory $scratch -Ssh $missingSsh -Target 'test-only'
    if ($alert -notlike '*File warning*' -or -not $alert.Contains($logPath) -or $alert -like '*portRanges*') {
        throw "Locked stderr log was not reported as a file failure: $alert"
    }
    if ([IO.File]::ReadAllText($logPath) -ne 'previous diagnostics') { throw 'Locked stderr log was overwritten' }
    $assignment = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json
    if ($assignment.active) { throw 'Locked stderr log retained an active assignment' }
    $logLock.Dispose()
    $logLock = $null
    $alert = Invoke-LocalForwardAttempt -Name 'locked-log' -DefaultPort 47101 -DataDirectory $scratch -Ssh $missingSsh -Target 'test-only'
    if ($alert -notlike '*Tunnel operation failed*' -or -not $alert.Contains($missingSsh)) {
        throw 'Releasing the log lock did not allow the next launch attempt'
    }
    $exclusiveLog = [IO.File]::Open($logPath, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $exclusiveLog.Dispose()
    Write-Output 'Locked stderr log: preserved diagnostics, inactive state, and recovery after unlock.'
} finally {
    if ($logLock) { $logLock.Dispose() }
    if ($null -eq $previousAlertMode) {
        Remove-Item Env:BESZEL_PORT_ALERT_TEST_MODE -ErrorAction SilentlyContinue
    } else {
        $env:BESZEL_PORT_ALERT_TEST_MODE = $previousAlertMode
    }
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($scratch)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-forward-failures-*') {
        throw "Refusing to clean unexpected test directory: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
