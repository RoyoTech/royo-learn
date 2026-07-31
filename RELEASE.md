# Release Runbook

This document is the single, self-contained procedure for cutting and rolling
back a `royo-learn` release. An operator with only this file open can walk the
trigger table top to bottom and execute a release end-to-end without opening
any other document. Cross-references to other docs are additive footnotes,
never required reads.

## Trigger table

The release flow is a pipeline of triggers. Each row fires the next; no tag is
created until every prior row is green.

| # | Event | What happens | Required checks / preconditions |
|---|-------|--------------|---------------------------------|
| 1 | PR merge to `main` | A pull request merges into `main`. | PR approved; `ci.yml` triggered on the merge SHA. |
| 2 | `ci.yml` quality matrix | CI runs on Windows/Linux/macOS with Go 1.25.0 and 1.26.x. | `go fmt ./...` clean; `go mod tidy` clean; `go mod verify`; `go vet ./...`; `go build ./cmd/royo-learn`; `go test -race ./...` (Linux); `go test ./...` (non-Linux). |
| 3 | `ci.yml` cross-build + coverage + smoke | Cross-build matrix (linux/darwin/windows × amd64/arm64); coverage gates; clean-install smoke test; PowerShell installer fail-closed test. | Cross-build green; coverage ≥ thresholds (domain ≥ 80%, storage ≥ 80%, publish ≥ 90%); smoke test green. |
| 4 | `release.yml` `workflow_run` trigger | The Release workflow fires automatically when CI completes successfully on the tagged SHA. | `github.event.workflow_run.conclusion == 'success'`; exactly one `v*` tag points at the proven SHA. |
| 5 | GoReleaser build | GoReleaser builds binaries, archives (`tar.gz`/`zip`), SBOM (`*.spdx.json`), and `checksums.txt`; uploads to GitHub Releases. | `.goreleaser.yml` carries the `sboms:` block with `formats: ['spdx-json']`; `GITHUB_TOKEN` available; GoReleaser v2.17.0 pinned. |
| 6 | Artifact upload | Release assets are published: archives, SBOM, `checksums.txt`, `install.sh`, `install.ps1`. | All assets present in the GitHub release; `checksums.txt` includes every archive. |

## Required CI checks

Before a tag is created, every CI job listed here MUST be green on the exact
SHA that will be tagged. The Release workflow (`release.yml`) enforces this by
keying off `workflow_run.conclusion == 'success'`.

**Quality matrix** (`.github/workflows/ci.yml`, job `quality`):
- **Cross-platform build:** Windows (`windows-latest`), Linux (`ubuntu-latest`),
  macOS (`macos-latest`) — all with Go 1.25.0 and Go 1.26.x.
- **Format check:** `go fmt ./...` followed by `git diff --exit-code` — no
  unformatted code.
- **Module tidy check:** `go mod tidy` followed by `git diff --exit-code` on
  `go.mod` and `go.sum`.
- **Dependency verification:** `go mod verify` passes.
- **Vet:** `go vet ./...` passes.
- **Build:** `go build ./cmd/royo-learn` succeeds.
- **Test (race):** `go test -race -count=1 ./...` passes on Linux.
- **Test:** `go test -count=1 ./...` passes on non-Linux runners.

**Cross-build** (`.github/workflows/ci.yml`, job `cross-build`):
- Compiles every target shipped by `.goreleaser.yml`:
  linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64,
  windows/arm64. CI cannot execute all of these (e.g. darwin/arm64 on Linux),
  so the cross-build job proves they compile.

**Coverage gates** (`.github/workflows/ci.yml`, job `coverage`):
- `internal/domain` ≥ 80%
- `internal/storage` ≥ 80%
- `internal/publish` ≥ 90%

**Clean install smoke** (`.github/workflows/ci.yml`, job `clean-install-smoke`):
- Installs the binary into a clean `GOBIN` and runs `royo-learn version`.

**Installer safety** (`.github/workflows/ci.yml`, job `installer-safety`):
- `scripts/test-install.ps1` runs fail-closed PowerShell installer tests.

**SBOM snapshot** (`tests/release/goreleaser_snapshot_test.go`):
- `go test ./tests/release/...` runs `goreleaser release --snapshot --clean`
  and asserts at least one `*.spdx.json` file lands in `dist/`. Skipped when
  the `goreleaser` binary is not on PATH (does not fail CI on runners without
  GoReleaser).

## Tag creation

### Naming convention

- **Stable release:** `vX.Y.Z` (e.g. `v1.0.0`, `v0.2.0`). Semantic Versioning
  rules apply: breaking changes bump `X`, new capabilities bump `Y`, fixes
  bump `Z`.
- **Prerelease:** `vX.Y.Z-pre.N` (e.g. `v1.0.0-pre.1`, `v0.2.0-rc1` is the
  legacy form already in use). The `-pre.N` suffix signals "not yet stable."

### SHA alignment

The Release workflow (`release.yml`) uses a `workflow_run` trigger: it fires
only when CI (`ci.yml`) completes successfully. The Release workflow checks out
the exact SHA that CI proved (`github.event.workflow_run.head_sha`) and requires
exactly one `v*` tag to point at that SHA. If zero tags point at the SHA, the
Release workflow exits without producing artifacts. If more than one tag
points at the SHA, the Release workflow fails.

This means: create the tag on the commit that CI already proved green. Do not
tag a SHA that CI has not validated.

### Preconditions

Before creating any tag, ALL of the following must be true:

1. **SBOM emission** — `.goreleaser.yml` carries the `sboms:` block with
   `formats: ['spdx-json']`, and the snapshot test in
   `tests/release/goreleaser_snapshot_test.go` passes (or skips because
   GoReleaser is absent from the runner).
2. **Release runbook** — `RELEASE.md` (this file) exists at the repo root and
   is self-contained.
3. **CHANGELOG backfill** — `CHANGELOG.md` has backfilled entries for Hitos 8,
   9, 10, and 11 under their respective `[0.8.0]`, `[0.9.0]`, `[0.10.0]`, and
   `[0.11.0]` sections. The `[Unreleased]` section contains only genuinely
   unreleased items.
4. **Drift detection** — the publication drift checker (Hito 12 Slice 1/2)
   ships four outcomes (`ok`, `drifted`, `target_missing`, `target_unreadable`)
   and is wired into the publish path.

### Creating the tag

```bash
# 1. Ensure you are on the proven SHA.
git checkout <proven-sha>

# 2. Create and push the tag.
git tag -a v1.0.0 -m "v1.0.0 — first production-ready release"
git push origin v1.0.0
```

Pushing the tag triggers the Release workflow (via `workflow_run` on CI). The
workflow checks out the tagged SHA, verifies tag identity, and runs GoReleaser.

## Install verification

After GoReleaser publishes the release artifacts, verify the install path on a
clean machine before announcing the release.

### Linux / macOS

```bash
# Download and run the installer for a specific tag.
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/download/v1.0.0/install.sh | bash

# Or pin a version explicitly:
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/latest/download/install.sh | bash -s -- --version v1.0.0

# Verify the binary is on PATH and reports the correct version.
royo-learn version
royo-learn doctor
```

The installer (`install.sh`) downloads the archive for the detected platform,
fetches `checksums.txt`, and verifies the SHA-256 checksum before extracting
the binary. If the checksum does not match, the installer aborts with
`checksum mismatch`.

### Windows (PowerShell)

```powershell
Invoke-WebRequest -Uri https://github.com/RoyoTech/royo-learn/releases/download/v1.0.0/install.ps1 -OutFile install.ps1
.\install.ps1 -Version v1.0.0

# Verify.
royo-learn version
royo-learn doctor
```

The PowerShell installer (`install.ps1`) performs the same SHA-256 checksum
verification using `checksums.txt` from the release assets. It fails closed on
any missing or mismatched checksum entry.

### Manual SHA-256 verification

To manually verify an archive against the published checksums:

```bash
# Download checksums.txt for the tag.
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/download/v1.0.0/checksums.txt -o checksums.txt

# Download the archive.
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/download/v1.0.0/royo-learn-linux-amd64.tar.gz -o royo-learn-linux-amd64.tar.gz

# Verify.
expected=$(awk '$2 ~ /royo-learn-linux-amd64.tar.gz$/ { print $1; exit }' checksums.txt)
actual=$(sha256sum royo-learn-linux-amd64.tar.gz | awk '{print $1}')
[ "$expected" = "$actual" ] && echo "checksum OK" || echo "CHECKSUM MISMATCH"
```

## Rollback recipe

When a release is broken, roll back by uninstalling the current version and
reinstalling the last known-good tag.

### Step 1 — Uninstall the broken version

```bash
# Linux / macOS
install.sh --uninstall

# Windows (PowerShell)
.\install.ps1 -Uninstall
```

The `--uninstall` / `-Uninstall` flag removes the `royo-learn` binary from the
install directory (`${HOME}/.local/bin` on Linux/macOS, the user profile on
Windows). It does not touch user data (SQLite DB, records, evidence blobs).

### Step 2 — Reinstall the previous tag

Identify the last known-good tag (e.g. `v0.11.0` if `v1.0.0` is broken) and
reinstall it:

```bash
# Linux / macOS — pin to the previous tag.
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/latest/download/install.sh | bash -s -- --version v0.11.0

# Windows (PowerShell)
.\install.ps1 -Version v0.11.0
```

### Step 3 — Verify the reinstall with SHA-256

```bash
# Confirm the binary reports the previous version.
royo-learn version

# Manually verify the archive checksum for the previous tag.
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/download/v0.11.0/checksums.txt -o checksums.txt
expected=$(awk '$2 ~ /royo-learn-linux-amd64.tar.gz$/ { print $1; exit }' checksums.txt)
actual=$(sha256sum "$(which royo-learn 2>/dev/null || echo /dev/null)" 2>/dev/null | awk '{print $1}')
# Re-download and verify the archive directly:
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/download/v0.11.0/royo-learn-linux-amd64.tar.gz -o royo-learn.tar.gz
actual=$(sha256sum royo-learn.tar.gz | awk '{print $1}')
[ "$expected" = "$actual" ] && echo "checksum OK — rollback verified" || echo "CHECKSUM MISMATCH — investigate"
```

The reinstall guard is the SHA-256 checksum from `checksums.txt` for the
previous tag. The installer performs this automatically; the manual recipe
above is for operator-led verification when the installer's own verification is
in question.

## Known Limitations

- **SBOM format:** `.goreleaser.yml` uses `formats: ['spdx-json']` which
  GoReleaser v2.17.0 supports natively. If a future GoReleaser release rejects
  `spdx-json`, the snapshot test will fail and the gap must be declared here
  with the supported alternative format listed.
- **`-race` requires CGO and a C compiler.** The project ships pure-Go
  SQLite (`modernc.org/sqlite`) to keep the binary CGO-free. The race
  detector therefore needs `CGO_ENABLED=1` plus a C compiler. Cross-build
  compiles without CGO (`go build` for linux/amd64, linux/arm64,
  darwin/amd64, darwin/arm64, windows/amd64 — all green locally).
  `go test -race ./...` runs only on the Linux CI runner where gcc is
  preinstalled (see `.github/workflows/ci.yml` job `quality`, step
  `Test (race)` guarded by `if: runner.os == 'Linux'`). Local operators
  without gcc should run `go test ./...` (no `-race`) and trust CI for
  the race gate.
- **`goreleaser` CLI not installed in this WSL sandbox.** The snapshot
  test in `tests/release/goreleaser_snapshot_test.go` calls `t.Skip` with
  a clear message when the binary is absent on PATH so it never fails CI
  due to missing tooling. CI installs goreleaser before running the
  snapshot job (see `.github/workflows/ci.yml`).
