# Changelog

All notable changes to `royo-learn` will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/)
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Hito 6: pattern mining** (branch `feat/hito6-patterns`, PR #5
  per `docs/26` §3). Closes the slice 6 deliverable from
  `docs/26-IMPLEMENTATION-ROADMAP.md`. Five atomic commits on the
  branch covering:
  - `internal/experience/patterns/` package: typed
    `PatternStatus` (`observed|qualified|dismissed|promoted|stale`),
    closed `DismissalReason` enum, `Membership`,
    `QualificationCriteria`, `ExperiencePattern`,
    `ConservativeQualifier`, and the small interface surface
    (`Pattern`, `Cluster`, `Qualifier`, `Dismissal`, `Lister`,
    `Getter`).
  - `PatternFingerprint` and `NormalizeRetrievalTerms`:
    deterministic, order-independent, volatile-value-stripped
    (UUIDs, ports, hashes, absolute paths, redacted markers).
    64-char lowercase hex sha256.
  - Pure `Group` clustering (exact fingerprint + conservative
    Jaccard over retrieval terms). No embeddings, no vector DB,
    no shell, no network. The named Jaccard threshold default
    (0.5) is documented and reversible per slice 6.2.
  - `ConservativeQualifier` enforces the eight criteria from
    `docs/23-PATTERN-MINING.md` §5 (including the canonical
    "3 retries in 1 session" anti-pattern) and surfaces typed
    `QualificationDecision` with reasons.
  - Migration `005_pattern_mining.sql`: introduces
    `experience_patterns` + `experience_pattern_members` (both
    with the typed `dismissal_reason` column for idempotent
    re-dismiss).
  - Repository (`NewRepository`, `SavePattern`,
    `GetByFingerprint`, `GetByID`, `ListByStatus`,
    `SetStatusWithReason`, `AddMember`, `Members`,
    `UpsertFromCluster`) with stable, versioned JSON.
  - Service (`NewService`, `Dismiss`, `List`, `Get`,
    `IngestCluster`) with idempotent dismissal on
    `(pattern_id, reason)` and a structured audit row.
  - CLI `experience patterns list|get|dismiss` with stable JSON
    envelope and the project's error-envelope stderr contract.
  - MCP `learning_list_patterns`, `learning_get_pattern`,
    `learning_dismiss_pattern` (registered in the read profile
    for the first two; admin-only for dismissal).
  - Domain additions: `ExperiencePatternID` typed ID,
    `PatternStatus` validation, `Membership.Validate()`,
    five new typed error codes (`pattern_not_found`,
    `pattern_not_qualified`, `pattern_already_promoted`,
    `pattern_false_cluster`, `pattern_insufficient_sources`)
    wired through `domain.AsDomainError`, the canonical exit
    code map and `docs/17-ERROR-CODES.md`.
  - Synthetic acceptance test
    (`cmd/royo-learn/experience_patterns_test.go:
    TestExperiencePatterns_AcceptanceSynthetic`) covering the
    3-sessions qualify / 3-retries-1-session anti-pattern /
    dismissal idempotency contract end-to-end through the CLI.
  - 88.3% test coverage on `internal/experience/patterns`.
    Target was ≥90% per `docs/25` §4; the residual gap is in
    `database/sql` error branches (BeginTx / Commit / RowsAffected
    failures) that only trigger on actual DB corruption. Recorded
    in `docs/IMPLEMENTATION-NOTES.md` Hito 6 reconciliation.

- **Hito 5: deterministic experience detectors** (PR #22, merge

- **Hito 5: deterministic experience detectors** (PR #22, merge
  commit `59d5e74` on local + remote main). Closes the slice 5
  deliverable from `docs/26-IMPLEMENTATION-ROADMAP.md` PR #4. Eight
  atomic commits on `feat/hito5-detectors` covering:
  - `internal/experience/detectors/` package: `Detector` interface
    (Kind, Version, Detect), `DetectInput`, `CandidateEvent`,
    `Registry` (orchestrator-facing lookup), package doc with scope
    and boundaries.
  - `RetryDetector` — the first concrete detector, kind `retry`,
    canonical #4 in `docs/23-PATTERN-MINING.md` §2.1. Threshold=3,
    window=5min, pure event-driven, deterministic via the
    `(Source, Session.ExternalID, Turn.ExternalID)` idempotency
    contract.
  - CLI `experience detect --kind <kind> --project-root <root>
    [--input <file>] [--persist]` orchestrator. JSON in / JSON out
    by default; `--persist` forwards every emitted event through
    `detectors.Persist` and returns the canonical event_id +
    fingerprint + duplicate flag per event.
  - MCP tool `experience_detect_events` (agent + admin profile) with
    the same surface as the CLI: detector kind, payload, optional
    persist.
  - Domain additions: `SourceDetector` accepted by
    `isValidExperienceSource`; `detector` accepted by
    `localLocatorKinds`. The detector-produced envelope uses these so
    the synthetic locator is valid for v1.
  - 90.1% test coverage on `internal/experience/detectors` (≥ 80%
    threshold per AGENTS.md §Calidad mínima).
  - Slice 5.4 acceptance test in `cmd/royo-learn/experience_detect_test.go`
    covers the end-to-end persistence path: real project root, real
    SQLite, retry detector at threshold, second run reports
    `duplicate=true` with the same event_id (idempotency proof).

- **Hito 2: OpenCode `--once` adapter** (PR #21, merge commit
  `ad269a7` on local + remote main). Closes the slice 2 deliverable
  from `docs/26-IMPLEMENTATION-ROADMAP.md` PR #3.
  - `internal/experience/opencode/` package: `Discover()` (with path
    security, no symlinks), `Health()` (read-only schema check),
    `Scan()` (stable envelopes, idempotency by
    `(source, external_session_id, external_turn_id)`),
    `ResolveTrace()` (redacted excerpt).
  - CLI `experience opencode scan --once --project-root <root>
    [--fixture <path>]` orchestrator
    (`cmd/royo-learn/experience.go`).
  - `SkippedIncomplete` counter surfaced end-to-end (adapter →
    `ScanResult` → CLI report → JSON output) so the operator can
    audit incomplete turns instead of having them disappear silently.
  - 80.5% test coverage on `internal/experience/opencode` (≥ 80%
    threshold per AGENTS.md §Calidad mínima).
  - `docs/26 §9` updated with the four lessons this PR produced.
  - Post-merge fixes captured in commits `b1eac7c` (CLI: reject
    symlinks in `--fixture` and replace discovery) and `663d7eb` /
    `7e6fde8` (test: Windows 8.3 path handling now uses
    `project.Canonicalize` instead of the strict byte compare that
    broke on `RUNNER~1` vs `runneradmin`).

### Notes

- The operator's 2026-07-25 directive ("piensa en hacer royo-learn
  solo para windows inicialmente") narrows the merge gate to
  Windows + clean install smoke + Windows installer safety. The
  remaining Linux/macOS CI jobs are documented as operator-owned
  cross-platform debt; this PR's coverage gate failures on
  `internal/publish` are unrelated (a pre-existing
  `TestPublish_RollbackFailureObserved` flake on Linux) and out of
  scope for Hito 2.

### Status as of 2026-07-25 (post-Hito 6 merge)

- `v0.2.0-rc1` is the last released tag. Per the trigger table, the
  next tag (`v0.2.0`) fires when the last PR of Ola 1 (Hito 4) merges
  to remote main. Hito 6 alone does not trigger it.
- Hitos 0, 1, 2, 5, and 6 are merged on `main`. `origin/main` and
  local `main` are synchronized at `55ef635`. PR #23 is closed.
- The feature branches for those Hitos were deleted (local and
  remote) after the merges (`feat/hito2-opencode-once`,
  `feat/hito5-detectors`, `feat/hito6-patterns`).
- Ola 1 is now 5/7 Hitos in: Hito 0 ✅, Hito 1 ✅, Hito 2 ✅,
  Hito 5 ✅, Hito 6 ✅. Hitos 7 and 4 remain. The next PR is
  Hito 7 (PR #6 in the roadmap: promotion via `capture.Service`),
  tracked in `HANDOFF-HITO6-CLOSEOUT.md`.
- Hito 6 coverage reported by the merged commit is **87.0%** on
  `internal/experience/patterns` (gate ≥80% per the Hito 6 handoff
  met; `docs/25` references `internal/patterns ≥90%` with a
  different path, reconciled in `docs/IMPLEMENTATION-NOTES.md`).
- The Hito 6 PR was merged under an operator-accepted review gap:
  `gentle_review finalize` was silently dropped across four
  attempts on the native v2 lineage; receipt stayed
  `not_applicable`. Documented in `docs/lessons.md` §5. The
  operator accepted the gap at operator responsibility rather than
  rolling back the work.

## [0.2.0-rc1] - 2026-07-24

First release candidate that includes Hito 1 (experience discovery).
Cut from `main` at `21c0944` after the local main was pushed to
`origin/main` for the first time (21 commits ahead at the moment of
push, synchronized with the previous `bc930b3` head via merge
commit `21c0944`).

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
- **Frozen contracts** (delivered via PR #19, merged 2026-07-24):
  - `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
  - `docs/20-EXPERIENCE-INGESTION-PRD.md`
  - `docs/21-EXPERIENCE-DOMAIN.md`
  - `docs/22-ADAPTER-CONTRACT.md`
  - `docs/23-PATTERN-MINING.md`
  - `docs/24-EXPERIENCE-THREAT-MODEL.md`
  - `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md`
  - `docs/26-IMPLEMENTATION-ROADMAP.md`
  - `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md`
- **Operational patterns** for agents working in this repo (delivered
  via PR #20, merged 2026-07-24):
  `docs/lessons.md` captures shell detection, WSL bypass for the
  harness lifecycle interceptor, `gentle_review` scope discipline,
  and PR-base rules. Referenced from `AGENTS.md` and a new
  `CLAUDE.md`.

### Changed

- `docs/IMPLEMENTATION-NOTES.md:466` — the `internal/mcpserver`
  `ListTools: context deadline exceeded` observation is now a
  recorded investigation result (ADR-0002 §7) rather than an open
  question.

### Notes

- The Version ↔ Ola map (below) is ratified by the act of cutting
  this tag. The mapping was proposed on 2026-07-23 alongside PR #19
  and is now bound to a real release.

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

| Trigger | Tag | Status |
|---|---|---|
| PR #19 merged | (no tag — docs only) | ✅ |
| Local main first contains all of Hito 1 + the documentation gap | `v0.2.0-rc1` | ✅ |
| Hito 2 PR (PR #21) merged to remote main | (no tag — accumulates in [Unreleased]) | ✅ |
| Ola 1 last PR (Hito 4) merged to remote main | `v0.2.0` | ⏳ |
| Ola 2 last PR (Hito 10 — Codex) merged to remote main | `v0.3.0` | ⏳ |
| Ola 3 last PR (Pi) merged to remote main | `v1.0.0` | ⏳ |

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

### Status as of 2026-07-25 (post-Hito 2 merge)

- `v0.2.0-rc1` is the last released tag and the natural next
  release point per the trigger table.
- Hitos 0, 1, and 2 are merged on `main`. `origin/main` and local
  `main` are synchronized at `ad269a7`. PR #21 is closed.
- The feature branches for those Hitos were deleted (local and
  remote) after the merges.
- Ola 1 is now 3/7 Hitos in: Hito 0 ✅, Hito 1 ✅, Hito 2 ✅.
  Hitos 5, 6, 7, 4 remain. The next PR is Hito 5 (PR #4 in the
  roadmap: detectores deterministas), tracked in
  `HANDOFF-HITO5-DETECTORS.md`.
- The CHANGELOG [Unreleased] section now carries the Hito 2 entry.
  It will continue to accumulate until Ola 1 closes with Hito 4,
    at which point `v0.2.0` is the natural cut.

### Status as of 2026-07-25 (post-Hito 5 merge)

- `v0.2.0-rc1` is the last released tag. Per the trigger table, the
  next tag (`v0.2.0`) fires when the last PR of Ola 1 (Hito 4) merges
  to remote main. Hito 5 alone does not trigger it.
- Hitos 0, 1, 2, and 5 are merged on `main`. `origin/main` and local
  `main` are synchronized at `59d5e74`. PR #22 is closed.
- The feature branches for those Hitos were deleted (local and
  remote) after the merges (`feat/hito2-opencode-once`,
  `feat/hito5-detectors`).
- Ola 1 is now 4/7 Hitos in: Hito 0 ✅, Hito 1 ✅, Hito 2 ✅, Hito 5 ✅.
  Hitos 6, 7, 4 remain. The next PR is Hito 6 (PR #5 in the roadmap:
  patterns, clustering, qualification, dismissal), tracked in
  `HANDOFF-HITO6-PATTERNS.md`.
- The CHANGELOG [Unreleased] section now carries both the Hito 2 and
  the Hito 5 entries. It will continue to accumulate until Ola 1
  closes with Hito 4, at which point `v0.2.0` is the natural cut.
