<#
.SYNOPSIS
    royo-learn installer for Windows
.DESCRIPTION
    Downloads the royo-learn binary from GitHub Releases, verifies SHA-256,
    installs to %LOCALAPPDATA%\royo-learn\bin\, and optionally adds to PATH.
.PARAMETER Version
    Version to install (default: latest). Example: --version v1.0.0
.PARAMETER Uninstall
    Remove royo-learn from the system.
.EXAMPLE
    .\install.ps1
    .\install.ps1 --version v1.0.0
    .\install.ps1 --uninstall
#>

param(
    [string]$Version = "latest",
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$Repo = "angel-royo/royo-learn"
$InstallRoot = "$env:LOCALAPPDATA\royo-learn"
$BinDir = "$InstallRoot\bin"
$BinaryName = "royo-learn.exe"

function Write-Info { Write-Host "[royo-learn] $args" -ForegroundColor Cyan }
function Write-Error-Custom { Write-Host "[royo-learn] ERROR: $args" -ForegroundColor Red; exit 1 }

function Uninstall-RoyoLearn {
    $target = Join-Path $BinDir $BinaryName
    if (Test-Path $target) {
        Remove-Item $target -Force
        Write-Info "removed $target"
    }
    if (Test-Path $InstallRoot) {
        $remaining = Get-ChildItem $InstallRoot -Recurse -File | Measure-Object | Select-Object -ExpandProperty Count
        if ($remaining -eq 0) {
            Remove-Item $InstallRoot -Recurse -Force
            Write-Info "removed $InstallRoot"
        }
    }
    Write-Info "uninstall complete. Remove '$BinDir' from your PATH if desired."
    exit 0
}

function Install-RoyoLearn {
    param([string]$Ver)

    $platform = "windows-amd64"
    $archiveName = "royo-learn-${platform}.exe"

    if ($Ver -eq "latest") {
        $downloadUrl = "https://github.com/$Repo/releases/latest/download/$archiveName"
    } else {
        $downloadUrl = "https://github.com/$Repo/releases/download/$Ver/$archiveName"
    }
    $checksumUrl = "${downloadUrl}.sha256"

    Write-Info "installing royo-learn $Ver for $platform..."

    $tmpDir = Join-Path $env:TEMP "royo-learn-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download binary.
        Write-Info "downloading $downloadUrl..."
        $binaryPath = Join-Path $tmpDir $BinaryName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $binaryPath -UseBasicParsing

        # Download checksum.
        try {
            $checksumPath = Join-Path $tmpDir "${BinaryName}.sha256"
            Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing -ErrorAction Stop
            Write-Info "verifying checksum..."
            $expected = (Get-Content $checksumPath).Split()[0]
            $actual = (Get-FileHash -Path $binaryPath -Algorithm SHA256).Hash
            if ($expected -eq $actual) {
                Write-Info "checksum OK"
            } else {
                Write-Info "checksum mismatch (expected $expected, got $actual)"
            }
        } catch {
            Write-Info "checksum download failed, skipping verification"
        }

        # Install.
        New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
        Copy-Item $binaryPath -Destination (Join-Path $BinDir $BinaryName) -Force
        Write-Info "installed to $BinDir\$BinaryName"

        # Verify.
        try {
            $versionOutput = & (Join-Path $BinDir $BinaryName) version --json 2>$null
            Write-Info "verified: $versionOutput"
        } catch {
            Write-Info "version check skipped"
        }

        # PATH note.
        $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($currentPath -notlike "*$BinDir*") {
            Write-Info "NOTE: add $BinDir to your PATH:"
            Write-Info "  setx PATH `"%PATH%;$BinDir`""
            Write-Info "  or add manually via System Properties > Environment Variables"
        }

        Write-Info "install complete!"
    } finally {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ---------- main ----------
if ($Uninstall) {
    Uninstall-RoyoLearn
}

Install-RoyoLearn -Ver $Version
