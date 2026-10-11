param([Parameter(Mandatory)][string]$ConfigPath, [string]$PortScript = (Join-Path $PSScriptRoot 'local-ports.ps1'))

$ErrorActionPreference = 'Stop'
. $PortScript
$config = Get-Content -Raw -LiteralPath $ConfigPath | ConvertFrom-Json
$range = $config.portRanges.localTunnel
$demoDirectory = Join-Path ([IO.Path]::GetTempPath()) ('beszel-port-alert-demo-' + [Guid]::NewGuid().ToString('N'))
$details = "【模拟 / Demo】隧道端口范围 $($range.start)-$($range.end) 全部不可用。`nTunnel port range $($range.start)-$($range.end) is unavailable (simulation).`n仅演示：不占用端口，无需修改配置。`nDemo only: no ports occupied; no configuration changes needed.`n弹窗不会自动关闭，请点击确定。`nThis warning stays open until you click OK."
Show-PortRangeAlert -Kind 'localTunnel' -Details $details -ConfigPath $ConfigPath -DataDirectory $demoDirectory -Title 'Beszel 端口告警 / Port warning (Demo)'
