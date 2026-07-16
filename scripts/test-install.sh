#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

version="0.1.10"
tag="v${version}"
platform="$(uname -s | tr '[:upper:]' '[:lower:]')-amd64"
archive="royo-learn-${platform}.tar.gz"
release_dir="${tmpdir}/releases/download/${tag}"
install_dir="${tmpdir}/install"
mkdir -p "$release_dir" "$install_dir" "${tmpdir}/payload"

if [[ -n "${ROYO_LEARN_TEST_BINARY:-}" ]]; then
  cp "$ROYO_LEARN_TEST_BINARY" "${tmpdir}/payload/royo-learn"
  chmod +x "${tmpdir}/payload/royo-learn"
else
  CGO_ENABLED=0 go build -o "${tmpdir}/payload/royo-learn" \
    -ldflags "-X agent-royo-learn/internal/buildinfo.Version=${version}" \
    "${repo_root}/cmd/royo-learn"
fi
tar -czf "${release_dir}/${archive}" -C "${tmpdir}/payload" royo-learn
checksum=$(sha256sum "${release_dir}/${archive}" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive" > "${release_dir}/checksums.txt"

ROYO_LEARN_INSTALL_DIR="$install_dir" \
ROYO_LEARN_RELEASES_URL="file://${tmpdir}/releases" \
bash "${repo_root}/install.sh" --version "$tag"
"${install_dir}/royo-learn" version --json | grep -q "\"version\":\"${version}\""

before=$(sha256sum "${install_dir}/royo-learn" | awk '{print $1}')
printf '00  %s\n' "$archive" > "${release_dir}/checksums.txt"
if ROYO_LEARN_INSTALL_DIR="$install_dir" ROYO_LEARN_RELEASES_URL="file://${tmpdir}/releases" \
  bash "${repo_root}/install.sh" --version "$tag"; then
  printf 'checksum mismatch unexpectedly succeeded\n' >&2
  exit 1
fi
after=$(sha256sum "${install_dir}/royo-learn" | awk '{print $1}')
test "$before" = "$after"

wrong_dir="${tmpdir}/releases/download/v0.1.11"
mkdir -p "$wrong_dir"
cp "${release_dir}/${archive}" "$wrong_dir/$archive"
checksum=$(sha256sum "$wrong_dir/$archive" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive" > "$wrong_dir/checksums.txt"
if ROYO_LEARN_INSTALL_DIR="$install_dir" ROYO_LEARN_RELEASES_URL="file://${tmpdir}/releases" \
  bash "${repo_root}/install.sh" --version v0.1.11; then
  printf 'version mismatch unexpectedly succeeded\n' >&2
  exit 1
fi
test "$before" = "$(sha256sum "${install_dir}/royo-learn" | awk '{print $1}')"

rm -f "${tmpdir}/probe-state"
if [[ -n "${ROYO_LEARN_TEST_PROBE:-}" ]]; then
  cp "$ROYO_LEARN_TEST_PROBE" "${tmpdir}/payload/royo-learn"
  chmod +x "${tmpdir}/payload/royo-learn"
else
  CGO_ENABLED=0 go build -o "${tmpdir}/payload/royo-learn" \
    -ldflags "-X main.version=${version}" \
    "${repo_root}/internal/integration/testdata/installer-probe"
fi
tar -czf "${release_dir}/${archive}" -C "${tmpdir}/payload" royo-learn
checksum=$(sha256sum "${release_dir}/${archive}" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive" > "${release_dir}/checksums.txt"
if ROYO_LEARN_INSTALL_DIR="$install_dir" ROYO_LEARN_RELEASES_URL="file://${tmpdir}/releases" \
  ROYO_LEARN_PROBE_STATE="${tmpdir}/probe-state" bash "${repo_root}/install.sh" --version "$tag"; then
  printf 'post-replacement failure unexpectedly succeeded\n' >&2
  exit 1
fi
test "$before" = "$(sha256sum "${install_dir}/royo-learn" | awk '{print $1}')"
