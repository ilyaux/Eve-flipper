param(
    [Parameter(Position=0)]
    [ValidateSet("build","run","test","frontend","cross","clean","all","help")]
    [string]$Command = "help"
)

$AppName  = "eve-flipper"
$BuildDir = "build"
$Version  = & git describe --tags --always --dirty 2>$null
if (-not $Version) { $Version = "dev" }
$LdFlags  = "-s -w -X main.version=$Version"

function Load-DotEnv {
    # Load variables from .env in repo root (if present) into the current process
    $envPath = Join-Path $PSScriptRoot ".env"
    if (Test-Path $envPath) {
        Get-Content $envPath | ForEach-Object {
            $line = $_.Trim()
            if (-not $line -or $line.StartsWith("#")) { return }
            if ($line -notmatch "=") { return }
            $parts = $line.Split("=", 2)
            $key = $parts[0].Trim()
            $value = $parts[1].Trim()
            if ($key) {
                # Set environment variable for current process
                Set-Item -Path "Env:$key" -Value $value
            }
        }
    }
}

function Build {
    Load-DotEnv
    Write-Host "Building frontend ($Version)..." -ForegroundColor Cyan
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install --silent 2>$null
    npm run build
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
    if ($LASTEXITCODE -ne 0) { Write-Host "Frontend build failed!" -ForegroundColor Red; return }

    Write-Host "Building $AppName ($Version)..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
    go build -ldflags $LdFlags -o "$BuildDir/$AppName.exe" .
    if ($LASTEXITCODE -eq 0) { Write-Host "OK: $BuildDir/$AppName.exe" -ForegroundColor Green }
}

function Run {
    Build
    if ($LASTEXITCODE -eq 0) { & "./$BuildDir/$AppName.exe" }
}

function Test {
    Write-Host "Running tests..." -ForegroundColor Cyan
    go test ./...
}

function Frontend {
    Write-Host "Building frontend ($Version)..." -ForegroundColor Cyan
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install
    npm run build
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
}

function Cross {
    Load-DotEnv
    Write-Host "Building frontend ($Version)..." -ForegroundColor Cyan
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install
    npm run build
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
    if ($LASTEXITCODE -ne 0) { return }
    Write-Host "Cross-compiling $AppName ($Version)..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null

    $targets = @(
        @{ GOOS="windows"; GOARCH="amd64"; Ext=".exe" },
        @{ GOOS="linux";   GOARCH="amd64"; Ext="" },
        @{ GOOS="linux";   GOARCH="arm64"; Ext="" },
        @{ GOOS="darwin";  GOARCH="amd64"; Ext="" },
        @{ GOOS="darwin";  GOARCH="arm64"; Ext="" }
    )

    foreach ($t in $targets) {
        $out = "$BuildDir/$AppName-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
        Write-Host "  $($t.GOOS)/$($t.GOARCH) -> $out"
        $env:GOOS   = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $env:CGO_ENABLED = "0"
        go build -ldflags $LdFlags -o $out .
    }

    # Reset env
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

    Write-Host "Done! Binaries in $BuildDir/" -ForegroundColor Green
}

function Clean {
    if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
    Write-Host "Cleaned." -ForegroundColor Green
}

function ShowHelp {
    Write-Host @"
Usage: .\make.ps1 <command>

Commands:
  build        Build frontend + backend into single .exe (Go embeds frontend)
  run          Build and run the backend
  test         Run all Go tests
  frontend     Install deps and build frontend
  cross        Cross-compile for Windows, Linux, macOS
  clean        Remove build artifacts
  all          Test + frontend + cross-compile
  help         Show this help
"@ -ForegroundColor Yellow
}

switch ($Command) {
    "build"    { Build }
    "run"      { Run }
    "test"     { Test }
    "frontend" { Frontend }
    "cross"    { Cross }
    "clean"    { Clean }
    "all"      { Test; Frontend; Cross }
    "help"     { ShowHelp }
}
