# Сборка GoNews под Windows.
#
# Примеры:
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Version 1.2.0 -Output bin\gonews.exe
#   .\scripts\build.ps1 -Goos linux -Goarch arm64 -Output bin\gonews-linux-arm64

param(
    [string]$Version = "",
    [string]$Output = "bin\gonews.exe",
    [string]$Goos = "windows",
    [string]$Goarch = "amd64"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Git may be unavailable or the directory may not be a repository yet.
$gitOk = $true
$prevEap = $ErrorActionPreference
$ErrorActionPreference = "SilentlyContinue"
try {
    if (-not $Version) {
        $Version = (git describe --tags --always --dirty 2>$null)
    }
    $commit = (git rev-parse --short HEAD 2>$null)
} catch {
    $gitOk = $false
} finally {
    $ErrorActionPreference = $prevEap
}

if (-not $Version) { $Version = "dev" }
if (-not $commit) { $commit = "none" }
if (-not $gitOk) { $commit = "none" }

$buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$ldflags = @(
    "-s", "-w",
    "-X", "github.com/gonews/gonews/internal/version.Version=$Version",
    "-X", "github.com/gonews/gonews/internal/version.Commit=$commit",
    "-X", "github.com/gonews/gonews/internal/version.BuildDate=$buildDate"
) -join " "

$outDir = Split-Path -Parent $Output
if ($outDir -and -not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}

Write-Host "Building GoNews $Version ($Goos/$Goarch) -> $Output" -ForegroundColor Cyan
$env:CGO_ENABLED = "0"
$env:GOOS = $Goos
$env:GOARCH = $Goarch

go build -trimpath -ldflags $ldflags -o $Output ./cmd/server
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}

$size = [math]::Round((Get-Item $Output).Length / 1MB, 2)
Write-Host "OK: $Output ($size MB)" -ForegroundColor Green
