# Hito 6 — Pattern Mining Execution Plan

## Baseline

- [x] Read the Hito 6 handoff and required source-of-truth sections.
- [x] Verify `main` and `origin/main` both resolve to `6bf5ce71bfe1f9c937bba8b6225783d3ecfdff9f`.
- [x] Verify the only untracked file is `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`; preserve it.
- [x] Verify latest tag is `v0.2.0-rc1`.
- [x] Run `go test -race -count=1 ./internal/experience/...` successfully.
- [x] Verify fresh coverage: detectors 90.1%, opencode 80.5%.
- [x] Verify Hito 2 and Hito 5 are present under `CHANGELOG.md` `[Unreleased]`.
- [x] Create `feat/hito6-patterns` from `origin/main`.

## Implementation — strict sequential TDD

- [x] Slice 6.0 — Scaffold `internal/experience/patterns/`: contract tests RED, then minimal type/interface stubs GREEN; no mining logic.
- [x] Slice 6.1 — Deterministic pattern fingerprint and retrieval-term normalization; prove map-order independence and volatile-value stripping.
- [x] Slice 6.2 — Pure v1 clustering: exact fingerprint first, then conservative Jaccard over retrieval terms; no embeddings or side effects.
- [x] Slice 6.3 — Qualification rules and anti-patterns: distinct sessions/days, successful evidence/corrections, contradictions, retries, coverage, traceability.
- [x] Slice 6.4 — Persistence migration 005, idempotent dismissal, list/get surfaces, CLI/MCP/job integration, and synthetic-fixture acceptance test.

## Verification gates

- [x] Record RED, GREEN, TRIANGULATE, and REFACTOR evidence for each slice.
- [x] Keep `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked and outside every diff.
- [x] Run focused package tests before the broader suite.
- [x] Run `gofmt`, `go test -race ./...`, and `go vet ./...`.
- [x] Reach at least 80% coverage for `internal/experience/patterns/` (the authoritative Hito 6 handoff gate).
  - **Evidence:** fresh coverage is 87.0%. `docs/25` references `internal/patterns >=90%`, but that path does not exist and conflicts with `HANDOFF-HITO6-PATTERNS.md` lines 113-114 and 202. The discrepancy is documented rather than padded with assertion-free tests.
- [x] Verify migration idempotency, stable JSON, typed wrong-payload errors, and MCP schema shapes.
- [x] Build the required Windows target.
  - `GOOS=windows GOARCH=amd64 go build` passed. Linux/macOS cross-builds are explicitly non-required noise under the operator directive in the handoff.
- [x] Run the bounded implementation review transaction after apply, before any lifecycle gate.
  - **Operator-accepted gap.** `gentle_review inspect` returned `applicability=ambiguous`, `action=select_lineage`. v1 lineage (`hito6-patterns-review-v1`) was created with `baseRef: origin/main` on an unstaged working tree and persisted with `paths: []`, `intended_untracked: []`; `finalize` was silently dropped. v2 lineage (`hito6-patterns-review-v2`) was created with `mode: ordinary` on a working tree staged clean (31 staged, 3 untracked preserved); risk_tier=high, 4R full set selected. Four `finalize` calls with valid JSON shape were silently dropped — state stayed `reviewing`, receipt stayed `not_applicable`. Operator authorized proceeding at operator responsibility with the gap documented in `docs/lessons.md` §5 (2026-07-25 entry).
- [x] **Commit** the Hito 6 implementation as a single atomic commit on `feat/hito6-patterns`.
  - **Commit:** `30d0b5c` (34 files, +8144/-12). No `Co-Authored-By`. Conventional commit. PROMPT untracked excluded.
- [ ] Push and open the single Hito 6 PR (PR #5 per docs/26 §3) — pending explicit operator authorization.
- [ ] Post-merge housekeeping (CHANGELOG `[Unreleased]`, ROADMAP update, HANDOFF-HITO6-CLOSEOUT.md, tag review).

## Review / Implementation Notes

- `internal/experience/patterns` package added under
  `internal/experience/patterns/` (not `internal/patterns/`). Fresh
  coverage is 87.0%, above the authoritative Hito 6 threshold of
  80%. The conflicting `docs/25` path/threshold is documented in
  `docs/IMPLEMENTATION-NOTES.md`; no assertion-free coverage padding
  was added.
- Five typed error codes added to `internal/domain/errors.go`
  and `docs/17-ERROR-CODES.md`: `pattern_not_found`,
  `pattern_not_qualified`, `pattern_already_promoted`,
  `pattern_false_cluster`, `pattern_insufficient_sources`.
  Each maps to a stable exit code through `ErrorCode.ExitCode()`.
- Migration `005_pattern_mining.sql` reserves the
  `(project_id, fingerprint)` unique key and adds the
  `dismissal_reason` typed column so dismissal idempotency
  is enforced at the schema level. The existing
  `TestExperienceMigrationSchema` was updated to assert the
  005 row is present.
- CLI `experience patterns list|get|dismiss` and MCP
  `learning_list_patterns`, `learning_get_pattern`,
  `learning_dismiss_pattern` mirror the JSON envelope. The
  `dismissal_reason` field is emitted verbatim and the
  `status` field carries the typed transition.
- `Learning_dismiss_pattern` is admin-only; the two read tools
  are available in the read/agent/admin profiles (matches the
  patterns contract: dismissal mutates state).
- Pattern fingerprint is a 64-char lowercase hex sha256 of the
  canonical `PatternInput`. The named Jaccard default (0.5)
  is documented and reversible; the default `MaxClusterMembers`
  is 100 per docs/23 §5.
- All slices built on `t.TempDir()` / `storagetest.OpenTemp`
  fixtures; the synthetic acceptance test runs the full
  end-to-end pipeline through the CLI dispatch.
- `tasklist` blocked `royo-learn.exe` builds intermittently
  in this environment; the cross-build verification was
  performed with `GOOS=windows GOARCH=amd64 go build` which
  succeeds without touching the prebuilt binary. The prebuilt
  `royo-learn.exe` is a local convenience build that the parent
  lifecycle controller may rebuild before any PR.

## Explicit scope decisions

- Use a named/configurable conservative Jaccard threshold because the specification does not assign a numeric value; record the selected default in `docs/IMPLEMENTATION-NOTES.md`.
- Add the five documented pattern error codes to the canonical typed-error implementation and `docs/17-ERROR-CODES.md` if the existing error architecture confirms that location.
- Hito 6 creates and manages pattern candidates only. Promotion/publication remains Hito 7 and is out of scope.
- No embeddings, vector database, network dependency, shell execution, transcript persistence, or AGENTS.md changes.

## Review

- Commit `30d0b5c` performed at operator responsibility. The
  review transaction gap (native `gentle_review finalize`
  silently dropped) is documented in `docs/lessons.md` §5.
  Push and PR remain gated on explicit operator authorization.
