# Run a GoNews k6 scenario on Windows.
# Usage: .\run.ps1 smoke|read|mixed|stress|spike
param(
    [ValidateSet('smoke', 'read', 'mixed', 'stress', 'spike')]
    [string]$Scenario = 'smoke',
    [string]$BaseUrl = $(if ($env:BASE_URL) { $env:BASE_URL } else { 'http://localhost:8080' })
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command k6 -ErrorAction SilentlyContinue)) {
    Write-Error 'k6 is not installed. See https://k6.io/docs/get-started/installation/'
}

$script = Join-Path $PSScriptRoot "$Scenario.js"
if (-not (Test-Path $script)) {
    Write-Error "Scenario not found: $script"
}

$env:BASE_URL = $BaseUrl
Write-Host "GoNews load test: $Scenario against $BaseUrl"
k6 run $script
