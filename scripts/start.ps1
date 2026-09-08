param([switch]$Demo, [switch]$Build)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if ($Build -or -not (Test-Path -LiteralPath (Join-Path $projectRoot 'server/wallplanet.exe'))) {
    & (Join-Path $PSScriptRoot 'build.ps1')
}
if ($Demo) {
    $env:DEMO_MODE = 'true'
    $env:APP_ADDR = '127.0.0.1:8081'
    $env:APP_ORIGIN = 'http://localhost:8081'
} else {
    $env:DEMO_MODE = 'false'
    $env:APP_ADDR = '127.0.0.1:8080'
    $env:APP_ORIGIN = 'http://localhost:8080'
}
Push-Location (Join-Path $projectRoot 'server')
try { & './wallplanet.exe' } finally { Pop-Location }
