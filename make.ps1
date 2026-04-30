param(
    [Parameter(Position=0)]
    [ValidateSet("build","run","test","frontend","wails","wails-run","cross","tauri","tauri-dev","clean","all","help")]
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

function Ensure-WindowsResource {
    $manifest = Join-Path $PSScriptRoot "assets/app.manifest"
    $icon = Join-Path $PSScriptRoot "assets/logo_black.ico"
    $out = Join-Path $PSScriptRoot "resource_windows_amd64.syso"

    # Use the committed .syso as the source of truth. Only generate if missing.
    if (Test-Path $out) { return }

    $rsrc = Get-Command rsrc -ErrorAction SilentlyContinue
    if (-not $rsrc) { return }
    if ((Test-Path $manifest) -and (Test-Path $icon)) {
        & $rsrc.Source -manifest $manifest -ico $icon -arch amd64 -o $out
    }
}

function Build {
    Load-DotEnv
    Ensure-WindowsResource
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

    # Embed ESI credentials if present
    $buildLdFlags = $LdFlags
    if ($env:ESI_CLIENT_ID -and $env:ESI_CLIENT_SECRET) {
        $buildLdFlags += " -X main.esiClientID=$($env:ESI_CLIENT_ID) -X main.esiClientSecret=$($env:ESI_CLIENT_SECRET)"
        Write-Host "  ESI credentials embedded" -ForegroundColor Green
    }

    go build -ldflags $buildLdFlags -o "$BuildDir/$AppName.exe" .
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

function BuildWails {
    Load-DotEnv
    Ensure-WindowsResource
    Write-Host "Building frontend for Wails ($Version)..." -ForegroundColor Cyan
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install --silent 2>$null
    npm run build:wails
    $feExit = $LASTEXITCODE
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
    if ($feExit -ne 0) { Write-Host "Frontend Wails build failed!" -ForegroundColor Red; return }

    Write-Host "Building $AppName Wails desktop binary ($Version)..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
    $wailsLdFlags = "-s -w -H=windowsgui -X main.version=$Version"

    # Embed ESI credentials if present
    if ($env:ESI_CLIENT_ID -and $env:ESI_CLIENT_SECRET) {
        $wailsLdFlags += " -X main.esiClientID=$($env:ESI_CLIENT_ID) -X main.esiClientSecret=$($env:ESI_CLIENT_SECRET)"
        Write-Host "  ESI credentials embedded" -ForegroundColor Green
    }

    go build -tags "wails,production" -ldflags $wailsLdFlags -o "$BuildDir/$AppName-wails.exe" .
    if ($LASTEXITCODE -eq 0) { Write-Host "OK: $BuildDir/$AppName-wails.exe" -ForegroundColor Green }
}

function RunWails {
    BuildWails
    if ($LASTEXITCODE -eq 0) { & "./$BuildDir/$AppName-wails.exe" }
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

    # Embed ESI credentials if present
    $crossLdFlags = $LdFlags
    if ($env:ESI_CLIENT_ID -and $env:ESI_CLIENT_SECRET) {
        $crossLdFlags += " -X main.esiClientID=$($env:ESI_CLIENT_ID) -X main.esiClientSecret=$($env:ESI_CLIENT_SECRET)"
        Write-Host "  ESI credentials will be embedded in all binaries" -ForegroundColor Green
    }

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
        go build -ldflags $crossLdFlags -o $out .
    }

    # Reset env
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

    Write-Host "Done! Binaries in $BuildDir/" -ForegroundColor Green
}

function BuildTauri {
    # Build the Go backend as a Tauri sidecar binary with the correct target-triple name.
    # Tauri v2 requires: binaries/<name>-<target-triple>[.exe]
    Load-DotEnv

    $triple = "x86_64-pc-windows-msvc"
    $sidecarDir = "frontend/src-tauri/binaries"
    $sidecarName = "eve-flipper-backend-$triple.exe"

    Write-Host ""
    Write-Host "=== Step 1/3: Building frontend ===" -ForegroundColor Cyan
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install --silent 2>$null
    npm run build
    $feExit = $LASTEXITCODE
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
    if ($feExit -ne 0) { Write-Host "Frontend build failed!" -ForegroundColor Red; return }

    Write-Host ""
    Write-Host "=== Step 2/3: Building Go sidecar ===" -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $sidecarDir -Force | Out-Null

    # Embed ESI credentials if present
    $tauriLdFlags = $LdFlags
    if ($env:ESI_CLIENT_ID -and $env:ESI_CLIENT_SECRET) {
        $tauriLdFlags += " -X main.esiClientID=$($env:ESI_CLIENT_ID) -X main.esiClientSecret=$($env:ESI_CLIENT_SECRET)"
        Write-Host "  ESI credentials embedded" -ForegroundColor Green
    }

    $env:CGO_ENABLED = "0"
    go build -ldflags $tauriLdFlags -o "$sidecarDir/$sidecarName" .
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    if ($LASTEXITCODE -ne 0) { Write-Host "Go sidecar build failed!" -ForegroundColor Red; return }
    Write-Host "  Sidecar: $sidecarDir/$sidecarName" -ForegroundColor Green

    Write-Host ""
    Write-Host "=== Step 3/3: Building Tauri desktop app ===" -ForegroundColor Cyan
    Push-Location frontend
    npx tauri build 2>&1
    $tauriExit = $LASTEXITCODE
    Pop-Location

    if ($tauriExit -ne 0) {
        Write-Host "Tauri build failed!" -ForegroundColor Red
        return
    }

    # Create portable zip
    Write-Host ""
    Write-Host "=== Packaging portable zip ===" -ForegroundColor Cyan
    $tauriExe = "frontend/src-tauri/target/release/eve-flipper.exe"
    if (-not (Test-Path $tauriExe)) {
        # Try alternate name based on productName
        $tauriExe = "frontend/src-tauri/target/release/EVE Flipper.exe"
    }
    if (-not (Test-Path $tauriExe)) {
        Write-Host "Warning: Tauri exe not found at expected path. Check target/release/ manually." -ForegroundColor Yellow
        return
    }

    $portableDir = "$BuildDir/EVE-Flipper-windows-x64"
    $portableBin = "$portableDir/binaries"
    New-Item -ItemType Directory -Path $portableBin -Force | Out-Null
    Copy-Item $tauriExe "$portableDir/EVE Flipper.exe"
    Copy-Item "$sidecarDir/$sidecarName" "$portableBin/$sidecarName"

    $zipPath = "$BuildDir/EVE-Flipper-windows-x64.zip"
    if (Test-Path $zipPath) { Remove-Item $zipPath }
    Compress-Archive -Path $portableDir -DestinationPath $zipPath

    Write-Host ""
    Write-Host "Done! Portable package:" -ForegroundColor Green
    Write-Host "  $zipPath" -ForegroundColor Green
    Write-Host ""
    Write-Host "To run: unzip and double-click 'EVE Flipper.exe'" -ForegroundColor Yellow
    Write-Host "  (requires WebView2, pre-installed on Windows 10/11)" -ForegroundColor Yellow
}

function BuildTauriDev {
    # Quick dev mode: build Go sidecar, then run Tauri dev
    Load-DotEnv

    $triple = "x86_64-pc-windows-msvc"
    $sidecarDir = "frontend/src-tauri/binaries"
    $sidecarName = "eve-flipper-backend-$triple.exe"

    Write-Host "Building Go sidecar for dev..." -ForegroundColor Cyan

    # Build frontend first (Go embed needs it)
    Push-Location frontend
    $env:VITE_APP_VERSION = $Version
    npm install --silent 2>$null
    npm run build
    $feExit = $LASTEXITCODE
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
    if ($feExit -ne 0) { Write-Host "Frontend build failed!" -ForegroundColor Red; return }

    New-Item -ItemType Directory -Path $sidecarDir -Force | Out-Null

    # Embed ESI credentials if present
    $tauriDevLdFlags = $LdFlags
    if ($env:ESI_CLIENT_ID -and $env:ESI_CLIENT_SECRET) {
        $tauriDevLdFlags += " -X main.esiClientID=$($env:ESI_CLIENT_ID) -X main.esiClientSecret=$($env:ESI_CLIENT_SECRET)"
        Write-Host "  ESI credentials embedded" -ForegroundColor Green
    }

    $env:CGO_ENABLED = "0"
    go build -ldflags $tauriDevLdFlags -o "$sidecarDir/$sidecarName" .
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    if ($LASTEXITCODE -ne 0) { Write-Host "Go sidecar build failed!" -ForegroundColor Red; return }
    Write-Host "  Sidecar ready: $sidecarDir/$sidecarName" -ForegroundColor Green

    Write-Host "Starting Tauri dev..." -ForegroundColor Cyan
    Push-Location frontend
    npx tauri dev
    Pop-Location
}

function Clean {
    if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
    if (Test-Path "frontend/src-tauri/binaries") { Remove-Item -Recurse -Force "frontend/src-tauri/binaries" }
    if (Test-Path "frontend/src-tauri/target") { Remove-Item -Recurse -Force "frontend/src-tauri/target" }
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
  wails        Build Wails desktop variant (.exe)
  wails-run    Build and run Wails desktop variant
  cross        Cross-compile for Windows, Linux, macOS
  tauri        Build portable Tauri desktop app (Windows x64 zip)
  tauri-dev    Build sidecar + run Tauri in dev mode
  clean        Remove build artifacts (including Tauri target/)
  all          Test + frontend + cross-compile
  help         Show this help

ESI SSO Configuration:
  To embed ESI credentials in the built executable, set environment variables:
    `$env:ESI_CLIENT_ID = "your-client-id"`
    `$env:ESI_CLIENT_SECRET = "your-client-secret"`

  Before running any build command (build, run, wails, tauri, cross, etc.).
  If set, credentials will be embedded via ldflags at compile time.

  Register ESI application at: https://developers.eveonline.com/applications

  Alternatively, load from .env file in repo root:
    ESI_CLIENT_ID=your-client-id
    ESI_CLIENT_SECRET=your-client-secret
"@ -ForegroundColor Yellow
}

switch ($Command) {
    "build"     { Build }
    "run"       { Run }
    "test"      { Test }
    "frontend"  { Frontend }
    "wails"     { BuildWails }
    "wails-run" { RunWails }
    "cross"     { Cross }
    "tauri"     { BuildTauri }
    "tauri-dev" { BuildTauriDev }
    "clean"     { Clean }
    "all"       { Test; Frontend; Cross }
    "help"      { ShowHelp }
}
