# Creates a build/ folder with the compiled exe and config files

param(
    [switch]$Clean
)

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$BuildDir = Join-Path $ProjectRoot "build"
$CmdDir = Join-Path $ProjectRoot "cmd\streammon"
$ConfigsDir = Join-Path $ProjectRoot "configs"
$ExeName = "streammon.exe"

Write-Host "Building StreamMon..." -ForegroundColor Cyan

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

# Build the executable
Write-Host "Compiling executable..." -ForegroundColor Cyan
Push-Location $CmdDir
go build -o (Join-Path $BuildDir $ExeName)
$buildResult = $LASTEXITCODE
Pop-Location

if ($buildResult -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}
Write-Host "Executable built successfully" -ForegroundColor Green

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
