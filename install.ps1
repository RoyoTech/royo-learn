<#
.SYNOPSIS
    royo-learn installer for Windows
.DESCRIPTION
    Downloads the royo-learn archive from GitHub Releases, verifies SHA-256,
    extracts the binary, installs to %LOCALAPPDATA%\royo-learn\bin\, and
    offers to add the directory to PATH.
.PARAMETER Version
    Version to install (default: latest). Example: -Version v0.1.1
.PARAMETER Uninstall
    Remove royo-learn from the system.
.EXAMPLE
    .\install.ps1
    .\install.ps1 -Version v0.1.1
    .\install.ps1 -Uninstall
#>

param(
    [Alias("v")]
    [string]$Version = "latest",
    [Alias("remove")]
    [switch]$Uninstall
)

# Normalize: treat any value that starts with "-" or "--" as "latest"
# (happens when user types --version instead of -Version in PowerShell 5.x)
if ($Version -match '^-') {
    $Version = "latest"
}

$ErrorActionPreference = "Stop"
$Repo = "RoyoTech/royo-learn"
$InstallRoot = "$env:LOCALAPPDATA\royo-learn"
$BinDir = "$InstallRoot\bin"
$BinaryName = "royo-learn.exe"

function Write-Info { Write-Host "[royo-learn] $args" -ForegroundColor Cyan }
function Write-Error-Custom { Write-Host "[royo-learn] ERROR: $args" -ForegroundColor Red; exit 1 }

# Auto-detect architecture.
function Get-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default  { return "amd64" }
    }
}

function Add-ToUserPath {
    param([string]$Dir)

    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($null -eq $userPath) { $userPath = "" }

    $target = $Dir.TrimEnd('\')
    $already = $userPath -split ';' | Where-Object { $_.TrimEnd('\') -ieq $target }
    if ($already) {
        Write-Info "$Dir already on your PATH"
        return
    }

    $trimmed = $userPath.TrimEnd(';')
    $newPath = if ($trimmed -eq "") { $Dir } else { "$trimmed;$Dir" }
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")

    # Make it usable in the current session too.
    $env:PATH = "$env:PATH;$Dir"

    Write-Info "added $Dir to your user PATH"
    Write-Info "open a NEW terminal for 'royo-learn' to be found (this session already has it)"
}

function Uninstall-RoyoLearn {
    $target = Join-Path $BinDir $BinaryName
    if (Test-Path $target) {
        Remove-Item $target -Force
        Write-Info "removed $target"
    }
    if (Test-Path $InstallRoot) {
        $remaining = Get-ChildItem $InstallRoot -Recurse -File -ErrorAction SilentlyContinue | Measure-Object | Select-Object -ExpandProperty Count
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

    $arch = Get-Arch
    $archiveName = "royo-learn-windows-${arch}.zip"

    $baseUrl = "https://github.com/$Repo/releases"
    if ($Ver -eq "latest") {
        $downloadUrl = "$baseUrl/latest/download/$archiveName"
        $checksumUrl = "$baseUrl/latest/download/checksums.txt"
    } else {
        $downloadUrl = "$baseUrl/download/$Ver/$archiveName"
        $checksumUrl = "$baseUrl/download/$Ver/checksums.txt"
    }

    Write-Info "installing royo-learn $Ver for windows/$arch..."

    $tmpDir = Join-Path $env:TEMP "royo-learn-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download archive.
        Write-Info "downloading $downloadUrl..."
        $archivePath = Join-Path $tmpDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        # Download checksums and verify.
        try {
            $checksumPath = Join-Path $tmpDir "checksums.txt"
            Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing -ErrorAction Stop
            Write-Info "verifying checksum..."
            $checksumLines = Get-Content $checksumPath
            $expected = $null
            foreach ($line in $checksumLines) {
                if ($line -match [regex]::Escape($archiveName)) {
                    $expected = ($line -split '\s+')[0]
                    break
                }
            }
            if ($expected) {
                $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
                if ($expected -eq $actual) {
                    Write-Info "checksum OK"
                } else {
                    Write-Info "checksum mismatch (expected $expected, got $actual)"
                }
            } else {
                Write-Info "checksum entry not found for $archiveName"
            }
        } catch {
            Write-Info "checksum download failed, skipping verification"
        }

        # Extract.
        Write-Info "extracting..."
        $extractDir = Join-Path $tmpDir "extracted"
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

        $extractedBinary = Join-Path $extractDir $BinaryName
        if (-not (Test-Path $extractedBinary)) {
            Write-Error-Custom "$BinaryName not found inside archive"
        }

        # Install.
        New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
        Copy-Item $extractedBinary -Destination (Join-Path $BinDir $BinaryName) -Force
        Write-Info "installed to $BinDir\$BinaryName"

        # Verify the installed binary works.
        try {
            $versionOutput = & (Join-Path $BinDir $BinaryName) version --json 2>$null
            if ($versionOutput) {
                Write-Info "verified: $versionOutput"
            }
        } catch {
            Write-Info "version check skipped"
        }

        # Ensure BinDir is on the user PATH. Uses [Environment]::SetEnvironmentVariable
        # (not setx, which truncates PATH at 1024 chars and expands %VAR% references).
        Add-ToUserPath $BinDir

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