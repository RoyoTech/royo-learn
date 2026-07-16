$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) "royo-install-test-$([guid]::NewGuid().ToString('N'))"
$version = '0.1.10'
$tag = "v$version"
$archive = 'royo-learn-windows-amd64.zip'
$releaseDir = Join-Path $tempRoot "releases\download\$tag"
$payloadDir = Join-Path $tempRoot 'payload'
$installRoot = Join-Path $tempRoot 'install-root'

try {
    New-Item -ItemType Directory -Path $releaseDir, $payloadDir, $installRoot -Force | Out-Null
    $candidate = Join-Path $payloadDir 'royo-learn.exe'
    & go build -o $candidate -ldflags "-X agent-royo-learn/internal/buildinfo.Version=$version" (Join-Path $repoRoot 'cmd/royo-learn')
    if ($LASTEXITCODE -ne 0) { throw 'candidate build failed' }
    $archivePath = Join-Path $releaseDir $archive
    Compress-Archive -LiteralPath $candidate -DestinationPath $archivePath
    $checksum = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value "$checksum  $archive" -Encoding ascii
    $releaseUrl = ([uri](Join-Path $tempRoot 'releases')).AbsoluteUri.TrimEnd('/')

    $env:ROYO_LEARN_INSTALL_ROOT = $installRoot
    $env:ROYO_LEARN_RELEASES_URL = $releaseUrl
    $env:ROYO_LEARN_SKIP_PATH_UPDATE = '1'
    & pwsh -NoProfile -File (Join-Path $repoRoot 'install.ps1') -Version $tag
    if ($LASTEXITCODE -ne 0) { throw 'valid install failed' }
    $target = Join-Path $installRoot 'bin\royo-learn.exe'
    $before = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash

    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value "00  $archive" -Encoding ascii
    & pwsh -NoProfile -File (Join-Path $repoRoot 'install.ps1') -Version $tag
    if ($LASTEXITCODE -eq 0) { throw 'checksum mismatch unexpectedly succeeded' }
    if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $before) { throw 'checksum failure replaced existing binary' }

    $wrongDir = Join-Path $tempRoot 'releases\download\v0.1.11'
    New-Item -ItemType Directory -Path $wrongDir -Force | Out-Null
    Copy-Item -LiteralPath $archivePath -Destination (Join-Path $wrongDir $archive)
    Set-Content -LiteralPath (Join-Path $wrongDir 'checksums.txt') -Value "$checksum  $archive" -Encoding ascii
    & pwsh -NoProfile -File (Join-Path $repoRoot 'install.ps1') -Version 'v0.1.11'
    if ($LASTEXITCODE -eq 0) { throw 'version mismatch unexpectedly succeeded' }
    if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $before) { throw 'version failure replaced existing binary' }

    $probeState = Join-Path $tempRoot 'probe-state'
    $probe = Join-Path $payloadDir 'royo-learn.exe'
    & go build -o $probe -ldflags "-X main.version=$version" (Join-Path $repoRoot 'internal/integration/testdata/installer-probe')
    if ($LASTEXITCODE -ne 0) { throw 'rollback probe build failed' }
    Remove-Item -LiteralPath $archivePath -Force
    Compress-Archive -LiteralPath $probe -DestinationPath $archivePath
    $checksum = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $releaseDir 'checksums.txt') -Value "$checksum  $archive" -Encoding ascii
    $env:ROYO_LEARN_PROBE_STATE = $probeState
    & pwsh -NoProfile -File (Join-Path $repoRoot 'install.ps1') -Version $tag
    if ($LASTEXITCODE -eq 0) { throw 'post-replacement failure unexpectedly succeeded' }
    if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $before) { throw 'post-replacement failure did not restore prior binary' }
} finally {
    Remove-Item Env:ROYO_LEARN_INSTALL_ROOT -ErrorAction SilentlyContinue
    Remove-Item Env:ROYO_LEARN_RELEASES_URL -ErrorAction SilentlyContinue
    Remove-Item Env:ROYO_LEARN_SKIP_PATH_UPDATE -ErrorAction SilentlyContinue
    Remove-Item Env:ROYO_LEARN_PROBE_STATE -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
