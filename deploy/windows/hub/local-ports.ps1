function Get-LocalPortFile {
    param([string]$Name, [string]$DataDirectory)
    if ($Name -notmatch '^[A-Za-z0-9._-]+$' -or $Name -eq '..') {
        throw "Invalid local tunnel name: $Name"
    }
    return Join-Path (Join-Path $DataDirectory 'local-ports') "$Name.json"
}

function Get-LocalPortAssignment {
    param([string]$Name, [int]$DefaultPort, [string]$DataDirectory)
    $path = Get-LocalPortFile -Name $Name -DataDirectory $DataDirectory
    if (-not (Test-Path -LiteralPath $path)) {
        return $DefaultPort
    }
    try {
        $assignment = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json
        if ($assignment.name -eq $Name -and [int]$assignment.port -ge 1 -and [int]$assignment.port -le 65535) {
            return [int]$assignment.port
        }
    } catch {}
    return $DefaultPort
}

function Set-LocalPortAssignment {
    param([string]$Name, [int]$Port, [bool]$Active, [string]$DataDirectory)
    if ($Port -lt 1 -or $Port -gt 65535) {
        throw "Invalid local tunnel port: $Port"
    }
    $path = Get-LocalPortFile -Name $Name -DataDirectory $DataDirectory
    $directory = Split-Path -Parent $path
    $null = New-Item -ItemType Directory -Path $directory -Force
    $temporary = Join-Path $directory ("$Name." + [Guid]::NewGuid().ToString('N') + '.tmp')
    $backup = Join-Path $directory ("$Name." + [Guid]::NewGuid().ToString('N') + '.backup')
    $json = @{ name = $Name; port = $Port; active = $Active } | ConvertTo-Json -Compress
    try {
        [IO.File]::WriteAllText($temporary, $json, [Text.UTF8Encoding]::new($false))
        for ($attempt = 0; $attempt -lt 20; $attempt++) {
            try {
                if (Test-Path -LiteralPath $path) {
                    [IO.File]::Replace($temporary, $path, $backup)
                } else {
                    [IO.File]::Move($temporary, $path)
                }
                break
            } catch [IO.IOException] {
                $code = $_.Exception.GetBaseException().HResult -band 0xffff
                if ($code -notin @(32, 33) -or $attempt -eq 19) { throw }
                Start-Sleep -Milliseconds 100
            }
        }
    } catch [IO.IOException], [UnauthorizedAccessException] {
        $cause = $_.Exception.GetBaseException()
        throw [IO.IOException]::new("无法保存隧道端口记录 / Cannot save tunnel port assignment:`n$path`n$($cause.Message)", $cause)
    } finally {
        if (Test-Path -LiteralPath $backup) {
            Remove-Item -LiteralPath $backup -Force
        }
        if (Test-Path -LiteralPath $temporary) {
            Remove-Item -LiteralPath $temporary -Force
        }
    }
}

function Test-LocalPortFree {
    param([int]$Port)
    if ($Port -lt 1 -or $Port -gt 65535) {
        return $false
    }
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $Port)
    try {
        $listener.Start()
        return $true
    } catch [Net.Sockets.SocketException] {
        return $false
    } finally {
        $listener.Stop()
    }
}

function Get-ConfiguredPortRange {
    param(
        [string]$ConfigPath,
        [string]$Name,
        [int]$DefaultStart,
        [int]$DefaultEnd
    )
    if (-not $ConfigPath -or -not (Test-Path -LiteralPath $ConfigPath)) {
        return [pscustomobject]@{ Start = $DefaultStart; End = $DefaultEnd }
    }
    $config = Get-Content -Raw -LiteralPath $ConfigPath | ConvertFrom-Json
    $property = if ($config.portRanges) { $config.portRanges.PSObject.Properties[$Name] } else { $null }
    if (-not $property) {
        return [pscustomobject]@{ Start = $DefaultStart; End = $DefaultEnd }
    }
    try {
        if ($null -eq $property.Value.start -or $null -eq $property.Value.end) {
            throw 'Both start and end are required'
        }
        if ([string]$property.Value.start -notmatch '^\d+$' -or [string]$property.Value.end -notmatch '^\d+$') {
            throw 'Expected integer port numbers'
        }
        $start = [int]$property.Value.start
        $end = [int]$property.Value.end
        if ($start -lt 1 -or $end -gt 65535 -or $start -gt $end) {
            throw 'Expected 1 <= start <= end <= 65535'
        }
        return [pscustomobject]@{ Start = $start; End = $end }
    } catch {
        throw [ArgumentException]::new("config.json 中 portRanges.$Name.start/end 无效，需满足 1 <= start <= end <= 65535。`nInvalid portRanges.$Name.start/end in config.json: $($_.Exception.Message)")
    }
}

function Show-PortRangeAlert {
    param(
        [ValidateSet('hub', 'localTunnel')]
        [string]$Kind,
        [string]$Details,
        [string]$ConfigPath,
        [string]$DataDirectory,
        [Exception]$Exception,
        [string]$Title = 'Beszel 端口告警 / Port warning'
    )
    $failure = 'range'
    if ($Exception) {
        $cause = $Exception.GetBaseException()
        if ($cause -is [IO.IOException] -or $cause -is [UnauthorizedAccessException]) {
            $failure = 'file'
        } elseif ($cause -isnot [ArgumentException] -and $cause -isnot [InvalidOperationException]) {
            $failure = 'operation'
        }
    }
    $setting = "portRanges.$Kind.start/end"
    $advice = if ($Kind -eq 'hub') {
        "也可修改网页端口 port；保存后重新启动 Hub。`nYou can also change port; save and restart Hub."
    } else {
        "修改并保存后点击确定，隧道会自动重试。`nSave changes, then click OK; the tunnel retries automatically."
    }
    $message = "$Details`n`n请在 config.json 修改 / Edit in config.json:`n$setting`n扩大范围或设置一段空闲端口。`nExpand the range or choose a free port range.`n$advice`n`n配置文件 / Configuration file:`n$ConfigPath"
    if ($failure -eq 'file') {
        $Title = 'Beszel 文件告警 / File warning'
        $message = "$Title`n$Details`n`n这是文件访问错误，不是端口范围耗尽。`nThis is a file access error, not port range exhaustion.`n关闭占用文件的程序，并检查目录写入权限。`nClose programs locking the file; check folder write permissions.`n处理后点击确定，隧道会自动重试。`nResolve the file issue, then click OK; the tunnel retries automatically."
    } elseif ($failure -eq 'operation') {
        $Title = 'Beszel 隧道告警 / Tunnel warning'
        $message = "$Details`n`n隧道操作失败，请检查上述原因。`nTunnel operation failed; check the cause above.`n处理后点击确定，隧道会自动重试。`nResolve the issue, then click OK; the tunnel retries automatically."
    }
    if ($env:BESZEL_PORT_ALERT_TEST_MODE -eq '1') {
        return $message
    }
    $directory = Join-Path $DataDirectory 'port-alerts'
    $null = New-Item -ItemType Directory -Path $directory -Force
    $markerName = if ($failure -eq 'range') { $Kind } else { "$Kind-$failure" }
    $marker = Join-Path $directory "$markerName.txt"
    $mutex = [Threading.Mutex]::new($false, 'Local\BeszelPortRangeAlert')
    $locked = $false
    try {
        $locked = $mutex.WaitOne([TimeSpan]::FromSeconds(5))
        if (-not $locked) { return }
        if ((Test-Path -LiteralPath $marker) -and ((Get-Date) - (Get-Item -LiteralPath $marker).LastWriteTime).TotalMinutes -lt 5) {
            return
        }
        [IO.File]::WriteAllText($marker, ((Get-Date -Format o) + "`n" + $message), [Text.UTF8Encoding]::new($false))
    } finally {
        if ($locked) { $mutex.ReleaseMutex() }
        $mutex.Dispose()
    }
    try {
        $shell = New-Object -ComObject WScript.Shell
        $null = $shell.Popup($message, 0, $Title, 48)
    } catch {
        Add-Type -AssemblyName System.Windows.Forms
        $null = [Windows.Forms.MessageBox]::Show($message, $Title, [Windows.Forms.MessageBoxButtons]::OK, [Windows.Forms.MessageBoxIcon]::Warning)
    }
}

function Select-LocalForwardPort {
    param([string]$Name, [int]$DefaultPort, [string]$DataDirectory, [string]$ConfigPath)
    $range = Get-ConfiguredPortRange -ConfigPath $ConfigPath -Name 'localTunnel' -DefaultStart 47100 -DefaultEnd 47999
    $mutex = [Threading.Mutex]::new($false, 'Local\BeszelLocalForwardPortSelection')
    $locked = $false
    try {
        $locked = $mutex.WaitOne([TimeSpan]::FromSeconds(30))
        if (-not $locked) {
            throw 'Timed out waiting to select a local tunnel port'
        }
        $reserved = [Collections.Generic.HashSet[int]]::new()
        $directory = Join-Path $DataDirectory 'local-ports'
        if (Test-Path -LiteralPath $directory) {
            foreach ($file in Get-ChildItem -LiteralPath $directory -Filter '*.json' -File) {
                if ($file.Name -eq "$Name.json") { continue }
                try {
                    $other = Get-Content -Raw -LiteralPath $file.FullName | ConvertFrom-Json
                    if ([int]$other.port -ge 1 -and [int]$other.port -le 65535) {
                        $null = $reserved.Add([int]$other.port)
                    }
                } catch {}
            }
        }
        $preferred = Get-LocalPortAssignment -Name $Name -DefaultPort $DefaultPort -DataDirectory $DataDirectory
        foreach ($candidate in @($preferred, $DefaultPort)) {
            if ($reserved.Contains($candidate) -or -not (Test-LocalPortFree -Port $candidate)) { continue }
            Set-LocalPortAssignment -Name $Name -Port $candidate -Active $false -DataDirectory $DataDirectory
            return $candidate
        }
        for ($candidate = $range.Start; $candidate -le $range.End; $candidate++) {
            if ($reserved.Contains($candidate) -or -not (Test-LocalPortFree -Port $candidate)) { continue }
            Set-LocalPortAssignment -Name $Name -Port $candidate -Active $false -DataDirectory $DataDirectory
            return $candidate
        }
        throw [InvalidOperationException]::new("隧道 $Name 的首选端口及范围 $($range.Start)-$($range.End) 均不可用。`nNo free local tunnel port for $Name in $($range.Start)-$($range.End), including preferred ports.")
    } finally {
        if ($locked) { $mutex.ReleaseMutex() }
        $mutex.Dispose()
    }
}

function Test-LocalForwardOwned {
    param([int]$Port, [int]$ProcessId)
    $listeners = Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    return [bool](@($listeners | Where-Object OwningProcess -eq $ProcessId).Count)
}

function Invoke-LocalForwardAttempt {
    param(
        [string]$Name,
        [int]$DefaultPort,
        [string]$DataDirectory,
        [string]$ConfigPath,
        [string]$Ssh,
        [string[]]$SshArgs,
        [string]$Target
    )
    try {
        $port = Select-LocalForwardPort -Name $Name -DefaultPort $DefaultPort -DataDirectory $DataDirectory -ConfigPath $ConfigPath
    } catch {
        Show-PortRangeAlert -Kind 'localTunnel' -Details $_.Exception.Message -Exception $_.Exception -ConfigPath $ConfigPath -DataDirectory $DataDirectory
        return
    }
    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $Ssh
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardError = $true
    foreach ($argument in $SshArgs + @('-N', '-L', "127.0.0.1:${port}:127.0.0.1:45876", $Target)) {
        $startInfo.ArgumentList.Add($argument)
    }
    $active = $false
    $process = $null
    $stderrLog = $null
    $stderrCopy = $null
    $failure = $null
    try {
        $stderrPath = [IO.Path]::ChangeExtension((Get-LocalPortFile -Name $Name -DataDirectory $DataDirectory), '.stderr.log')
        $stderrLog = [IO.File]::Open($stderrPath, [IO.FileMode]::Create, [IO.FileAccess]::Write, [IO.FileShare]::Read)
        $process = [Diagnostics.Process]::Start($startInfo)
        $stderrCopy = $process.StandardError.BaseStream.CopyToAsync($stderrLog)
        $deadline = (Get-Date).AddSeconds(45)
        while (-not $process.HasExited -and (Get-Date) -lt $deadline) {
            if (Test-LocalForwardOwned -Port $port -ProcessId $process.Id) {
                Set-LocalPortAssignment -Name $Name -Port $port -Active $true -DataDirectory $DataDirectory
                $active = $true
                break
            }
            Start-Sleep -Milliseconds 500
        }
        if ($active) {
            $process.WaitForExit()
        } elseif (-not $process.HasExited) {
            $process.Kill()
            $process.WaitForExit()
        }
        if ($process.ExitCode -ne 0) {
            throw "SSH exited with code $($process.ExitCode). See $stderrPath"
        }
    } catch {
        $failure = $_
    } finally {
        if ($process) {
            if (-not $process.HasExited) { $process.Kill(); $process.WaitForExit() }
            try {
                if ($stderrCopy) { $null = $stderrCopy.GetAwaiter().GetResult() }
            } catch {
                if (-not $failure) { $failure = $_ }
            }
            $process.Dispose()
        }
        if ($stderrLog) { $stderrLog.Dispose() }
        try {
            Set-LocalPortAssignment -Name $Name -Port $port -Active $false -DataDirectory $DataDirectory
        } catch {
            if (-not $failure) { $failure = $_ }
        }
    }
    if ($failure) {
        Show-PortRangeAlert -Kind 'localTunnel' -Details $failure.Exception.Message -Exception $failure.Exception -ConfigPath $ConfigPath -DataDirectory $DataDirectory
    }
}
