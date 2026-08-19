# Idempotently registers monitored systems in the local hub via its REST API.
# Hub URL and admin credentials are read from config.json next to this script.
#
# One row per monitored system:
#   name  - display name in the dashboard
#   host/port - address of the agent (use 127.0.0.1 + a forwarded local port
#               when reaching the agent through an SSH tunnel)
#   token - the agent's KEY / fingerprint token set on the target machine
$ErrorActionPreference = 'Stop'
$PSDefaultParameterValues['Invoke-RestMethod:NoProxy'] = $true

$config = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'config.json') -Raw | ConvertFrom-Json
$baseUrl = "http://$($config.host):$($config.port)"

$authBody = @{
    identity = $config.hub.userEmail
    password = $config.hub.userPassword
} | ConvertTo-Json
$auth = Invoke-RestMethod -Method Post -Uri "$baseUrl/api/collections/users/auth-with-password" -ContentType 'application/json' -Body $authBody
$headers = @{ Authorization = $auth.token }
$info = Invoke-RestMethod -Method Get -Uri "$baseUrl/api/beszel/info" -Headers $headers

$systems = @(
    @{ name = 'gpu-node-1'; host = '10.0.0.11'; port = '45876'; token = '<agent-key-token>' }
    # @{ name = 'gpu-node-2'; host = '10.0.0.12'; port = '45876'; token = '<agent-key-token>' }
)

$created = foreach ($system in $systems) {
    $body = @{
        name  = $system.name
        host  = $system.host
        port  = $system.port
        pkey  = $info.key
        users = @($auth.record.id)
        status = 'pending'
    } | ConvertTo-Json

    $filter = [uri]EscapeDataString("name=`"$($system.name)`"")
    $existing = Invoke-RestMethod -Method Get -Uri "$baseUrl/api/collections/systems/records?filter=$filter" -Headers $headers
    if ($existing.totalItems -gt 0) {
        $record = Invoke-RestMethod -Method Patch -Uri "$baseUrl/api/collections/systems/records/$($existing.items[0].id)" -Headers $headers -ContentType 'application/json' -Body $body
    } else {
        $record = Invoke-RestMethod -Method Post -Uri "$baseUrl/api/collections/systems/records" -ContentType 'application/json' -Body $body -Headers $headers
    }

    $fingerprintBody = @{ system = $record.id; token = $system.token } | ConvertTo-Json
    $fingerprintFilter = [uri]::EscapeDataString("system=`"$($record.id)`"")
    $fingerprints = Invoke-RestMethod -Method Get -Uri "$baseUrl/api/collections/fingerprints/records?filter=$fingerprintFilter" -Headers $headers
    if ($fingerprints.totalItems -gt 0) {
        if ($fingerprints.items[0].token -ne $system.token) {
            $null = Invoke-RestMethod -Method Patch -Uri "$baseUrl/api/collections/fingerprints/records/$($fingerprints.items[0].id)" -Headers $headers -ContentType 'application/json' -Body $fingerprintBody
        }
    } else {
        $null = Invoke-RestMethod -Method Post -Uri "$baseUrl/api/collections/fingerprints/records" -ContentType 'application/json' -Body $fingerprintBody -Headers $headers
    }
    [pscustomobject]@{ Name = $system.name; Host = $system.host; Port = $system.port; Id = $record.id }
}

[pscustomobject]@{ PublicKey = $info.key; Systems = $created } | ConvertTo-Json -Depth 4
