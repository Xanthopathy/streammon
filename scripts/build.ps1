# Creates a build/ folder with cross-platform compiled binaries in separate folders, each with config files, then zips them

param(
    [switch]$Clean
)

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$BuildDir = Join-Path $ProjectRoot "build"
$CmdDir = Join-Path $ProjectRoot "cmd\streammon"
$ConfigsDir = Join-Path $ProjectRoot "configs"

# Define build targets: @{OS; Arch; OutputName; FolderName; ZipName}
$Targets = @(
    @{OS = "windows"; Arch = "amd64"; OutputName = "streammon.exe"; FolderName = "streammon-windows"; ZipName = "streammon-windows.zip"},
    @{OS = "linux"; Arch = "amd64"; OutputName = "streammon"; FolderName = "streammon-linux"; ZipName = "streammon-linux.zip"},
    @{OS = "darwin"; Arch = "amd64"; OutputName = "streammon-macos"; FolderName = "streammon-macos"; ZipName = "streammon-macos.zip"},
    @{OS = "darwin"; Arch = "arm64"; OutputName = "streammon-macos-arm64"; FolderName = "streammon-macos-arm64"; ZipName = "streammon-macos-arm64.zip"}
)

# Config files to include in each build
$ConfigFiles = @("streammon_config.toml", "streammon_config_yt.toml", "streammon_config_twitch.toml")

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
$BuiltPaths = @()

foreach ($Target in $Targets) {
    $OS = $Target.OS
    $Arch = $Target.Arch
    $OutputName = $Target.OutputName
    $FolderName = $Target.FolderName
    $ZipName = $Target.ZipName
    
    # Create platform-specific folder
    $PlatformBuildDir = Join-Path $BuildDir $FolderName
    if (-not (Test-Path $PlatformBuildDir)) {
        New-Item -ItemType Directory -Path $PlatformBuildDir | Out-Null
    }
    
    $OutputPath = Join-Path $PlatformBuildDir $OutputName
    
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
        continue
    }
    
    Write-Host "  Successfully built: $OutputName" -ForegroundColor Green
    
    # Copy config files to platform folder
    foreach ($ConfigFile in $ConfigFiles) {
        $SourcePath = Join-Path $ConfigsDir $ConfigFile
        if (Test-Path $SourcePath) {
            Copy-Item -Path $SourcePath -Destination $PlatformBuildDir -Force
            Write-Host "  Copied $ConfigFile" -ForegroundColor Green
        } else {
            Write-Host "  Warning: $ConfigFile not found in configs/" -ForegroundColor Yellow
        }
    }
    
    # Zip the platform folder with cross-platform path separators
    $ZipPath = Join-Path $BuildDir $ZipName
    Write-Host "  Creating $ZipName..." -ForegroundColor Cyan
    
    # Use .NET ZipArchive to ensure forward slashes in zip entries (cross-platform compatible)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    if (Test-Path $ZipPath) {
        Remove-Item $ZipPath -Force
    }
    
    $zip = [System.IO.Compression.ZipFile]::Open($ZipPath, 'Create')
    Get-ChildItem -Path $PlatformBuildDir -Recurse | ForEach-Object {
        if (-not $_.PSIsContainer) {
            $relPath = $_.FullName.Substring($PlatformBuildDir.Length + 1)
            $zipPath = "$FolderName/" + $relPath.Replace('\', '/')
            [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $_.FullName, $zipPath) | Out-Null
        }
    }
    $zip.Dispose()
    
    Write-Host "  Zipped: $ZipName" -ForegroundColor Green
    
    $BuiltPaths += $ZipPath
}

if ($AnyFailed) {
    Write-Host "`nSome builds failed!" -ForegroundColor Red
    exit 1
}

Write-Host "`nBuild complete! Output in: $BuildDir" -ForegroundColor Green
Write-Host "Zipped releases:" -ForegroundColor Cyan
Get-ChildItem $BuildDir -Filter "*.zip" | ForEach-Object { Write-Host "  - $($_.Name)" }
Write-Host "Platform folders:" -ForegroundColor Cyan
Get-ChildItem $BuildDir -Directory | ForEach-Object { Write-Host "  - $($_.Name)" }
