param(
    [Parameter(Mandatory)]
    [string]$HubBinary
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'local-ports.ps1')

$scratch = Join-Path ([IO.Path]::GetTempPath()) ('beszel-local-port-sync-' + [Guid]::NewGuid().ToString('N'))
$dataDirectory = Join-Path $scratch 'data'
$null = New-Item -ItemType Directory -Path $dataDirectory -Force
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$httpPort = ([Net.IPEndPoint]$listener.LocalEndpoint).Port
$listener.Stop()
$baseUrl = "http://127.0.0.1:$httpPort"
$email = 'local-port-sync@example.com'
$password = 'Beszel-Local-Port-Test-2026!'
$hubProcess = $null

try {
    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = (Resolve-Path -LiteralPath $HubBinary).Path
    $startInfo.WorkingDirectory = $scratch
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($argument in @('--dir', $dataDirectory, 'serve', '--http', "127.0.0.1:$httpPort")) {
        $startInfo.ArgumentList.Add($argument)
    }
    $startInfo.Environment['BESZEL_HUB_USER_EMAIL'] = $email
    $startInfo.Environment['BESZEL_HUB_USER_PASSWORD'] = $password
    $startInfo.Environment['BESZEL_HUB_APP_URL'] = $baseUrl
    $startInfo.Environment['BESZEL_HUB_CHECK_UPDATES'] = 'false'
    $hubProcess = [Diagnostics.Process]::Start($startInfo)
    $stdout = $hubProcess.StandardOutput.ReadToEndAsync()
    $stderr = $hubProcess.StandardError.ReadToEndAsync()

    $healthy = $false
    for ($attempt = 0; $attempt -lt 120; $attempt++) {
        if ($hubProcess.HasExited) {
            throw "Scratch Hub exited with code $($hubProcess.ExitCode)"
        }
        try {
            $health = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/health" -TimeoutSec 2
            if ($health.code -eq 200) { $healthy = $true; break }
        } catch {}
        Start-Sleep -Milliseconds 250
    }
    if (-not $healthy) { throw 'Scratch Hub did not become healthy' }

    $authBody = @{ identity = $email; password = $password } | ConvertTo-Json -Compress
    $auth = Invoke-RestMethod -NoProxy -Method Post -Uri "$baseUrl/api/collections/users/auth-with-password" -ContentType 'application/json' -Body $authBody
    $headers = @{ Authorization = $auth.token }
    $unrelatedSystems = @()
    foreach ($target in @(@{host = '192.0.2.10'; ssh_config = ''}, @{host = '127.0.0.1'; ssh_config = 'test-only-config'})) {
        $body = @{
            name = 'local-port-probe'; host = $target.host; ssh_config = $target.ssh_config
            port = '47076'; users = @($auth.record.id); status = 'paused'
        } | ConvertTo-Json -Compress
        $unrelatedSystems += Invoke-RestMethod -NoProxy -Method Post -Uri "$baseUrl/api/collections/systems/records" -Headers $headers -ContentType 'application/json' -Body $body
    }
    $systemBody = @{
        name = 'local-port-probe'
        host = '127.0.0.1'
        port = '47077'
        users = @($auth.record.id)
        status = 'paused'
    } | ConvertTo-Json -Compress
    $system = Invoke-RestMethod -NoProxy -Method Post -Uri "$baseUrl/api/collections/systems/records" -Headers $headers -ContentType 'application/json' -Body $systemBody

    Set-LocalPortAssignment -Name 'local-port-probe' -Port 47101 -Active $false -DataDirectory $dataDirectory
    Start-Sleep -Seconds 6
    $record = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/collections/systems/records/$($system.id)" -Headers $headers
    if ($record.port -ne '47077') { throw 'Inactive tunnel assignment changed the system port' }

    Set-LocalPortAssignment -Name 'local-port-probe' -Port 47101 -Active $true -DataDirectory $dataDirectory
    $updated = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        Start-Sleep -Milliseconds 500
        $record = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/collections/systems/records/$($system.id)" -Headers $headers
        if ($record.port -eq '47101') {
            $updated = $true
            break
        }
    }
    if (-not $updated -or $record.status -ne 'paused') {
        throw "Active tunnel assignment was not applied correctly (port=$($record.port), status=$($record.status))"
    }
    foreach ($unrelated in $unrelatedSystems) {
        $record = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/collections/systems/records/$($unrelated.id)" -Headers $headers
        if ($record.port -ne '47076') { throw 'Same-name remote or SSH system was modified' }
    }
    $duplicateBody = @{
        name = 'local-port-probe'; host = '127.0.0.1'; port = '47078'
        users = @($auth.record.id); status = 'paused'
    } | ConvertTo-Json -Compress
    $duplicate = Invoke-RestMethod -NoProxy -Method Post -Uri "$baseUrl/api/collections/systems/records" -Headers $headers -ContentType 'application/json' -Body $duplicateBody
    Set-LocalPortAssignment -Name 'local-port-probe' -Port 47102 -Active $true -DataDirectory $dataDirectory
    Start-Sleep -Seconds 6
    $originalRecord = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/collections/systems/records/$($system.id)" -Headers $headers
    $duplicateRecord = Invoke-RestMethod -NoProxy -Uri "$baseUrl/api/collections/systems/records/$($duplicate.id)" -Headers $headers
    if ($originalRecord.port -ne '47101' -or $duplicateRecord.port -ne '47078') {
        throw 'Ambiguous local tunnel assignment modified a system'
    }
    Write-Output 'Scratch Hub applied only the unique active local assignment; inactive, unrelated and ambiguous systems were preserved.'
} finally {
    if ($hubProcess) {
        if (-not $hubProcess.HasExited) {
            $hubProcess.Kill($true)
            $hubProcess.WaitForExit()
        }
        $null = $stdout.GetAwaiter().GetResult()
        $null = $stderr.GetAwaiter().GetResult()
        $hubProcess.Dispose()
    }
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($scratch)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-local-port-sync-*') {
        throw "Refusing to clean unexpected test directory: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
