$ErrorActionPreference = 'Stop'

Unregister-ScheduledTask -TaskName 'Beszel Hub' -Confirm:$false
Write-Host 'Scheduled task removed. Stop the running hub with: Stop-Process -Name beszel -ErrorAction SilentlyContinue'
