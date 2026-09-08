param([switch]$Demo, [switch]$CreateAdmin)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $projectRoot '.env'
if (-not (Test-Path -LiteralPath $envFile)) {
    Copy-Item -LiteralPath (Join-Path $projectRoot '.env.example') -Destination $envFile
    $keyBytes = New-Object byte[] 32
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    $generator.GetBytes($keyBytes)
    $generator.Dispose()
    $config = [IO.File]::ReadAllText($envFile).Replace('MASTER_KEY=', 'MASTER_KEY=' + [Convert]::ToBase64String($keyBytes))
    [IO.File]::WriteAllText($envFile, $config, [Text.UTF8Encoding]::new($false))
    Write-Host 'Created .env. Fill DATABASE_URL (and DEMO_DATABASE_URL if needed), then run this script again.'
    exit 0
}
$env:DEMO_MODE = if ($Demo) { 'true' } else { 'false' }
Push-Location (Join-Path $projectRoot 'server')
try {
    go run . init-db
    if ($LASTEXITCODE -ne 0) { throw 'Database initialization failed' }
    go run . migrate
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed' }
    if ($Demo) {
        go run . seed-demo
        if ($LASTEXITCODE -ne 0) { throw 'Demo initialization failed' }
    }
    if ($CreateAdmin) {
        if (-not $env:ADMIN_USERNAME) { $env:ADMIN_USERNAME = Read-Host 'Administrator username (3-32 characters)' }
        $adminSecret = Read-Host 'Administrator password (10-128 characters)' -AsSecureString
        $env:ADMIN_PASSWORD = [Net.NetworkCredential]::new('', $adminSecret).Password
        try {
            go run . create-admin
            if ($LASTEXITCODE -ne 0) { throw 'Administrator creation failed' }
        } finally { Remove-Item Env:ADMIN_PASSWORD -ErrorAction SilentlyContinue }
    }
} finally { Pop-Location }
