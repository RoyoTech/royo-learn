#!/usr/bin/env bash
# install.sh — royo-learn installer for Linux and macOS
# Usage: curl -fsSL <url>/install.sh | bash
#        ./install.sh --version v0.1.0
#        ./install.sh --uninstall
set -euo pipefail

REPO="RoyoTech/royo-learn"
DEFAULT_VERSION="latest"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="royo-learn"

# ---------- helpers ----------
info()  { echo "[royo-learn] $*"; }
error() { echo "[royo-learn] ERROR: $*" >&2; exit 1; }

detect_platform() {
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) error "unsupported architecture: $arch" ;;
    esac
    case "$os" in
        linux|darwin) ;;
        msys*|mingw*|cygwin*)
            error "Git Bash / MSYS / Cygwin detected. This installer is for Linux/macOS/WSL.
  On Windows, use PowerShell instead:

    Invoke-WebRequest -Uri https://github.com/RoyoTech/royo-learn/releases/latest/download/install.ps1 -OutFile install.ps1
    .\install.ps1" ;;
        *) error "unsupported OS: $os" ;;
    esac
    echo "${os}-${arch}"
}

# ---------- uninstall ----------
do_uninstall() {
    local target="${INSTALL_DIR}/${BINARY_NAME}"
    if [ -f "$target" ]; then
        rm -f "$target"
        info "removed ${target}"
    else
        info "${BINARY_NAME} not found at ${target}"
    fi
    info "uninstall complete. Remove '${INSTALL_DIR}' from your PATH if desired."
    exit 0
}

# ---------- download + install ----------
do_install() {
    local version="${1:-$DEFAULT_VERSION}"
    local platform
    platform=$(detect_platform)

    # GoReleaser naming: royo-learn-linux-amd64.tar.gz
    local archive_name="${BINARY_NAME}-${platform}.tar.gz"

    local base_url="https://github.com/${REPO}/releases"
    local download_url checksum_url
    if [ "$version" = "latest" ]; then
        download_url="${base_url}/latest/download/${archive_name}"
        checksum_url="${base_url}/latest/download/checksums.txt"
    else
        download_url="${base_url}/download/${version}/${archive_name}"
        checksum_url="${base_url}/download/${version}/checksums.txt"
    fi

    info "installing royo-learn ${version} for ${platform}..."

    local tmpdir
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    # Download archive.
    info "downloading ${download_url}..."
    local archive_path="${tmpdir}/${archive_name}"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$archive_path" "$download_url" || error "download failed"
        curl -fsSL -o "${tmpdir}/checksums.txt" "$checksum_url" 2>/dev/null || true
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$archive_path" "$download_url" || error "download failed"
        wget -q -O "${tmpdir}/checksums.txt" "$checksum_url" 2>/dev/null || true
    else
        error "curl or wget required"
    fi

    # Verify checksum if available.
    local expected
    expected=$(grep "$archive_name" "${tmpdir}/checksums.txt" 2>/dev/null | awk '{print $1}')
    if [ -n "$expected" ] && command -v sha256sum >/dev/null 2>&1; then
        local actual
        actual=$(sha256sum "$archive_path" | awk '{print $1}')
        if [ "$expected" = "$actual" ]; then
            info "checksum OK"
        else
            info "checksum mismatch (expected $expected, got $actual)"
        fi
    fi

    # Extract.
    info "extracting..."
    tar -xzf "$archive_path" -C "$tmpdir"
    if [ ! -f "${tmpdir}/${BINARY_NAME}" ]; then
        error "${BINARY_NAME} not found inside archive"
    fi
    chmod +x "${tmpdir}/${BINARY_NAME}"

    # Install.
    mkdir -p "$INSTALL_DIR"
    mv "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    info "installed to ${INSTALL_DIR}/${BINARY_NAME}"

    # Verify.
    if "${INSTALL_DIR}/${BINARY_NAME}" version --json >/dev/null 2>&1; then
        info "verified: $("${INSTALL_DIR}/${BINARY_NAME}" version --json)"
    fi

    # PATH note.
    if ! echo "$PATH" | tr ':' '\n' | grep -qxF "$INSTALL_DIR"; then
        info "NOTE: add ${INSTALL_DIR} to your PATH:"
        info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        info "  (add this to ~/.bashrc, ~/.zshrc, or ~/.profile)"
    fi

    info "install complete!"
}

# ---------- main ----------
VERSION="$DEFAULT_VERSION"
UNINSTALL=false

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --uninstall) UNINSTALL=true; shift ;;
        *) error "unknown argument: $1" ;;
    esac
done

if [ "$UNINSTALL" = true ]; then
    do_uninstall
fi

do_install "$VERSION"