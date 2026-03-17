# Creates a build/ folder with cross-platform compiled binaries and config files

param(
    [switch]$Clean
)

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$BuildDir = Join-Path $ProjectRoot "build"
$CmdDir = Join-Path $ProjectRoot "cmd\streammon"
$ConfigsDir = Join-Path $ProjectRoot "configs"

# Define build targets: @{OS; Arch; OutputName}
$Targets = @(
    @{OS = "windows"; Arch = "amd64"; OutputName = "streammon.exe"},
    @{OS = "linux"; Arch = "amd64"; OutputName = "streammon"},
    @{OS = "darwin"; Arch = "amd64"; OutputName = "streammon-macos"},
    @{OS = "darwin"; Arch = "arm64"; OutputName = "streammon-macos-arm64"}
)

Write-Host "Building StreamMon for multiple platforms..." -ForegroundColor Cyan

# Clean previous build if requested
if ($Clean -and (Test-Path $BuildDir)) {
    Write-Host "Cleaning previous build..." -ForegroundColor Yellow
    Remove-Item $BuildDir -Recurse -Force
}

# Create build directory
if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
    Write-Host "Created build directory" -ForegroundColor Green
}

# Build executables for each target
$AnyFailed = $false
foreach ($Target in $Targets) {
    $OS = $Target.OS
    $Arch = $Target.Arch
    $OutputName = $Target.OutputName
    $OutputPath = Join-Path $BuildDir $OutputName
    
    Write-Host "Compiling for $OS/$Arch ($OutputName)..." -ForegroundColor Cyan
    
    Push-Location $CmdDir
    $env:GOOS = $OS
    $env:GOARCH = $Arch
    go build -o $OutputPath
    $buildResult = $LASTEXITCODE
    [Environment]::SetEnvironmentVariable('GOOS', '')
    [Environment]::SetEnvironmentVariable('GOARCH', '')
    Pop-Location
    
    if ($buildResult -ne 0) {
        Write-Host "  Failed to build for $OS/$Arch!" -ForegroundColor Red
        $AnyFailed = $true
    } else {
        Write-Host "  Successfully built: $OutputName" -ForegroundColor Green
    }
}

if ($AnyFailed) {
    Write-Host "`nSome builds failed!" -ForegroundColor Red
    exit 1
}
Write-Host "`nAll executables built successfully" -ForegroundColor Green

# Copy config files
$ConfigFiles = @("streammon_config.toml", "streammon_config_yt.toml", "streammon_config_twitch.toml")
foreach ($ConfigFile in $ConfigFiles) {
    $SourcePath = Join-Path $ConfigsDir $ConfigFile
    if (Test-Path $SourcePath) {
        Copy-Item -Path $SourcePath -Destination $BuildDir -Force
        Write-Host "Copied $ConfigFile" -ForegroundColor Green
    } else {
        Write-Host "Warning: $ConfigFile not found in configs/" -ForegroundColor Yellow
    }
}

Write-Host "`nBuild complete! Output in: $BuildDir" -ForegroundColor Green
Write-Host "Files:" -ForegroundColor Cyan
Get-ChildItem $BuildDir | ForEach-Object { Write-Host "  - $($_.Name)" }
