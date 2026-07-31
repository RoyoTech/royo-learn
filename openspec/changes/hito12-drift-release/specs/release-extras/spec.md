# Release Extras Specification

## Purpose

Close the pre-v1.0.0 gaps in the release pipeline: emit an SBOM
alongside the GoReleaser artifacts, ship a consolidated `RELEASE.md`
runbook the operator can execute end-to-end without consulting other
documents, and backfill the `CHANGELOG.md` so the v1.0.0 boundary is
honest about what has already merged (Hitos 8/9/10/11) versus what is
still pending the operator's tag decision. The capability covers three
sub-deliverables bundled in one change: SBOM emission in
`.goreleaser.yml`, the new `RELEASE.md` runbook (linked from
`docs/15-OPERATIONS.md`), and the additive `CHANGELOG.md` backfill plus
v1.0.0 ⏳ marker demotion. The change ships preconditions only; it does
NOT tag v1.0.0.

## Requirements

### Requirement: `.goreleaser.yml` emits an SPDX-JSON SBOM alongside archives

The system SHALL add an `sboms:` block under `archives:` in
`.goreleaser.yml` with the configuration
`formats: ['spdx-json']`. GoReleaser v2.17.0 supports `spdx-json`
natively; if a future release rejects the format, the gap MUST be
declared explicitly in `RELEASE.md` §"Known Limitations" and the
snapshot test MUST assert the alternative format actually emitted.
The SBOM artifact MUST sit next to the `tar.gz` / `zip` archives in
the release output directory so install scripts that already verify
SHA-256 can also reference the SBOM without changing their directory
layout. The `brews:` block is unaffected.

#### Scenario: Snapshot build produces an `*.spdx.json` SBOM

- GIVEN `.goreleaser.yml` carries the `sboms:` block
- WHEN `goreleaser release --snapshot --clean` runs locally
- THEN the `dist/` directory contains `*.spdx.json` files alongside
  the `*.tar.gz` and `*.zip` artifacts
- AND every archive has a matching SBOM file with the same base name.

#### Scenario: SBOM format unsupported declares the gap in `RELEASE.md`

- GIVEN a GoReleaser version that rejects `spdx-json`
- WHEN the snapshot build fails with the format error
- THEN the failure is reproduced in CI
- AND `RELEASE.md` §"Known Limitations" contains a sentence declaring
  the gap and listing the supported alternative format
- AND `docs/11-INSTALLATION.md` line 124 SBOM row is updated to
  `"planned"` with a footnote pointing at `RELEASE.md`.

### Requirement: `RELEASE.md` is a self-contained release runbook

The system SHALL create `RELEASE.md` at the repository root. The file
MUST contain, in order: (1) a trigger table that maps each event in the
release flow (`PR merge` → `ci.yml` quality matrix → `release.yml`
workflow_run trigger → GoReleaser build → artifact upload) to its
required checks and prerequisites; (2) a "Required CI checks" section
listing every CI job that MUST be green before a tag is created
(cross-build on Windows/Linux/macOS, `go test -race ./...`,
`go vet ./...`, `gofmt -l .` clean, coverage gates per
`docs/25` §4); (3) a "Tag creation" section describing the tag
naming convention (`vX.Y.Z` for stable releases, `vX.Y.Z-pre.N` for
prereleases), the `workflow_run` SHA alignment requirement, and the
explicit preconditions that all four Hito 12 deliverables must be
merged AND the CHANGELOG backfilled before tagging; (4) an "Install
verification" section with the exact `install.sh` / `install.ps1`
invocations and the SHA-256 verification recipe; (5) a "Rollback
recipe" section describing how to revert via
`install.sh --uninstall` plus reinstalling the previous tag. The
document MUST be executable end-to-end without consulting other docs;
any cross-reference is an additive footnote, not a required read.

#### Scenario: `RELEASE.md` exists and contains the five sections

- GIVEN `RELEASE.md` at the repo root
- WHEN a static review greps for the section headers
- THEN the literals `## Trigger table`, `## Required CI checks`,
  `## Tag creation`, `## Install verification`, and `## Rollback recipe`
  are all present
- AND each section has at least one paragraph of prose (no placeholder
  TODOs).

#### Scenario: `RELEASE.md` is executable without consulting other docs

- GIVEN the operator has only `RELEASE.md` open
- WHEN they walk the trigger table top to bottom
- THEN every step references commands or files within `RELEASE.md`
  itself or within the repo root
- AND no step requires opening `docs/15-OPERATIONS.md` to complete.

#### Scenario: Rollback recipe references `install.sh --uninstall`

- GIVEN `RELEASE.md` §"Rollback recipe"
- WHEN the operator reads the section
- THEN the literal `install.sh --uninstall` is present
- AND the recipe names the previous tag as the reinstall target
- AND the recipe names the SHA-256 verification command as the
  reinstall guard.

### Requirement: `docs/15-OPERATIONS.md` gains a Release runbook section linking to `RELEASE.md`

The system SHALL add a "Release runbook" section to
`docs/15-OPERATIONS.md`. The section MUST be additive (existing
sections unchanged) and MUST contain a single sentence plus a Markdown
link to `RELEASE.md` at the repo root. The link MUST use the
repository-relative form `[Release runbook](../RELEASE.md)` so it
resolves correctly from the `docs/` subtree.

#### Scenario: Operations doc links to the runbook

- GIVEN `docs/15-OPERATIONS.md`
- WHEN `grep -n "Release runbook" docs/15-OPERATIONS.md` runs
- THEN at least one match is present
- AND `grep -n "RELEASE.md" docs/15-OPERATIONS.md` returns at least
  one match.

### Requirement: `CHANGELOG.md` is backfilled for Hitos 8, 9, 10, and 11

The system SHALL move the `[Unreleased]` entries corresponding to Hitos
8, 9, 10, and 11 into their respective release sections
(`[0.8.0]`, `[0.9.0]`, `[0.10.0]`, `[0.11.0]`) using PR titles extracted
from `git log --oneline` for the corresponding merge SHAs. Each entry
MUST carry a footnote of the form `[^pr-N]: #N` linking to the GitHub
PR. If a PR title is misleading, the operator MAY amend the entry
post-merge; the change does not rewrite PR bodies, only titles. The
`[Unreleased]` section after the backfill MUST NOT contain any Hito
8/9/10/11 entries.

#### Scenario: Hito 8 entry lives under `[0.8.0]`

- GIVEN `CHANGELOG.md` post-backfill
- WHEN a static review greps for the Hito 8 PR title
- THEN the title appears under the `[0.8.0]` heading
- AND the entry has a `[^pr-N]: #N` footnote linking to the PR.

#### Scenario: `[Unreleased]` no longer contains Hito 8–11 entries

- GIVEN `CHANGELOG.md` post-backfill
- WHEN the `[Unreleased]` section is read in isolation
- THEN it contains zero references to Hito 8, Hito 9, Hito 10, or
  Hito 11 PR titles
- AND it carries only entries that genuinely are unreleased (e.g.
  Hito 12 itself).

### Requirement: `v1.0.0` ⏳ marker is demoted to an explicit "no tag yet" section

The system SHALL demote the existing `v1.0.0` ⏳ marker in
`CHANGELOG.md` to a clearly worded section that says no `v1.0.0` tag
exists yet, lists the Hito 12 preconditions that must merge before
tagging (SBOM, runbook, CHANGELOG backfill, drift detection), and
links to `RELEASE.md` for the operator-facing procedure. The demoted
section MUST NOT claim a release date; it MUST use the literal phrase
`no tag yet`.

#### Scenario: `v1.0.0` section is present and clearly pending

- GIVEN `CHANGELOG.md` post-backfill
- WHEN the file is read
- THEN a heading contains `v1.0.0`
- AND the section body contains the literal `no tag yet`
- AND the section references `RELEASE.md` via a Markdown link.

#### Scenario: `v1.0.0` section does not claim a release date

- GIVEN `CHANGELOG.md` post-backfill
- WHEN a static review greps the `v1.0.0` section for an ISO-8601 date
- THEN no `20YY-MM-DD` literal appears under that heading.

### Requirement: Coverage gate — release extras have at least one golden-fixture snapshot test

The system SHALL add a snapshot test in
`tests/release/goreleaser_snapshot_test.go` (or equivalent) that runs
`goreleaser release --snapshot --clean` and asserts the `dist/`
directory contains at least one `*.spdx.json` file alongside the
archive artifacts. If the format is unsupported on the CI runner, the
test MUST assert the alternative format emitted and the gap MUST be
recorded in `RELEASE.md` §"Known Limitations". The test MUST use
`t.TempDir()` plus `t.Cleanup()` to avoid leaving snapshot artifacts on
the CI runner.

#### Scenario: Snapshot test passes on linux/amd64

- GIVEN the snapshot test runs in CI on linux/amd64
- WHEN `go test ./tests/release/...` completes
- THEN at least one `*.spdx.json` file is present in the snapshot
  output
- AND the test passes.

#### Scenario: Snapshot test declares the gap when format is unsupported

- GIVEN a GoReleaser version that does not support `spdx-json`
- WHEN the snapshot test runs
- THEN it fails with a message naming the alternative format
- AND `RELEASE.md` §"Known Limitations" lists the gap
- AND `docs/11-INSTALLATION.md` SBOM row is `"planned"`.

## Out of scope

- Code signing / detached signatures (`gpg` / cosign) for the release
  artifacts. The install scripts verify SHA-256 only.
- Dependabot / Renovate config for `go.mod`. Dependency review stays
  manual until a separate change introduces the policy.
- Tagging `v1.0.0`. The change ships preconditions only; the operator
  decides when to tag and which tag scheme to use.
- Transcribing PR descriptions into CHANGELOG entries. The backfill
  uses PR titles only; bodies are not parsed.
- `goreleaser-prerelease: auto` behavior changes for v1.0.0. Kept as
  documented; revisited only if/when tagging happens.

## References

- `docs/11-INSTALLATION.md` line 124 (SBOM row)
- `docs/15-OPERATIONS.md` (target of the cross-link from `RELEASE.md`)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 (Hito 12 acceptance
  rows), §4 (coverage gate)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 (Hito 12 row, pre-v1.0.0
  versioning scheme)
- GoReleaser v2.17.0 SBOM docs
