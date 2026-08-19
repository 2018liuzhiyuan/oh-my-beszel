$ErrorActionPreference = 'Stop'

Set-Location -LiteralPath $PSScriptRoot
$config = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'config.json') -Raw | ConvertFrom-Json

$listenHost = if ($config.host) { [string]$config.host } else { '127.0.0.1' }
$port = if ($config.port) { [int]$config.port } else { 8090 }

$env:APP_URL = "http://${listenHost}:${port}"
if ($config.hub.userEmail)    { $env:USER_EMAIL = $config.hub.userEmail }
if ($config.hub.userPassword) { $env:USER_PASSWORD = $config.hub.userPassword }
if ($config.hub.autoLogin)    { $env:AUTO_LOGIN = $config.hub.autoLogin }
if ($null -ne $config.hub.checkUpdates) { $env:CHECK_UPDATES = if ([bool]$config.hub.checkUpdates) { 'true' } else { 'false' } }
if ($config.sshConfigPath)    { $env:SSH_CONFIG_PATH = $config.sshConfigPath }

& (Join-Path $PSScriptRoot 'beszel.exe') serve --http "${listenHost}:${port}" *>> (Join-Path $PSScriptRoot 'hub.log')
exit $LASTEXITCODE
