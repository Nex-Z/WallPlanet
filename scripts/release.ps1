param([string]$Tag = (Get-Date -Format 'yyyyMMdd-HHmmss'))
$ErrorActionPreference = 'Stop'
if ($Tag -notmatch '^[a-zA-Z0-9][a-zA-Z0-9._-]*$') { throw 'Invalid release tag' }
$root = Split-Path $PSScriptRoot -Parent
$output = Join-Path $root "releases/$Tag"
if (Test-Path -LiteralPath $output) { throw 'Release directory already exists; use a new tag' }
New-Item -ItemType Directory -Path "$output/web" -Force | Out-Null
$oldOS = $env:GOOS; $oldArch = $env:GOARCH; $oldCGO = $env:CGO_ENABLED
Push-Location $root
try {
    pnpm --dir web build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    Copy-Item web/dist/* "$output/web" -Recurse
    Copy-Item deploy/Dockerfile.runtime "$output/Dockerfile"
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Push-Location server
    try {
        go build -trimpath -ldflags='-s -w' -o "$output/wallplanet" .
        if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    } finally { Pop-Location }
    $archive = Join-Path $root "releases/wallplanet-$Tag-linux-amd64.tar.gz"
    tar -czf $archive -C $output .
    if ($LASTEXITCODE -ne 0) { throw 'Archive creation failed' }
    Get-FileHash $archive -Algorithm SHA256
} finally {
    $env:GOOS = $oldOS; $env:GOARCH = $oldArch; $env:CGO_ENABLED = $oldCGO
    Pop-Location
}
