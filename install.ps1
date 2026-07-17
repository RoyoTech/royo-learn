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
$InstallRoot = if ($env:ROYO_LEARN_INSTALL_ROOT) { $env:ROYO_LEARN_INSTALL_ROOT } else { "$env:LOCALAPPDATA\royo-learn" }
$BinDir = "$InstallRoot\bin"
$BinaryName = "royo-learn.exe"

function Write-Info { Write-Host "[royo-learn] $args" -ForegroundColor Cyan }
function Write-Error-Custom { Write-Host "[royo-learn] ERROR: $args" -ForegroundColor Red; exit 1 }

function Receive-File {
    param([Parameter(Mandatory)][string]$Uri, [Parameter(Mandatory)][string]$OutFile)
    if ($Uri.StartsWith('file:', [System.StringComparison]::OrdinalIgnoreCase)) {
        Copy-Item -LiteralPath ([uri]$Uri).LocalPath -Destination $OutFile
        return
    }
    Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing -ErrorAction Stop
}

function Get-BinaryVersion {
    param([Parameter(Mandatory)][string]$Path)
    $raw = & $Path version --json
    if ($LASTEXITCODE -ne 0 -or -not $raw) {
        throw "version verification failed for $Path"
    }
    $parsed = $raw | ConvertFrom-Json
    if (-not $parsed.version) {
        throw "version output from $Path does not contain version"
    }
    return [string]$parsed.version
}

function Remove-StaleArtifacts {
    # Tolerate locked or otherwise un-removable files (antivirus, indexing, open handles).
    # We never abort the install because of cleanup hygiene; we only warn.
    param([Parameter(Mandatory)][string]$Dir)
    if (-not (Test-Path -LiteralPath $Dir)) { return }
    $patterns = @("*.bak", "*.previous", "*.rollback.*", "*.new.*")
    foreach ($pat in $patterns) {
        Get-ChildItem -LiteralPath $Dir -Filter $pat -Force -ErrorAction SilentlyContinue | ForEach-Object {
            $path = $_.FullName
            try {
                Remove-Item -LiteralPath $path -Force -ErrorAction Stop
            } catch {
                # Some installers historically produced trailing-dot names that Win32
                # accepts but cannot remove through the normal path. Use the
                # \\?\ extended path to bypass normalization.
                $extended = "\\?\" + $path
                try {
                    Remove-Item -LiteralPath $extended -Force -ErrorAction Stop
                } catch {
                    Write-Info "warning: could not remove stale artifact: $path ($($_.Exception.Message))"
                }
            }
        }
    }
}

function Restore-PreviousBinary {
    param(
        [Parameter(Mandatory)][string]$Target,
        [Parameter(Mandatory)][string]$Backup,
        [Parameter(Mandatory)][bool]$HadExisting
    )
    if ($HadExisting -and (Test-Path -LiteralPath $Backup)) {
        [System.IO.File]::Move($Backup, $Target, $true)
        Write-Info "rollback restored the previous binary"
    } elseif (Test-Path -LiteralPath $Target) {
        Remove-Item -LiteralPath $Target -Force
    }
}

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

    $baseUrl = if ($env:ROYO_LEARN_RELEASES_URL) { $env:ROYO_LEARN_RELEASES_URL.TrimEnd('/') } else { "https://github.com/$Repo/releases" }
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
        Receive-File -Uri $downloadUrl -OutFile $archivePath

        # Download checksums and verify. Every missing or mismatched input fails closed.
        $checksumPath = Join-Path $tmpDir "checksums.txt"
        Receive-File -Uri $checksumUrl -OutFile $checksumPath
        Write-Info "verifying checksum..."
        $expected = $null
        foreach ($line in (Get-Content -LiteralPath $checksumPath)) {
            $parts = $line -split '\s+', 2
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $archiveName) {
                $expected = $parts[0].ToLowerInvariant()
                break
            }
        }
        if (-not $expected) { throw "checksum entry not found for $archiveName" }
        $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($expected -ne $actual) { throw "checksum mismatch (expected $expected, got $actual)" }
        Write-Info "checksum OK"

        # Extract.
        Write-Info "extracting..."
        $extractDir = Join-Path $tmpDir "extracted"
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

        $extractedBinary = Join-Path $extractDir $BinaryName
        if (-not (Test-Path $extractedBinary)) {
            Write-Error-Custom "$BinaryName not found inside archive"
        }

        $actualVersion = Get-BinaryVersion -Path $extractedBinary
        if ($Ver -eq "latest") {
            if ($actualVersion -eq "dev") { throw "version mismatch: latest release resolved to a development build" }
        } else {
            $expectedVersion = $Ver.TrimStart('v')
            if ($actualVersion -ne $expectedVersion) {
                throw "version mismatch (expected $expectedVersion, got $actualVersion)"
            }
        }

        # Stage on the destination filesystem and atomically replace with rollback.
        New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

        # Best-effort cleanup of artifacts from previous runs that may have aborted.
        # Doing this BEFORE the replace avoids lock conflicts against $target/backup.
        Remove-StaleArtifacts -Dir $BinDir

        $target = Join-Path $BinDir $BinaryName
        # NB: do NOT append a trailing dot to $backup. Win32 accepts it but NTFS cannot
        # normalize it, which makes later Remove-Item calls fail with PermissionDenied.
        $staged = Join-Path $BinDir "$BinaryName.new-$([guid]::NewGuid().ToString('N'))"
        $backup = Join-Path $BinDir "$BinaryName.rollback-$([guid]::NewGuid().ToString('N'))"
        $hadExisting = Test-Path -LiteralPath $target
        Copy-Item -LiteralPath $extractedBinary -Destination $staged
        try {
            if ($hadExisting) {
                [System.IO.File]::Replace($staged, $target, $backup, $true)
            } else {
                [System.IO.File]::Move($staged, $target)
            }
            $installedVersion = Get-BinaryVersion -Path $target
            if ($installedVersion -ne $actualVersion) {
                throw "installed binary version mismatch (expected $actualVersion, got $installedVersion)"
            }
        } catch {
            Restore-PreviousBinary -Target $target -Backup $backup -HadExisting $hadExisting
            throw
        } finally {
            Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        }
        if (Test-Path -LiteralPath $backup) {
            try {
                Remove-Item -LiteralPath $backup -Force -ErrorAction Stop
            } catch {
                # Backup may be held by antivirus/indexer. Warn but do NOT fail the install;
                # the new binary is already in place and verified. Remove-StaleArtifacts
                # will sweep it on the next run.
                Write-Info "warning: backup not removed (will be cleaned on next install): $backup ($($_.Exception.Message))"
            }
        }
        Write-Info "installed to $target"
        Write-Info "verified version: $installedVersion"

        # Ensure BinDir is on the user PATH. Uses [Environment]::SetEnvironmentVariable
        # (not setx, which truncates PATH at 1024 chars and expands %VAR% references).
        if ($env:ROYO_LEARN_SKIP_PATH_UPDATE -ne '1') {
            Add-ToUserPath $BinDir
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
