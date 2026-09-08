param([switch]$Integration, [switch]$Browser)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if ($Integration -or $Browser) {
    foreach ($line in (Get-Content -LiteralPath (Join-Path $projectRoot '.env'))) {
        if ($Integration -and $line -match '^DATABASE_URL=(.*)$') { $env:TEST_DATABASE_URL = $matches[1] }
        if ($Browser -and $line -match '^(ADMIN_USERNAME|ADMIN_PASSWORD)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process') }
    }
}
Push-Location (Join-Path $projectRoot 'server')
try {
    go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { throw 'Server tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go static checks failed' }
} finally { Pop-Location }
Push-Location (Join-Path $projectRoot 'web')
try {
    pnpm build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    if ($Browser) {
        $env:E2E_BASE_URL = 'http://localhost:8081'
        pnpm exec playwright test
        if ($LASTEXITCODE -ne 0) { throw 'Browser tests failed' }
    }
} finally { Pop-Location }
