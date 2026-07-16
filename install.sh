#!/usr/bin/env bash
# install.sh — royo-learn installer for Linux and macOS
# Usage: curl -fsSL <url>/install.sh | bash
#        ./install.sh --version v0.1.1
#        ./install.sh --uninstall
set -euo pipefail

REPO="RoyoTech/royo-learn"
DEFAULT_VERSION="latest"
INSTALL_DIR="${ROYO_LEARN_INSTALL_DIR:-${HOME}/.local/bin}"
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

    local base_url="${ROYO_LEARN_RELEASES_URL:-https://github.com/${REPO}/releases}"
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
    local target="${INSTALL_DIR}/${BINARY_NAME}"
    local staged="${target}.new.$$"
    local backup="${target}.rollback.$$"
    local replacement_started=false
    local install_succeeded=false
    local had_existing=false
    rollback() {
        if [ "$replacement_started" = true ] && [ "$install_succeeded" != true ]; then
            if [ "$had_existing" = true ] && [ -f "$backup" ]; then
                mv -f "$backup" "$target"
                info "rollback restored the previous binary"
            else
                rm -f "$target"
            fi
        fi
        rm -f "$staged" "$backup"
        rm -rf "$tmpdir"
    }
    trap rollback EXIT

    # Download archive.
    info "downloading ${download_url}..."
    local archive_path="${tmpdir}/${archive_name}"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$archive_path" "$download_url" || error "download failed"
        curl -fsSL -o "${tmpdir}/checksums.txt" "$checksum_url" || error "checksum download failed"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$archive_path" "$download_url" || error "download failed"
        wget -q -O "${tmpdir}/checksums.txt" "$checksum_url" || error "checksum download failed"
    else
        error "curl or wget required"
    fi

    # Verification is mandatory: missing tools, files, entries, or mismatches fail closed.
    local expected
    expected=$(awk -v name="$archive_name" '$2 == name || $2 == "*" name { print $1; exit }' "${tmpdir}/checksums.txt")
    [ -n "$expected" ] || error "checksum entry not found for ${archive_name}"
    local actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive_path" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive_path" | awk '{print $1}')
    else
        error "sha256sum or shasum is required for checksum verification"
    fi
    [ "$expected" = "$actual" ] || error "checksum mismatch (expected $expected, got $actual)"
    info "checksum OK"

    # Extract.
    info "extracting..."
    tar -xzf "$archive_path" -C "$tmpdir"
    if [ ! -f "${tmpdir}/${BINARY_NAME}" ]; then
        error "${BINARY_NAME} not found inside archive"
    fi
    chmod +x "${tmpdir}/${BINARY_NAME}"

    local version_json actual_version expected_version
    version_json=$("${tmpdir}/${BINARY_NAME}" version --json) || error "candidate version verification failed"
    actual_version=$(printf '%s\n' "$version_json" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p')
    [ -n "$actual_version" ] || error "candidate version output is invalid"
    if [ "$version" = "latest" ]; then
        [ "$actual_version" != "dev" ] || error "version mismatch: latest release resolved to a development build"
    else
        expected_version=${version#v}
        [ "$actual_version" = "$expected_version" ] || error "version mismatch (expected $expected_version, got $actual_version)"
    fi

    # Stage on the destination filesystem, preserve the old binary, then replace atomically.
    mkdir -p "$INSTALL_DIR"
    cp "${tmpdir}/${BINARY_NAME}" "$staged"
    chmod +x "$staged"
    if [ -f "$target" ]; then
        had_existing=true
        cp -p "$target" "$backup"
    fi
    replacement_started=true
    mv -f "$staged" "$target"

    version_json=$("$target" version --json) || error "installed binary version verification failed"
    if ! printf '%s\n' "$version_json" | grep -q "\"version\"[[:space:]]*:[[:space:]]*\"${actual_version}\""; then
        error "installed binary version mismatch"
    fi
    install_succeeded=true
    rm -f "$backup"
    info "installed to ${target}"
    info "verified: ${version_json}"

    # PATH note.
    if ! echo "$PATH" | tr ':' '\n' | grep -qxF "$INSTALL_DIR"; then
        info "NOTE: add ${INSTALL_DIR} to your PATH:"
        info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        info "  (add this to ~/.bashrc, ~/.zshrc, or ~/.profile)"
    fi

    info "install complete!"
    rollback
    trap - EXIT
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
