# Changelog

All notable changes to `royo-learn` will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/)
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

Local `main` is 21 commits ahead of `origin/main` (Hito 1 closure +
housekeeping). None of this is shipped yet. The first tag that
includes any of this work is the trigger-driven `v0.2.0-rc1` once
Hito 1 + the documentation gap (PR #19) are merged on the remote
side.

### Added

- **Hito 1: experience discovery** (slices 1.A-1.D, merge commit
  `b105e34` on local main).
  - Domain model, validation, and typed errors
    (`internal/domain/experience.go`).
  - Capture/ingest service with idempotency, fingerprint, and
    append-only audit (`internal/experience/`, migration `004`).
  - CLI `experience inject` fixture command
    (`cmd/royo-learn/experience.go`).
  - 90% test coverage on `internal/experience`.
- **Frozen contracts** (delivered via PR #19, not yet merged):
  - `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
  - `docs/20-EXPERIENCE-INGESTION-PRD.md`
  - `docs/21-EXPERIENCE-DOMAIN.md`
  - `docs/22-ADAPTER-CONTRACT.md`
  - `docs/23-PATTERN-MINING.md`
  - `docs/24-EXPERIENCE-THREAT-MODEL.md`
  - `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md`
  - `docs/26-IMPLEMENTATION-ROADMAP.md`
  - `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md`
- **Operational patterns** for agents working in this repo:
  `docs/lessons.md` captures shell detection, WSL bypass for the
  harness lifecycle interceptor, `gentle_review` scope discipline,
  and PR-base rules. Referenced from `AGENTS.md` and a new
  `CLAUDE.md`.

### Changed

- `docs/IMPLEMENTATION-NOTES.md:466` — the `internal/mcpserver`
  `ListTools: context deadline exceeded` observation is now a
  recorded investigation result (ADR-0002 §7) rather than an open
  question.

### Fixed

- **ADR-0002 §4** investigated the MCP timeout flake under the
  documented scope. Result: 0 of 40 iterations across base `4fe9774`
  and HEAD `b105e34` reproduced the failure. `internal/mcpserver`
  source is bit-identical between the two commits, so the flake is
  environmental and timing-sensitive. The ADR remains `Proposed`
  for monitoring; the original "investigation needed before Hito 2"
  sentence in `IMPLEMENTATION-NOTES.md` is replaced with a one-line
  pointer to the new §7.
- `cmd/royo-learn/experience_test.go` now uses `testutil.TempDir(t)`
  to amortize Windows Defender cleanup flakes (post-review
  correction, commit `f989579`).

## [0.1.10] - 2026-07-16

### Fixed

- Release safety in the publish layer (PR #11, commit `e88090b`).
  Backstop checks on rollback conflict reporting and managed-block
  verification.

## [0.1.9] - 2026-07-13

### Added

- Self-update flow (PR #10). The `setup upgrade` command, version
  parsing, and checksumming that `install.sh` / `install.ps1` rely
  on for in-place upgrades.

## [0.1.8] - 2026-07-13

### Added

- Onboarding discoverability and publication improvements (PR #9).
  Coverage for the first-time setup path and clearer errors when
  `doctor` finds an uninitialized project.

## [0.1.7] - 2026-07-12

### Fixed

- `royo-learn` now lists available subcommands when run without
  arguments instead of returning a generic error.

## [0.1.6] - 2026-07-12

### Added

- `install.sh` / `install.ps1` automatically add the install
  directory to the user `PATH` on Windows so the binary is
  invokable from a fresh shell.

## [0.1.0] - [0.1.5]

Earlier releases predate this changelog file. They exist as Git
tags in the remote repository (`RoyoTech/royo-learn`) but their
release notes have not been backfilled. To recover what changed
in any of these versions, inspect the tag and the merge commit
that introduced it, e.g.:

```bash
git log v0.1.4..v0.1.5 --oneline
git log v0.1.0..v0.1.1 --oneline
```

Backfilling these entries is a separate task; tracked outside this
file.

---

## Version ↔ Ola map (proposed 2026-07-23)

This project organizes work in three "olas" (waves) of capability
increments. Each ola is one or more PRs that ends in a release
tag. The map below is the proposed binding between olas and
semantic versions. It was added on 2026-07-23 alongside PR #19
and is **not yet ratified by a separate ADR**; treat it as a
working agreement until a future ADR formalizes it.

| Tag | Ola | PRs | What it buys |
|---|---|---|---|
| `v0.2.0-rc1` | (next) | #19 (docs) + the local main commits that close Hito 1 | Hito 1 ready for review; docs/20-26 + lessons in main |
| `v0.2.0` | Ola 1 | Hito 0 + 1 + 2 + 5 + 6 + 7 + 4 (7 PRs) | End-to-end experience loop: capture, validate, detect, cluster, promote, trace |
| `v0.3.0` | Ola 2 | Hito 8 + 9 + 3 + 10 (5 PRs) | Multi-agent, robust, searchable: jobs, FTS, OpenCode `--watch`, Claude Code, Codex |
| `v1.0.0` | Ola 3 | Hito 12 + 11 + Pi (3 PRs) | First production-ready version with optional semantics, drift/release hardening |

### Why this mapping

- **`v0.2.0` ↔ Ola 1** — the "salto de producto" defined in
  `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §37 is satisfied
  exactly when all 7 PRs land. It is the first time an agent can
  do the full experience loop.
- **`v0.3.0` ↔ Ola 2** — the robustness work (lease-based jobs,
  FTS, multi-agent adapters) changes the operational API. A minor
  bump reflects that.
- **`v1.0.0` ↔ Ola 3** — the moment the project commits to a
  stable contract for outside teams. Beyond `1.0.0`, breaking
  changes require a new major.

### Trigger → tag

| Trigger | Tag |
|---|---|
| PR #19 merged | (no tag — docs only) |
| Local main first contains all of Hito 1 + the documentation gap | `v0.2.0-rc1` |
| Ola 1 last PR (Hito 4) merged to remote main | `v0.2.0` |
| Ola 2 last PR (Hito 10 — Codex) merged to remote main | `v0.3.0` |
| Ola 3 last PR (Pi) merged to remote main | `v1.0.0` |

Until these triggers fire, the corresponding tag does not exist.
The trigger table is the source of truth for "are we ready for
the next tag"; the table above is the answer to "what does that
tag mean".

### Status as of 2026-07-23

- `v0.1.10` is the last released tag.
- Local `main` is 21 commits ahead of `origin/main` and contains
  all of Hito 1 (slices 1.A-1.D) plus the ADR-0002 §4 result and
  the Hito 1 handoff refresh.
- PR #19 (docs/20-26 + lessons + AGENTS/CLAUDE references) is open
  on `docs/grieta-20-26-clean` and is the next thing to land.
- Once PR #19 is merged and `main` is pushed, `v0.2.0-rc1` is the
  natural next tag.
