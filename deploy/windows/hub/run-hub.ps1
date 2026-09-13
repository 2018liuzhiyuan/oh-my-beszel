$ErrorActionPreference = 'Stop'

Set-Location -LiteralPath $PSScriptRoot
# the user-facing config.json lives one level up, beside Monitor.exe
$config = Get-Content -LiteralPath (Join-Path (Split-Path $PSScriptRoot -Parent) 'config.json') -Raw | ConvertFrom-Json

$listenHost = if ($config.host) { [string]$config.host } else { '127.0.0.1' }
$port = if ($config.port) { [int]$config.port } else { 8090 }

$logPath = Join-Path $PSScriptRoot 'hub.log'
# rotate once the log grows beyond 10 MB so a long-running hub cannot fill the disk
if ((Get-Item -LiteralPath $logPath -ErrorAction SilentlyContinue)?.Length -gt 10MB) {
    Move-Item -LiteralPath $logPath -Destination (Join-Path $PSScriptRoot 'hub.log.1') -Force
}

$env:APP_URL = "http://${listenHost}:${port}"
if ($config.hub.userEmail)    { $env:USER_EMAIL = $config.hub.userEmail }
if ($config.hub.userPassword) { $env:USER_PASSWORD = $config.hub.userPassword }
if ($config.hub.autoLogin)    { $env:AUTO_LOGIN = $config.hub.autoLogin }
if ($null -ne $config.hub.checkUpdates) { $env:CHECK_UPDATES = if ([bool]$config.hub.checkUpdates) { 'true' } else { 'false' } }
if ($config.sshConfigPath)    { $env:SSH_CONFIG_PATH = $config.sshConfigPath }
if ($config.hub.logLevel)     { $env:BESZEL_LOG_LEVEL = $config.hub.logLevel }

& (Join-Path $PSScriptRoot 'beszel.exe') serve --http "${listenHost}:${port}" *>> $logPath
exit $LASTEXITCODE
