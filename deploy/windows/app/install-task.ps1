$ErrorActionPreference = 'Stop'

$taskName = 'Beszel Hub'
$runner = Join-Path $PSScriptRoot 'run-hub.ps1'
$taskRunner = Join-Path $PSScriptRoot 'Monitor.exe'
$action = New-ScheduledTaskAction `
    -Execute $taskRunner `
    -Argument "task `"$runner`"" `
    -WorkingDirectory $PSScriptRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet `
    -RestartCount 10 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -Hidden

Register-ScheduledTask `
    -TaskName $taskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Description 'Run the local Beszel monitoring hub.' `
    -Force | Out-Null
Start-ScheduledTask -TaskName $taskName
Write-Host "Scheduled task '$taskName' registered and started (runs at logon)."
