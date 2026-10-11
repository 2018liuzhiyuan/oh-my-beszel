param([string]$EvidenceDirectory, [string]$MonitorBinary, [string]$HubBinary, [ValidateRange(0, 60)][int]$PersistenceSeconds = 0)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class PortAlertWindow {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr window, IntPtr context, uint flags);
    [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr window, uint message, IntPtr wParam, IntPtr lParam);
}
'@
$null = [PortAlertWindow]::SetProcessDPIAware()
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('beszel-port-alert-ui-' + [Guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $scratch
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = ([Net.IPEndPoint]$listener.LocalEndpoint).Port
$probe = Join-Path $scratch 'probe.ps1'
$configPath = Join-Path $scratch 'config.json'
$process = $null
try {
    if ($EvidenceDirectory) { $null = New-Item -ItemType Directory -Path $EvidenceDirectory -Force }
    @{ port = $port; portRanges = @{ hub = @{ start = $port; end = $port }; localTunnel = @{ start = $port; end = $port } } } |
        ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $configPath -Encoding utf8
    [IO.File]::WriteAllText($probe, @'
param($PortScript, $ConfigPath, $DataDirectory, $Kind, [int]$Port)
$ErrorActionPreference = 'Stop'
. $PortScript
if ($Kind -eq 'stateFile') {
    $probeListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $probeListener.Start()
    $freePort = ([Net.IPEndPoint]$probeListener.LocalEndpoint).Port
    $probeListener.Stop()
    Set-LocalPortAssignment -Name 'ui-lock' -Port $freePort -Active $false -DataDirectory $DataDirectory
    $path = Get-LocalPortFile -Name 'ui-lock' -DataDirectory $DataDirectory
    $readLock = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    try { Invoke-LocalForwardAttempt -Name 'ui-lock' -DefaultPort $freePort -DataDirectory $DataDirectory -ConfigPath $ConfigPath }
    finally { $readLock.Dispose() }
} elseif ($Kind -eq 'localTunnel') {
    Invoke-LocalForwardAttempt -Name 'ui-probe' -DefaultPort $Port -DataDirectory $DataDirectory -ConfigPath $ConfigPath
}
'@, [Text.UTF8Encoding]::new($false))

    $cases = @('localTunnel', 'stateFile')
    if ($MonitorBinary) {
        if (-not $HubBinary) { throw 'HubBinary is required to test native Monitor' }
        Copy-Item -LiteralPath $MonitorBinary -Destination (Join-Path $scratch 'Monitor.exe')
        Copy-Item -LiteralPath $HubBinary -Destination (Join-Path $scratch 'beszel.exe')
        $cases += 'nativeHub'
    }
    foreach ($kind in $cases) {
        $native = $kind -eq 'nativeHub'
        $rangeName = if ($native) { 'hub' } else { $kind }
        $title = if ($native) { 'Beszel Hub 端口告警 / Port warning' } elseif ($kind -eq 'stateFile') { 'Beszel 文件告警 / File warning' } else { 'Beszel 端口告警 / Port warning' }
        $startInfo = [Diagnostics.ProcessStartInfo]::new()
        $startInfo.FileName = if ($native) { Join-Path $scratch 'Monitor.exe' } else { (Get-Command pwsh.exe).Source }
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        $null = $startInfo.Environment.Remove('BESZEL_PORT_ALERT_TEST_MODE')
        $null = $startInfo.Environment.Remove('BESZEL_MONITOR_NO_MSGBOX')
        $arguments = if ($native) { @('hub') } else { @('-NoProfile', '-NonInteractive', '-File', $probe, (Join-Path $PSScriptRoot 'local-ports.ps1'), $configPath, $scratch, $kind, [string]$port) }
        foreach ($argument in $arguments) {
            $startInfo.ArgumentList.Add($argument)
        }
        $process = [Diagnostics.Process]::Start($startInfo)
        $condition = [Windows.Automation.AndCondition]::new(
            [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ProcessIdProperty, $process.Id),
            [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty, $title)
        )
        $window = $null
        $deadline = (Get-Date).AddSeconds(15)
        while (-not $window -and -not $process.HasExited -and (Get-Date) -lt $deadline) {
            $window = [Windows.Automation.AutomationElement]::RootElement.FindFirst([Windows.Automation.TreeScope]::Children, $condition)
            if (-not $window) { Start-Sleep -Milliseconds 200 }
        }
        if (-not $window -or $window.Current.IsOffscreen) { throw "$kind warning did not appear on the interactive desktop" }
        $windowHandle = [IntPtr]$window.Current.NativeWindowHandle
        if (-not [PortAlertWindow]::IsWindowVisible($windowHandle)) { throw "$kind warning window was hidden" }
        Start-Sleep -Milliseconds 500
        if ($kind -eq 'hub' -and $PersistenceSeconds -gt 0) {
            Start-Sleep -Seconds $PersistenceSeconds
            if ($process.HasExited -or -not [PortAlertWindow]::IsWindowVisible($windowHandle)) {
                throw 'Warning closed before acknowledgement'
            }
            Write-Output "Warning remained visible for $PersistenceSeconds seconds without acknowledgement."
        }
        $controls = $window.FindAll([Windows.Automation.TreeScope]::Descendants, [Windows.Automation.Condition]::TrueCondition)
        $text = ($controls | ForEach-Object { $_.Current.Name }) -join "`n"
        if ($kind -eq 'stateFile') {
            foreach ($expected in @('无法保存隧道端口记录', 'Cannot save tunnel port assignment', 'ui-lock.json', '不是端口范围耗尽', 'not port range exhaustion', '写入权限', 'write permissions', 'click OK', 'retries automatically')) {
                if (-not $text.Contains($expected)) { throw "File warning missing guidance: $expected" }
            }
            if ($text -like '*portRanges*' -or $text -like '*扩大范围*' -or $text -like '*Edit in config.json*') { throw 'File error was misreported as port exhaustion' }
        } elseif ($text -notlike "*portRanges.$rangeName.start/end*" -or $text -notlike '*config.json*' -or $text -notlike '*扩大范围*' -or $text -notlike '*Expand the range*' -or $text -notlike '*Edit in config.json*') {
            throw "$kind warning did not explain the settings to change"
        }
        if ($kind -eq 'localTunnel') {
            if ($text -notlike '*自动重试*' -or $text -notlike '*click OK*retries automatically*' -or $text -notlike '*隧道*均不可用*' -or $text -notlike '*No free local tunnel port*') { throw 'Tunnel warning did not explain the cause and retry in both languages' }
        } elseif ($kind -ne 'stateFile' -and ($text -notlike '*重新启动 Hub*' -or $text -notlike '*save and restart Hub*' -or $text -notlike '*Hub 端口*均不可用*' -or $text -notlike '*Hub port*fallback range*')) {
            throw 'Hub warning did not explain the cause and restart in both languages'
        }
        if ($EvidenceDirectory) {
            $rectangle = $window.Current.BoundingRectangle
            $bitmap = [Drawing.Bitmap]::new([int]$rectangle.Width, [int]$rectangle.Height)
            $graphics = [Drawing.Graphics]::FromImage($bitmap)
            try {
                $context = $graphics.GetHdc()
                try {
                    if (-not [PortAlertWindow]::PrintWindow($windowHandle, $context, 2)) { throw 'Could not capture the warning window' }
                } finally { $graphics.ReleaseHdc($context) }
                $bitmap.Save((Join-Path $EvidenceDirectory "$kind-popup.png"), [Drawing.Imaging.ImageFormat]::Png)
                [IO.File]::WriteAllText((Join-Path $EvidenceDirectory "$kind-popup.txt"), $text, [Text.UTF8Encoding]::new($false))
            } finally {
                $graphics.Dispose()
                $bitmap.Dispose()
            }
        }
        $button = $controls | Where-Object { $_.Current.Name -in @('确定', 'OK') } | Select-Object -First 1
        if (-not $button) { throw "$kind warning had no acknowledgement button" }
        try {
            ([Windows.Automation.InvokePattern]$button.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)).Invoke()
        } catch {
            $buttonHandle = [IntPtr]$button.Current.NativeWindowHandle
            if ($buttonHandle -eq [IntPtr]::Zero) { throw }
            $null = [PortAlertWindow]::SendMessage($buttonHandle, 0xF5, [IntPtr]::Zero, [IntPtr]::Zero)
        }
        $expectedExitCode = if ($native) { 8 } else { 0 }
        if (-not $process.WaitForExit(5000) -or $process.ExitCode -ne $expectedExitCode) { throw "$kind probe did not finish after acknowledgement" }
        $process.Dispose()
        if (-not $native) {
            $process = [Diagnostics.Process]::Start($startInfo)
            if (-not $process.WaitForExit(5000) -or $process.ExitCode -ne 0) { throw "$kind repeated warning was not throttled" }
            $process.Dispose()
        }
        $process = $null
        Write-Output "${kind}: visible warning, exact configuration guidance, and acknowledgement passed."
    }
} finally {
    if ($process) {
        if (-not $process.HasExited) { $process.Kill($true); $process.WaitForExit() }
        $process.Dispose()
    }
    $listener.Stop()
    $temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $resolved = [IO.Path]::GetFullPath($scratch)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolved) -notlike 'beszel-port-alert-ui-*') { throw "Refusing to clean unexpected directory: $resolved" }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
