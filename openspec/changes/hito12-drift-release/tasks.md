# Tasks: Hito 12 — Drift / Release Hardening

> Strict TDD Mode is active. Test runner: `go test ./...` (CI: `go test -race ./...`).
> Every code task follows RED → GREEN → TRIANGULATE → REFACTOR.
> Every checkbox ends with exactly one ownership marker.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~710 (per proposal §"Intent"; matches design.md File-Level Changes table) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (drift core: T12.1–T12.6) → PR 2 (surface + parity: T12.7–T12.12) → PR 3 (release extras + docs: T12.13–T12.20) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |
| Correction budget | `min(200, ceil(710/2)) = 200` |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
```

> The orchestrator MUST ask the user to confirm the PR split and chain
> strategy (`stacked-to-main` vs `feature-branch-chain`) before launching
> `sdd-apply`. The three PR boundaries above are the **natural
> delivery points** based on the proposal's four deliverables and the
> design's per-package dependencies; they fit inside the 400-line
> per-PR budget.

---

## Phase 0 — Pre-flight (mechanical; no production code)

- [ ] **T12.0.1** Confirm working tree is clean and on base ref `main` at `3f1b112`. Run `git status --porcelain` (must be empty) and `git rev-parse HEAD` (must print `3f1b112`). Acceptance: both commands succeed; if either fails the orchestrator aborts. <!-- sdd-owner: implementation -->
- [ ] **T12.0.2** Verify Go toolchain + GoReleaser availability. `go version` prints ≥ the version pinned in `go.mod`; `which goreleaser` resolves; `goreleaser --version` is v2.17.0+. Acceptance: all three commands succeed; missing GoReleaser is a blocker for Phase 4. <!-- sdd-owner: implementation -->
- [ ] **T12.0.3** Confirm Hito 11 audit hook intact. `grep -n "RunOne" internal/experience/jobs/service.go` returns ≥ 1 match at line 512 area. Acceptance: signature `func (s *Service) RunOne(ctx, projectID, jobName, owner, j *semantic.Job) (JobRunOutcome, error)` is present (Phase 1 will reuse it). <!-- sdd-owner: implementation -->

---

## Phase 1 — Drift foundation (Deliverable 1; TDD estricto)

Each task in this phase follows RED → GREEN → TRIANGULATE → REFACTOR. T12.6 ships before T12.5 because the job references the constant.

### T12.1 — Migration `009_publication_drift.sql`

- [ ] **T12.1** Add migration 009 with idempotent forward-only DDL for `publication_drift_state`. <!-- sdd-owner: implementation -->
  - **RED** Write `TestMigrate_009_Forward` (creates fresh in-mem DB, applies 009, asserts table exists with documented columns + CHECK constraint + 3 indexes). Write `TestMigrate_009_Idempotent` (applies 009 twice, asserts success both times). Write `TestMigrate_009_ChecksumStable` (asserts the embedded SHA matches the file on disk).
  - **GREEN** Create `internal/storage/migrations/009_publication_drift.sql` with the schema in design.md §Data Model (PK `(publication_id, target_path)`, CHECK on the 4 enum values, FK to `publications(id)`, 3 indexes).
  - **TRIANGULATE** Add `TestMigrate_009_CheckConstraintRejectsUnknownStatus` (insert with `status='corrupted'` → expect CHECK violation).
  - **REFACTOR** Extract column list / indexes as Go constants in `migrate_test.go` for reuse; assert CHECK constraint regex matches `'(ok|drifted|target_missing|target_unreadable)'`.
  - **Acceptance** All 4 tests pass; `go test ./internal/storage/...` green; `tasks.md` Phase 1 contains the literal manual-rollback SQL recipe (asserted by `TestDrift_TasksDocumentRollback`):
    ```sql
    DROP INDEX IF EXISTS idx_drift_publication;
    DROP INDEX IF EXISTS idx_drift_run_id;
    DROP INDEX IF EXISTS idx_drift_status_checked;
    DROP TABLE IF EXISTS publication_drift_state;
    DELETE FROM schema_migrations WHERE version = 9;
    ```
  - **Dependencies** T12.0.1

### T12.6 — `semantic.JobIntentDrift` constant + `IsValidIntent` switch extension

- [ ] **T12.6** Extend the `JobIntent` enum with the `drift` value and accept it in the validator. <!-- sdd-owner: implementation -->
  - **RED** Write `TestJobIntent_DriftAccepted` (asserts `IsValidIntent(JobIntentDrift)` returns `true`); extend `TestJobIntent_KnownValues` with the new constant.
  - **GREEN** Edit `internal/experience/semantic/types.go`: add `JobIntentDrift JobIntent = "drift"` and extend the `IsValidIntent` switch. Update the doc comment from "JobIntentIngest is the only value used by Hito 11" to reflect the new constant (decision captured in design.md Open Questions).
  - **TRIANGULATE** Add `TestJobIntent_UnknownValueRejected` (already exists; verify the new switch rejects e.g. `"bogus"`).
  - **REFACTOR** No further refactor expected.
  - **Acceptance** `go test ./internal/experience/semantic/...` green; `validateTaxonomy` at `jobs/repository.go:271` accepts `"drift"`.
  - **Dependencies** T12.0.1

### T12.2 — Drift repository (`RecordDrift`, `ListDrift`)

- [ ] **T12.2** Implement the per-row upsert repository for `publication_drift_state`. <!-- sdd-owner: implementation -->
  - **RED** Write `TestRecordDrift_RoundTrip` (insert, read back, assert fields), `TestRecordDrift_UpsertOnConflict` (same `(publication_id, target_path)` updates in place), `TestRecordDrift_RejectsUnknownStatus` (CHECK violation).
  - **GREEN** Create `internal/publish/drift/repository.go` with `DriftRow` struct, `Repository.RecordDrift(ctx, row)` (per-row `*sql.Tx`, upsert SQL in design.md §B), and `Repository.ListDrift(ctx, filters)` returning rows for CLI/MCP.
  - **TRIANGULATE** Add `TestListDrift_FilterBySource` and `TestListDrift_FilterByRunID`.
  - **REFACTOR** Extract the upsert SQL into a package-level constant; add a `time.Now()` injection hook for test determinism.
  - **Acceptance** All 5 tests pass; CHECK constraint rejection is hit by the test, not silently caught.
  - **Dependencies** T12.1, T12.0.3

### T12.3 — `Checker.Check` with the 4 outcomes

- [ ] **T12.3** Implement `Checker.Check(ctx, target, expectedHash) (Result, error)` with the four outcomes. <!-- sdd-owner: implementation -->
  - **RED** Write `TestChecker_OKOnHashMatch`, `TestChecker_DriftedOnHashMismatch`, `TestChecker_TargetMissingReturnsErrTargetMissing` (use path under `testutil.TempDir(t)` that does not exist), `TestChecker_TargetUnreadableWrapsUnderlying` (use `os.Open` mock returning error after a successful `os.Stat`), `TestChecker_RespectsContextCancellation` (`ctx` already cancelled → `target_unreadable`).
  - **GREEN** Create `internal/publish/drift/checker.go` per design.md §A: `Status` enum (`StatusOK`, `StatusDrifted`, `StatusTargetMissing`, `StatusTargetUnreadable`), `Result` struct, sentinels `ErrTargetMissing` + `ErrTargetUnreadable`, `Checker` struct with injectable `openFn`, decision order `Stat → Open → sha256`.
  - **TRIANGULATE** Add `TestChecker_ActualHashIsHex` (asserts `len(Result.ActualHash) == 64` for the `ok` and `drifted` outcomes) and `TestChecker_LargeFileStreamingHash` (8 KiB random file matches the reference SHA-256).
  - **REFACTOR** Extract the SHA-256 streaming loop into a private `hashFile` helper; document the read-only invariant in the package doc comment.
  - **Acceptance** All 8 tests pass; `errors.Is(err, ErrTargetMissing)` returns `true` for the missing outcome; `errors.Is(err, ErrTargetUnreadable)` returns `true` for the unreadable outcome (use `errors.Join`).
  - **Dependencies** T12.0.1

### T12.4 — Read-only contract + pre-commit grep

- [ ] **T12.4** Lock the read-only invariant via contract test + lint guard. <!-- sdd-owner: implementation -->
  - **RED** Write `TestChecker_IsReadOnly` (snapshot `Mode() / ModTime() / Size()` before/after `Check`, assert byte-identical); write `TestChecker_NoWriteAPIsImported` (static grep over `internal/publish/drift/` for `os.WriteFile|os.Create|ioutil.WriteFile|os.Chtimes|os.Remove`, fail on any match).
  - **GREEN** No production change required (T12.3 already implements read-only). Wire the grep as a Go test using `exec.Command("grep", ...)` so CI catches regressions.
  - **TRIANGULATE** Write `TestChecker_PermissionDeniedOnPOSIX` (skipped on Windows via `runtime.GOOS` check; `chmod 0o000` → `target_unreadable`).
  - **REFACTOR** Extract the grep pattern list into a package-level var so CI scripts can import the same constant.
  - **Acceptance** Both tests pass; CI step `grep -rnE 'os\.WriteFile|os\.Create|ioutil\.WriteFile' internal/publish/drift/` exits 1 (zero matches); contract_test.go uses `testutil.TempDir(t)` + `t.Cleanup` (Hito 5/10 pattern).
  - **Dependencies** T12.3

### T12.5 — Job `publication_drift_check` with gate in `JobFunc` body

- [ ] **T12.5** Register and implement the `publication_drift_check` job with the `status='published'` gate in Go (not SQL alone). <!-- sdd-owner: implementation -->
  - **RED** Write `TestPublicationDriftCheck_SkipsInProgress` (insert two `publications` rows: one `published`, one `in_progress`; run the `JobFunc`; assert `publication_drift_state` grows by exactly one row, and the new row's `publication_id` is the published one). Write `TestPublicationDriftCheck_GateInJobFuncBody` (static grep proves `status != "published"` literal lives in the `runPublicationDriftCheck` body).
  - **GREEN** Create `internal/publish/drift/jobs.go` with `JobName = "publication_drift_check"`, `Job() *semantic.Job` accessor returning `&semantic.Job{Entry: JobRegistryEntry(), JobFunc: runPublicationDriftCheck}` (per design.md §D), and `runPublicationDriftCheck(d semantic.Deps)` per Decision D1 (SELECT publications, iterate, gate in Go with `if status != "published" { skipped++; continue }`, decode `targets_json`/`verification_json` blobs per Decision D2, call `Checker.Check`, upsert via `Repository.RecordDrift`).
  - **TRIANGULATE** Write `TestPublicationDriftCheck_AllFourOutcomes` (seed 4 rows → one per outcome → assert each `publication_drift_state` row has the expected status). Write `TestPublicationDriftCheck_AuditPayloadHasNoTargetPath` (assert no `audit_events` JSON payload contains `/home/alice` or `excerpt`).
  - **REFACTOR** Extract the row-decode logic into a private `decodePublicationRow` helper; document the gate invariant in the `JobFunc` doc comment.
  - **Acceptance** All 5 tests pass; default `default_interval_sec = 3600`, `default_max_retries = 3`, `Enabled = false`; pre-commit grep `grep -n "status = 'published'" internal/publish/drift/jobs.go` returns ≥ 1 match in `runPublicationDriftCheck` body.
  - **Dependencies** T12.2, T12.3, T12.4, T12.6

---

## Phase 2 — Unified surface (Deliverable 2; TDD estricto)

### T12.7 — Shared `driftHandler` + CLI subcommand `experience drift`

- [ ] **T12.7** Implement `driftHandler(ctx, db, sourceFilter)` shared by CLI and MCP, plus the CLI subcommand in `cmd/royo-learn/experience.go`. <!-- sdd-owner: implementation -->
  - **RED** Write `TestDriftHandler_GoldenEnvelope` (byte-equal JSON envelope, sorted before equality per design.md §API Contracts), `TestDriftHandler_FilterBySource` (`--source=claudecode` returns one `sources` entry), `TestDriftHandler_TargetPathIsBasename` (`filepath.Base(...)` invariant). At the CLI layer, write `TestExperienceDrift_GoldenEnvelope`, `TestExperienceDrift_FilterFlags`, `TestExperienceDrift_RejectsBothFlags` (exit 2 + usage on stderr when both `--all-sources` and `--source=` are passed).
  - **GREEN** Create `internal/experience/cli/drift.go` exporting `driftHandler` (per design.md Decision D5: applies `filepath.Base` redaction at the point of JSON construction). Extend `cmd/royo-learn/experience.go` with `runExperienceDrift(args []string) error` registered as the `experience drift` subcommand; flags `--all-sources` (default true), `--source=<opencode|claudecode|codex>`, `--json` (default true); mutually-exclusive validation returns exit 2.
  - **TRIANGULATE** Add `TestExperienceDrift_BothFlagsError` (asserts exit code 2 + stderr usage + no JSON on stdout).
  - **REFACTOR** Extract flag parsing into a `parseDriftFlags(args []string) (driftFilter, error)` helper; share the JSON-sorting comparator between unit and CLI tests.
  - **Acceptance** All 7 tests pass; envelope matches design.md §CLI JSON envelope (`sources` + `publications` top-level keys only); per-source `experience <source> scan` dispatchers remain reachable one minor version (design.md Decision D6).
  - **Dependencies** T12.5

### T12.8 — PII redaction in JSON output

- [ ] **T12.8** Lock the `target_path` redaction invariant via cross-platform fixture test. <!-- sdd-owner: implementation -->
  - **RED** Write `TestDriftCLI_NoPIIInOutput` (fixture with `target_path` values `/home/alice/x.jsonl`, `/Users/bob/y.jsonl`, `C:\Users\carol\z.jsonl`; render JSON; grep for `/home/`, `/Users/`, `C:\Users\`; assert zero matches). Write `TestDriftHandler_NoPIIInOutput` (handler-level counterpart).
  - **GREEN** No new code (T12.7 already applies `filepath.Base`). Wire the test fixture into `tests/testdata/drift_pii_fixtures/` if not already present.
  - **TRIANGULATE** Add a regression test that *forces* the redaction off (mocking `driftHandler` to emit full paths) and asserts the test fails; then restore.
  - **REFACTOR** Extract the PII marker set into a package-level var so tests and any future CLI/MCP variants reuse it.
  - **Acceptance** All 3 tests pass on linux/amd64, windows-latest, macos-latest in CI; failure message names the offending marker substring.
  - **Dependencies** T12.7

### T12.9 — MCP tool `experience_drift_status`

- [ ] **T12.9** Register the MCP tool and assert envelope parity with CLI. <!-- sdd-owner: implementation -->
  - **RED** Write `TestExperienceDriftStatus_Schema` (asserts tool registered, `RequiredProfile == "admin"`, `InputSchema` accepts `{}`), `TestExperienceDriftStatus_EnvelopeParity` (MCP handler output byte-equals CLI output modulo sort), `TestExperienceDriftStatus_ZeroPII` (re-runs PII markers against MCP output).
  - **GREEN** Extend `internal/mcp/experience.go` with the tool registration per design.md §F (name `"experience_drift_status"`, description, admin profile, empty input schema); handler delegates to the same `driftHandler` from T12.7.
  - **TRIANGULATE** Add `TestExperienceDriftStatus_NonAdminProfileRejected` (asserts the tool refuses non-admin profiles via the existing profile gate).
  - **REFACTOR** Extract tool-definition boilerplate (name, description, schema) into a helper if more drift-flavoured tools follow.
  - **Acceptance** All 4 tests pass; envelope parity golden fixture is byte-equal after sort.
  - **Dependencies** T12.7, T12.8

---

## Phase 3 — Adapter parity (Deliverable 3; TDD estricto)

### T12.10 — Remove advisory excerpt branch from `claudecode/resolve_trace.go`

- [ ] **T12.10** Reconcile `claudecode/resolve_trace.go` with the `opencode` / `codex` policy: no excerpt, no advisory field. <!-- sdd-owner: implementation -->
  - **RED** Write `TestResolveTrace_SourceChanged_OmitsExcerpt` in `internal/experience/claudecode/resolve_trace_test.go` (reuses Hito 10 fixtures; asserts `result.Excerpt == ""` AND `result.Advisory == false` on `trace_source_changed` AND `trace_source_unavailable` outcomes). Write `TestResolveTrace_SourceChanged_OmitsAdvisoryField` (asserts the same after the branch is removed).
  - **GREEN** Edit `internal/experience/claudecode/resolve_trace.go` lines 100–119: remove the `if excerpt != "" { result.Excerpt = excerpt; result.Advisory = true }` block; replace with the nil-out shape used by `opencode/resolve_trace.go` (return `ResolveTraceResult{Status: ..., Code: "trace_source_changed"}, nil` without the excerpt).
  - **TRIANGULATE** Add a CI grep test `TestClaudecode_ResolveTrace_NoAdvisoryTrue` asserting `grep -n "Advisory: true" internal/experience/claudecode/resolve_trace.go` returns zero matches.
  - **REFACTOR** Audit `result.Advisory` field usage across the package; document it as reserved (`false` for every outcome) in the type's doc comment.
  - **Acceptance** All 3 tests pass; Hito 10 SEVERE trace-leak invariant preserved (`docs/24-EXPERIENCE-THREAT-MODEL.md` §6); existing Hito 10 fixtures untouched.
  - **Dependencies** T12.0.1

### T12.11 — Parity tests on `opencode` and `codex`

- [ ] **T12.11** Pin the parity invariant on the two already-conforming adapters. <!-- sdd-owner: implementation -->
  - **RED** (Both adapters already return `Excerpt == ""` on drift; tests should fail today only if a regression is introduced, which is the goal.) Write `TestResolveTrace_SourceChanged_OmitsExcerpt` in `internal/experience/opencode/resolve_trace_test.go` and `internal/experience/codex/resolve_trace_test.go` (same shape as T12.10's claudecode test).
  - **GREEN** No production change required; the tests pin the invariant.
  - **TRIANGULATE** Add a per-adapter `TestResolveTrace_SourceUnavailable_OmitsExcerpt` covering the `trace_source_unavailable` outcome (deleted-locator fixture).
  - **REFACTOR** Extract the parity-assertion boilerplate into a shared helper under `internal/experience/testsupport/` if Go's test framework allows cross-package helpers; otherwise leave as 3 inline tests for review simplicity.
  - **Acceptance** All 6 tests pass (3 adapters × 2 outcomes); the same fixture used by Hito 10 is reused (no new fixture files).
  - **Dependencies** T12.10

### T12.12 — Tighten `docs/22-ADAPTER-CONTRACT.md`

- [ ] **T12.12** Lift and tighten Scenario "Source changes or disappears" to cross-adapter scope. <!-- sdd-owner: implementation -->
  - **RED** Write `TestAdapterContract_ParitySentence` (reads `docs/22-ADAPTER-CONTRACT.md`, asserts the literal `All three adapters MUST return no excerpt` appears adjacent to the scenario header).
  - **GREEN** Edit `docs/22-ADAPTER-CONTRACT.md`: move the scenario out of the Codex §11 sub-section; append the tightening sentence; reference `internal/experience/{opencode,claudecode,codex}/resolve_trace_test.go::TestResolveTrace_SourceChanged_OmitsExcerpt` as the executable assertion.
  - **TRIANGULATE** Add a sibling test asserting the file lists the three adapter package names beside the scenario.
  - **REFACTOR** None.
  - **Acceptance** Test passes; doc remains additive; no other scenario is altered.
  - **Dependencies** T12.10, T12.11

---

## Phase 4 — Release extras (Deliverable 4; TDD where applicable)

### T12.13 — SBOM emission in `.goreleaser.yml`

- [ ] **T12.13** Add `sboms:` block producing `*.spdx.json` alongside archives. <!-- sdd-owner: implementation -->
  - **RED** Write `TestGoReleaserSnapshot_ProducesSBOM` in `tests/release/goreleaser_snapshot_test.go` (runs `goreleaser release --snapshot --clean` in `t.TempDir()`, asserts at least one `*.spdx.json` exists next to `*.tar.gz` / `*.zip`). Write `TestGoReleaser_YAML_HasSbomsBlock` (parses `.goreleaser.yml` with `gopkg.in/yaml.v3`; asserts `sboms` key exists with `formats: ['spdx-json']`).
  - **GREEN** Edit `.goreleaser.yml`: add the `sboms:` block per design.md §H (top-level per design.md Open Questions resolution). Verify `goreleaser release --snapshot --clean` locally before commit.
  - **TRIANGULATE** If `spdx-json` is rejected on the local GoReleaser version, the test fails on `*.spdx.json`; record the actual emitted format and update the test to accept the alternative (per `release-extras` spec scenario "SBOM format unsupported declares the gap in RELEASE.md"). Defer to T12.14.
  - **REFACTOR** Extract the YAML field paths into constants if multiple snapshot tests follow.
  - **Acceptance** Both tests pass on linux/amd64; the snapshot output contains at least one `*.spdx.json` (or the alternative format and a `Known Limitations` row in `RELEASE.md`).
  - **Dependencies** T12.0.2

### T12.14 — `RELEASE.md` runbook

- [ ] **T12.14** Create the self-contained, 5-section release runbook at repo root. <!-- sdd-owner: implementation -->
  - **RED** Write `TestReleaseRunbook_HasFiveSections` (static grep asserts the literals `## Trigger table`, `## Required CI checks`, `## Tag creation`, `## Install verification`, `## Rollback recipe` are all present) and `TestReleaseRunbook_RollbackReferencesUninstall` (asserts `install.sh --uninstall` literal is in the rollback section).
  - **GREEN** Create `RELEASE.md` at repo root with the five sections in order per `release-extras` spec requirement; each section contains actionable prose (no `TODO` placeholders). The "Rollback recipe" section names the previous tag as the reinstall target and the SHA-256 verification command as the reinstall guard.
  - **TRIANGULATE** Add `TestReleaseRunbook_SelfContained` (asserts no step requires opening `docs/15-OPERATIONS.md` — i.e. no `[operations](../docs/15-OPERATIONS.md)` style required link in the prose, only additive footnotes allowed).
  - **REFACTOR** None.
  - **Acceptance** All 3 tests pass; runbook is executable top-to-bottom with only `RELEASE.md` open.
  - **Dependencies** T12.13

### T12.15 — Link from `docs/15-OPERATIONS.md`

- [ ] **T12.15** Add a "Release runbook" section to the operations doc. <!-- sdd-owner: implementation -->
  - **RED** Write `TestOperationsDoc_LinksToReleaseRunbook` (asserts both `grep -n "Release runbook" docs/15-OPERATIONS.md` and `grep -n "RELEASE.md" docs/15-OPERATIONS.md` return ≥ 1 match).
  - **GREEN** Edit `docs/15-OPERATIONS.md`: add an additive section "Release runbook" containing one sentence + the Markdown link `[Release runbook](../RELEASE.md)`. Existing sections unchanged.
  - **TRIANGULATE** None.
  - **REFACTOR** None.
  - **Acceptance** Test passes; doc remains additive.
  - **Dependencies** T12.14

### T12.16 — `CHANGELOG.md` backfill + v1.0.0 ⏳ demotion

- [ ] **T12.16** Backfill Hitos 8/9/10/11 entries; demote the `v1.0.0` ⏳ marker to "no tag yet". <!-- sdd-owner: implementation -->
  - **RED** Write `TestChangelog_Backfilled` (asserts each Hito 8/9/10/11 PR title appears under `[0.8.0]` / `[0.9.0]` / `[0.10.0]` / `[0.11.0]` and is **absent** from `[Unreleased]`). Write `TestChangelog_V100NoTagYet` (asserts `v1.0.0` section contains the literal `no tag yet`, references `RELEASE.md`, and contains no `20YY-MM-DD` ISO date).
  - **GREEN** Read `git log --oneline` for the merge SHAs of Hitos 8/9/10/11 (CI runs with `fetch-depth: 0`), extract PR titles, move `[Unreleased]` entries into their respective release sections with `[^pr-N]: #N` footnotes. Rewrite the `v1.0.0` ⏳ marker as a clearly worded "no tag yet" section referencing `RELEASE.md`.
  - **TRIANGULATE** Add `TestChangelog_UnreleasedOnlyHasHito12` (asserts `[Unreleased]` contains only genuinely unreleased entries).
  - **REFACTOR** None.
  - **Acceptance** All 3 tests pass; `[Unreleased]` is honest about what has not yet shipped.
  - **Dependencies** T12.14

---

## Phase 5 — Final verification

- [ ] **T12.17** Coverage gates. Run `go test -cover ./internal/publish/drift/...`, `./internal/experience/semantic/...`, and `./internal/experience/{opencode,claudecode,codex}/...`; thresholds per design.md Test Strategy and `docs/25` §4 (`internal/publish/drift/` ≥ 90%, `semantic/` ≥ 90% post-extension, adapters ≥ 85% maintained). Acceptance: all three report above threshold; CI fails otherwise. <!-- sdd-owner: implementation -->
- [ ] **T12.18** Cross-build + race. `GOOS=windows GOARCH=amd64 go build ./cmd/royo-learn`; same for `linux/amd64` and `darwin/universal` (or `darwin/arm64` + `amd64`). Then `go test -race ./...` on `linux/amd64`. Acceptance: all four builds succeed; `-race` run passes with no data-race reports. <!-- sdd-owner: implementation -->
- [ ] **T12.19** Roadmap update. Edit `docs/26-IMPLEMENTATION-ROADMAP.md` §5: mark the Hito 12 row as "in flight" with a Markdown link to `openspec/changes/hito12-drift-release/proposal.md`. Acceptance: file diff is additive only; `grep -n "in flight" docs/26-IMPLEMENTATION-ROADMAP.md` returns ≥ 1 match. <!-- sdd-owner: implementation -->
- [ ] **T12.20** Acceptance matrix update. Add Hito 12 rows to `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 (4 drift outcomes + unified CLI/MCP envelope + adapter parity) and to `docs/14-ACCEPTANCE-CRITERIA.md` §E. Acceptance: each new row references at least one test from the design.md Test Strategy tables. <!-- sdd-owner: implementation -->

---

## Parent-only actions (gated review; not implementation)

> These are NOT to be claimed by `sdd-apply`. They run after the parent's
> chosen PR slice is green and stable.

- [ ] **POST-APPLY-REVIEW** After `sdd-apply` reports the chosen PR slice complete, the orchestrator runs `gentle-ai review start` (only when no valid receipt exists) and walks the native review lifecycle per the authority-first terminal procedure; never reset the correction budget. <!-- sdd-owner: parent -->
- [ ] **PRE-COMMIT-VALIDATE** Before any commit, the orchestrator stages every reviewed path without content/mode changes and runs `gentle-ai review validate --gate pre-commit --cwd <repo> --lineage <known-lineage>`; never create a new review budget here. <!-- sdd-owner: parent -->
- [ ] **PR-SCOPE-DECISION** If the user opts for chained PRs, the orchestrator triggers `gentle-ai-chained-pr` per the cached `chain_strategy` (`stacked-to-main` vs `feature-branch-chain`); if `pending`, ASK the user before launching `sdd-apply`. <!-- sdd-owner: parent -->
- [ ] **VERIFY-GATE** After all three PRs merge, run `/sdd-verify hito12-drift-release` and address every CRITICAL finding before `/sdd-archive`. <!-- sdd-owner: parent -->

---

## Out-of-scope reminders (do NOT spawn child subagents for these)

- v1.0.0 tag creation (separate operator decision).
- Code signing / `cosign` signatures (follow-up change).
- `Dependabot` / `Renovate` config (follow-up change).
- Drift retention policy / TTL (Hito 13 follow-up).
- Cross-adapter `ScanResult` shape unification (already deferred in Hito 11).

## Constraints (apply to every task)

- Every task is **committable independently** as a conventional commit.
- Tests before implementation (RED first); CI gates: `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.
- No `continue-on-error` in CI.
- Cross-build gates for Windows/Linux/macOS are mandatory, not skippable.
- Coverage gate for `internal/publish/drift/` is ≥ 90% (proposal §Success Criteria).
