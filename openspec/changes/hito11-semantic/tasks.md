# Tasks: Hito 11 — Semantic / Symmetric Job Engine

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,250 (17 created, 12 modified, 0 deleted; design lists 30 files) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR #13 (semantic package + migration) → PR #14 (jobs engine rewrite + audit hook) → PR #15 (per-adapter `Job()` rewrite + CLI collapse) |
| Delivery strategy | chained (locked from preflight) |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

The design enumerates 30 files (17 created, 12 modified, 0 deleted). The
single-PR diff would land around 1,250 LOC plus the SQL migration plus the
`exp` collapse flag. Three reviewable PRs make the diff focused and let
the orchestrator stop between slices for the `gentle-ai
review validate --gate pre-commit`/`pre-push` invariant.

## PR Plan

### PR #13 — `semantic` package + `008_job_semantics.sql` migration

- **Title**: `feat(semantic): new semantic package + job taxonomy migration 008`
- **Branch**: `feat/hito11-pr13-semantic-package`
- **Tracker branch**: `feat/hito11-semantic` (draft, no-merge)
- **Target branch**: `feat/hito11-semantic`
- **Approximate changed lines**: ~430 (new package + migration + tests)
- **Phases inside**: 1, 2, 3
- **Acceptance criteria**:
  - [x] `go build ./internal/experience/semantic/...` clean on Win/Linux/macOS
    (verified cross-build on this WSL: `GOOS=windows`, `GOOS=darwin`, `GOOS=linux` all exit 0;
    package has no OS-specific bindings).
  - [x] `internal/storage/migrations/008_job_semantics.sql` applies
    idempotently on a fresh SQLite DB; the migration test asserts
    forward apply, idempotency, and checksum stability. The runner
    is forward-only; the manual-rollback SQL is documented in
    `tasks.md` Phase 1 for operator use (REQ-JSE-4 Scenario).
    Verified: `TestMigrate_008_Forward` (0.08s), `TestMigrate_008_Idempotent`
    (0.06s), `TestMigrate_008_ChecksumStable` (0.07s) all PASS.
  - [x] `JobIntent`/`JobScope`/`JobRiskClass` constants exported and
    `IsValidX` helpers reject unknown values (REQ-JSE-2). All 9 enum tests
    PASS (known values + unknown rejected + IsValid consistency).
  - [x] `EventJob*` constants exported and `jobPayload` allow-list emits
    only the documented keys (REQ-JSE-3, REQ-JSE-6). **Implementation
    delta**: 8 engine-owned keys (`job_name`, `run_id`, `source`, `state`,
    `transition`, `attempt`, `error_code`, `error_message`) instead of the
    originally-spec'd 7. `transition` is added with an explicit comment at
    `internal/experience/semantic/events.go:50-54` documenting it as
    "required for PR #14's audit hook". The allow-list remains strict
    (no free-form fields); REQ-JSE-6's transcript-leak invariant is
    preserved. Tasks.md originally said "7 documented keys" — this is
    updated to reflect implementation reality.
  - [x] `internal/experience/semantic/` coverage ≥ 90% per `docs/25` §4.
    Verified: 100.0% of statements.

### PR #14 — `jobs.Service.RunOne` + audit hook + repository taxonomy

- **Title**: `feat(jobs): RunOne audit hook + job_run_log + repository taxonomy`
- **Branch**: `feat/hito11-pr14-jobs-runone`
- **Tracker branch**: `feat/hito11-semantic`
- **Target branch**: `feat/hito11-pr13-semantic-package` (then rebase into
  tracker once #13 merges)
- **Approximate changed lines**: ~520 (engine rewrite + helper plumbing +
  tests)
- **Phases inside**: 4, 5, 6
- **Acceptance criteria**:
  - [ ] `Service.RunOne` emits exactly four `job_*` audit events per run
    in one `*sql.Tx`, sharing one `run_id` (REQ-JSE-3, REQ-JSE-5).
  - [ ] `TestAuditHook_DoesNotLeakTranscriptText` passes (Hito 10 SEVERE
    invariant preserved).
  - [ ] `Repository.RecordRunLog`/`UpdateRunLogAttempt`/`FinishRunLog`
    round-trip; `UpsertRegistryEntry` populates `intent`/`scope`/`risk_class`
    and rejects unknown values (REQ-JSE-4, REQ-EA-2).
  - [ ] `internal/experience/jobs/` retains ≥ 94% coverage.
  - [ ] `TestRunOne_PerAdapterPathNoDirectAuditCall` greps the three
    per-adapter files and finds zero `RecordEventTx` calls.

### PR #15 — Per-adapter `Job()` accessors + CLI collapse

- **Title**: `feat(experience): per-adapter Job() accessor + unified experience scan`
- **Branch**: `feat/hito11-pr15-cli-collapse`
- **Tracker branch**: `feat/hito11-semantic`
- **Target branch**: `feat/hito11-pr14-jobs-runone` (then rebase into
  tracker once #14 merges)
- **Approximate changed lines**: ~300 (rewrites + dispatcher + tests)
- **Phases inside**: 7, 8, 9, 10
- **Acceptance criteria**:
  - [ ] All three adapters expose `Job() *semantic.Job` with matching
    `Source`; compile-time contract test `TestAdapter_ImplementsContract`
    stays green (REQ-EA-1 MODIFIED).
  - [ ] Per-adapter `Job()` tests (`TestOpencodeJob_*`,
    `TestClaudecodeJob_*`, `TestCodexJob_*`) pass (REQ-EA-1 ADDED).
  - [ ] `experience scan --source=<value>` routes to the correct adapter
    `Job()`; missing/invalid `--source` returns exit code 2 (REQ-ECC-1,
    REQ-ECC-2).
  - [ ] JSON envelope shape parity with legacy form preserved byte-for-byte
    (REQ-ECC-5).
  - [ ] `--experimental-cli-collapse` ldflags default ON; OFF produces
    `DEPRECATED:` stderr note on legacy call (REQ-ECC-3, REQ-ECC-4).
  - [ ] `docs/04-CLI-SPEC.md` documents `--source` flag and the unified
    subcommand; `docs/14-ACCEPTANCE-CRITERIA.md` §E adds the audit-event
    and migration rows.
  - [ ] Three per-adapter `internal/experience/{opencode,claudecode,codex}`
    packages report ≥ 85% coverage each.

## Phases

### Phase 1: Migration 008 + manual rollback recipe (Foundation)

- **Goal**: Ship the `008_job_semantics.sql` migration, with
  idempotency + checksum-stability tests. The runner is forward-only;
  the rollback recipe is documented in `tasks.md` (not auto-applied).
- **Files**: Create
  `internal/storage/migrations/008_job_semantics.sql`;
  create or modify `internal/storage/migrate_test.go` (in the
  storage package).
- **Test**: `TestMigrate_008_Forward`, `TestMigrate_008_Idempotent`,
  `TestMigrate_008_ChecksumStable`. There is NO reverse test
  because the runner does not implement down-migrations.
- **Acceptance**: REQ-JSE-4 (idempotent forward + manual rollback recipe
  documented in this phase's `Manual rollback recipe` block below).
- **Depends on**: none.
- **PR**: PR #13.
- **Status**: complete (PR #13, sdd-apply batch 1).

#### Manual rollback recipe (operator use only)

The runner (`internal/storage/migrate.go`) is forward-only. For
emergency rollback, the operator runs these statements manually
against the DB after `git revert`:

```sql
DROP INDEX IF EXISTS idx_job_run_log_run_id;
DROP INDEX IF EXISTS idx_job_run_log_job_started;
DROP TABLE IF EXISTS job_run_log;

ALTER TABLE job_registry RENAME TO job_registry_pre_008;
CREATE TABLE job_registry (
    job_name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    default_interval_sec INTEGER NOT NULL DEFAULT 3600,
    default_max_retries INTEGER NOT NULL DEFAULT 3,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
INSERT INTO job_registry (job_name, description, default_interval_sec,
    default_max_retries, enabled, created_at)
SELECT job_name, description, default_interval_sec, default_max_retries,
    enabled, created_at FROM job_registry_pre_008;
DROP TABLE job_registry_pre_008;

DELETE FROM schema_migrations WHERE version = 8;
```

This recipe is not auto-applied; it lives in `tasks.md` so the
operator can copy it when needed.

### Phase 2: `semantic` package — types, events, Job contract (Foundation)

- **Goal**: Create the new package with the enum types, audit constants,
  payload allow-list, and the `Job`/`JobFunc`/`JobResult`/`Deps` runtime
  contract.
- **Files**: Create
  `internal/experience/semantic/types.go`,
  `internal/experience/semantic/events.go`,
  `internal/experience/semantic/job.go`,
  `internal/experience/semantic/types_test.go`,
  `internal/experience/semantic/events_test.go`,
  `internal/experience/semantic/job_test.go`.
- **Test**: `TestJobIntent_KnownValues`, `TestJobIntent_UnknownRejected`,
  `TestJobScope_KnownValues`, `TestJobScope_UnknownRejected`,
  `TestJobRiskClass_KnownValues`, `TestJobRiskClass_UnknownRejected`,
  `TestJobPayload_AllowListContract`, `TestJobPayload_RejectsForbiddenKeys`,
  `TestJob_ContractCompiles`, `TestJobFunc_RespectsContextCancellation`.
- **Acceptance**: REQ-JSE-1, REQ-JSE-2, REQ-JSE-3, REQ-JSE-5, REQ-JSE-6.
- **Depends on**: Phase 1.
- **PR**: PR #13.
- **Status**: complete (PR #13, sdd-apply batch 1).

### Phase 3: `JobRegistryEntry` struct gains taxonomy fields (Foundation)

- **Goal**: Extend the `JobRegistryEntry` struct in `internal/experience/jobs/types.go`
  with `Intent`, `Scope`, `RiskClass` typed fields and add validation
  helper calls in preparation for the Phase 4 repository extension.
- **Files**: Modify `internal/experience/jobs/types.go`.
- **Test**: covered by `TestUpsertRegistryEntry_PopulatesThreeColumns`
  added in Phase 5.
- **Acceptance**: REQ-JSE-2 (types align with semantic enums).
- **Depends on**: Phase 2.
- **PR**: PR #13.
- **Status**: complete (PR #13, sdd-apply batch 1). The new
  `Validate()` helper is also covered by `TestJobRegistryEntry_Validate_*`
  in the new `internal/experience/jobs/types_test.go`.

### Phase 4: `jobs.Service.RunOne` + audit hook (Core Implementation)

- **Goal**: Add `RunOne(ctx, projectID, jobName, owner, *semantic.Job)` as a
  sibling to `RunDue`, owning the lease + audit + `job_run_log` transaction
  that emits exactly four events per run.
- **Files**: Create
  `internal/experience/jobs/jobs.go` (new `JobRunOutcome` + `jobRunLog`),
  modify `internal/experience/jobs/service.go` (add `RunOne`,
  `recordJobAuditTx`, `buildRecordEventTx`), modify
  `internal/experience/jobs/service_test.go`.
- **Test**: `TestRunOne_Leases`, `TestRunOne_EmitsFourEvents`,
  `TestRunOne_FailureEmitsJobFailed`, `TestRunOne_LeaseConflictSkips`,
  `TestRunOne_CancellationHonoured`, `TestRunOne_RunIDsAreUnique`,
  `TestRunOne_PayloadAllowList`, `TestRunOne_PerAdapterPathNoDirectAuditCall`,
  `TestAuditHook_DoesNotLeakTranscriptText`.
- **Acceptance**: REQ-JSE-3, REQ-JSE-5, REQ-JSE-6, REQ-EA-2.
- **Depends on**: Phase 2, Phase 3.
- **PR**: PR #14.

### Phase 5: Repository gains run-log + taxonomy upsert (Core Implementation)

- **Goal**: Add `RecordRunLog`, `UpdateRunLogAttempt`, `FinishRunLog` for
  the new `job_run_log` table; extend `UpsertRegistryEntry` to write
  `intent`/`scope`/`risk_class` and reject unknown values.
- **Files**: Modify `internal/experience/jobs/repository.go`; modify
  `internal/experience/jobs/repository_test.go`.
- **Test**: `TestRecordRunLog_RoundTrip`,
  `TestUpsertRegistryEntry_PopulatesThreeColumns`,
  `TestUpsertRegistryEntry_RejectsUnknownTaxonomy`,
  `TestUpsertRegistryEntry_RejectsUnknownRiskClass`.
- **Acceptance**: REQ-JSE-2, REQ-JSE-4, REQ-EA-2.
- **Depends on**: Phase 1, Phase 3.
- **PR**: PR #14.

### Phase 6: Integration coverage — three end-to-end RunOne paths (Testing)

- **Goal**: Add real-DB integration tests proving `RunOne` flows end-to-end
  for each static adapter, asserting four events + one run-log row.
- **Files**: Modify `internal/experience/jobs/service_test.go` (or new
  `integration_test.go` in same package).
- **Test**: `TestRunOne_EndToEnd_OpenCode`, `TestRunOne_EndToEnd_Codex`,
  `TestRunOne_EndToEnd_ClaudeCode`.
- **Acceptance**: REQ-JSE-3, REQ-JSE-5, REQ-EA-1.
- **Depends on**: Phase 4, Phase 5.
- **PR**: PR #14.

### Phase 7: Per-adapter `Job()` rewrite (Integration / Wiring)

- **Goal**: Rewrite each per-adapter `jobs.go` to expose `Job()
  *semantic.Job`; the legacy static constructor becomes an unexported
  `newIngestJobRegistryEntry` helper called inside the `JobFunc` body.
  OpenCode gets a `jobs.go` for the first time.
- **Files**: Create
  `internal/experience/opencode/jobs.go`,
  `internal/experience/opencode/jobs_test.go`; modify
  `internal/experience/claudecode/jobs.go`,
  `internal/experience/claudecode/jobs_test.go`,
  `internal/experience/codex/jobs.go`,
  `internal/experience/codex/jobs_test.go`.
- **Test**: `TestOpencodeJob_AccessorReturnsTypedJob`,
  `TestOpencodeJob_SourceMatches`, `TestOpencodeJob_DistinctPerCall`,
  `TestClaudecodeJob_*`, `TestCodexJob_*`.
- **Acceptance**: REQ-EA-1 MODIFIED + ADDED, REQ-EA-2.
- **Depends on**: Phase 4, Phase 5.
- **PR**: PR #15.

### Phase 8: CLI collapse flag + dispatcher rewrite (Integration / Wiring)

- **Goal**: Add the build-time `--experimental-cli-collapse` ldflags
  helper with env-var override; rewrite `cmd/royo-learn/experience.go`
  to expose `runExperienceUnified` and route legacy subcommands when the
  flag is off.
- **Files**: Create
  `internal/experience/cli/collapse.go`,
  `internal/experience/cli/collapse_test.go`; modify
  `cmd/royo-learn/experience.go`; create
  `cmd/royo-learn/experience_unified_test.go`.
- **Test**: `TestCollapseFlag_DefaultsToOn`,
  `TestCollapseFlag_EnvOverride`,
  `TestExperienceUnified_MissingSource`,
  `TestExperienceUnified_InvalidSource`,
  `TestExperienceUnified_OpenCodeAccepted`,
  `TestExperienceUnified_CodexAccepted`,
  `TestExperienceUnified_ClaudecodeAccepted`,
  `TestExperienceUnified_OutputKeysParity`,
  `TestExperienceUnified_DeprecationNote_CollapseOff`,
  `TestExperienceUnified_NoDeprecationNote_Unified`.
- **Acceptance**: REQ-ECC-1, REQ-ECC-2, REQ-ECC-3, REQ-ECC-4, REQ-ECC-5.
- **Depends on**: Phase 7.
- **PR**: PR #15.

### Phase 9: E2E bin tests (Testing)

- **Goal**: Build the CLI binary and assert the unified form + the legacy
  collapse-off path produce the documented JSON and stderr.
- **Files**: Modify `cmd/royo-learn/e2e_test.go` (existing file).
- **Test**: `TestExperienceScanUnified_OpenCode`,
  `TestExperienceScanUnified_Codex`,
  `TestExperienceScanUnified_Claudecode`,
  `TestExperienceScanUnified_Deprecation`.
- **Acceptance**: REQ-ECC-1, REQ-ECC-4.
- **Depends on**: Phase 8.
- **PR**: PR #15.

### Phase 10: Documentation closeout (Cleanup)

- **Goal**: Mark PR #14 row in `docs/26` §5 as "in flight" with link to
  `openspec/changes/hito11-semantic`; add the `--source` flag and unified
  subcommand to `docs/04-CLI-SPEC.md`; add the audit-event + migration
  acceptance rows to `docs/14-ACCEPTANCE-CRITERIA.md` §E.
- **Files**: Modify `docs/04-CLI-SPEC.md`,
  `docs/14-ACCEPTANCE-CRITERIA.md`, `docs/26-IMPLEMENTATION-ROADMAP.md`.
- **Test**: docs-only; no Go test. `gofmt -l .` and `go vet ./...` must
  stay clean.
- **Acceptance**: proposal Success Criteria #11.
- **Depends on**: Phase 9.
- **PR**: PR #15.

## Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| WU-1 | Land migration 008 + idempotency + checksum-stable test (no reverse) | PR #13 (complete, batch 1) | `go test ./internal/storage/ -run TestMigrate_008 -v` | `cmd/royo-learn` boot against a fresh test DB; the test embeds a real SQLite and asserts `pragma_table_info` + the runner's recorded SHA-256 | `git revert` PR #13 removes the SQL file; operator applies the manual-rollback recipe in `tasks.md` Phase 1 to drop the table + columns + `schema_migrations` row |
| WU-2 | Land `semantic` package + types + events + tests | PR #13 (complete, batch 1) | `go test -race ./internal/experience/semantic/... -cover` | `go test ./internal/experience/semantic/...` runs the full enum + payload allow-list suite | `git rm -r internal/experience/semantic/` removes the new package without affecting other paths |
| WU-3 | Engineer `jobs.Service.RunOne` + audit hook helpers | PR #14 | `go test -race ./internal/experience/jobs/ -run TestRunOne -v` | `TestRunOne_EndToEnd_OpenCode` boots a real SQLite + real adapter; asserts 4 audit rows + 1 run-log row | `git revert` PR #14 restores the static `RunDue`-only engine; the diff is contained to `jobs/service.go` + `jobs/jobs.go` |
| WU-4 | Repository run-log + taxonomy upsert + repository tests | PR #14 | `go test ./internal/experience/jobs/ -run TestUpsertRegistryEntry -v` | `TestRecordRunLog_RoundTrip` boots a fresh SQLite; inserts a row and reads it back | `git revert` PR #14 reverts `UpsertRegistryEntry` to the Hito 10 5-column shape; the new columns are dropped by the manual-rollback recipe in `tasks.md` Phase 1 |
| WU-5 | End-to-end RunOne integration tests for three adapters | PR #14 | `go test -race ./internal/experience/jobs/ -run TestRunOne_EndToEnd -v` | each test boots a real DB + real adapter; asserts 4 audit events + 1 run-log row | tests are additive; revert removes the test files only |
| WU-6 | Per-adapter `Job()` accessor + per-adapter tests | PR #15 | `go test -race ./internal/experience/opencode/... ./internal/experience/claudecode/... ./internal/experience/codex/...` | `TestOpencodeJob_AccessorReturnsTypedJob` etc. cover each adapter's `Job()` accessor and the `Source` invariant | `git revert` PR #15 restores the static `JobRegistryEntry()` constructors; the three adapters compile under the legacy shape |
| WU-7 | CLI collapse flag + dispatcher rewrite + dispatcher tests | PR #15 | `go test ./cmd/royo-learn/ -run TestExperienceUnified -v` | builds the CLI binary; runs the unified form against a fixture; asserts stdout JSON shape and stderr deprecation note | `git revert` PR #15 restores the three per-source dispatchers; the dispatcher code is local to `cmd/royo-learn/experience.go` |
| WU-8 | Audit-event terminal invariant: no transcript text in payloads | PR #14 | `go test -race ./internal/experience/jobs/ -run TestAuditHook_DoesNotLeakTranscriptText -v` | fixture envelope with `LEAK_CANARY_USER` / `LEAK_CANARY_ASSISTANT`; the test loads the four rows and greps the payload JSON | the test is a CLI-grade contract; revert removes the test without code change |
| WU-9 | E2E binary tests for unified form + deprecation note | PR #15 | `go test ./cmd/royo-learn/ -run TestExperienceScanUnified -v` | builds the binary via `go build -o /tmp/royo-learn ./cmd/royo-learn`; runs the CLI against a fixture | tests are additive; revert removes the test file only |
| WU-10 | Documentation closeout (docs/04, docs/14, docs/26) | PR #15 | `gofmt -l .` empty; `go vet ./...` clean | not applicable (docs only) | revert the three doc files independently |
| WU-11 | Coverage gate validation pass | PR #15 | `bash scripts/coverage_check.sh` (must hit `docs/25` §4 thresholds) | runs each package's coverage; `internal/experience/semantic` ≥ 90%, `internal/experience/jobs` ≥ 94%, per-adapter ≥ 85% | the gate is a CI check; failure blocks PR; coverage threshold is a parameter, not a code change |
| WU-12 | `gentle-ai review` lifecycle for the tracker branch | PR #15 (final) | `gentle-ai review validate --gate pre-commit --cwd <repo> --lineage experience-hito11-semantic-v1` | runs the native CLI; the orchestrator alone executes the lifecycle commands | the receipt is a content-bound artifact; superseded receipts invalidate without rollback |

## Coverage Targets

Per `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §4:

- `internal/experience/semantic/` ≥ 90% (new package — the proposal's
  dominant risk coverage gate).
- `internal/experience/jobs/` retains ≥ 94% (current baseline; new
  `RunOne` + audit-hook paths are heavily tested).
- Each per-adapter package (`opencode`, `claudecode`, `codex`) ≥ 85%.
- `internal/experience/cli/` ≥ 90% (new subpackage; ldflags helper + env
  override).
- `cmd/royo-learn/` retains ≥ 80% (dispatcher rewrite).

The coverage gate is enforced by `scripts/coverage_check.sh` and run during
the `gentle-ai review finalize` wrapper. Any sub-threshold is a SEVERE
blocker for the matching PR.

## Risk Mitigation Tasks

| # | Risk | Mitigation task |
|---|------|-----------------|
| 1 | Migration 008 lacks a runner-driven rollback | Phase 1: the manual-rollback recipe is documented in `tasks.md` Phase 1; `TestMigrate_008_ChecksumStable` asserts the runner's SHA-256 path is stable across re-applies. |
| 2 | Audit-event emission duplicates existing `experience_turn_ingested` / `experience_session_discovered` events | Phase 4 + Phase 6: `TestRunOne_EmitsFourEvents` + `TestRunOne_PerAdapterPathNoDirectAuditCall` (greps the three per-adapter files for `RecordEventTx` and asserts zero matches). |
| 3 | Backwards-incompatible CLI change breaks operator muscle memory | Phase 8: `--experimental-cli-collapse` ldflags default ON; `TestExperienceUnified_DeprecationNote_CollapseOff` proves the legacy form still works with a `DEPRECATED:` note during the migration window. |
| 4 | `JobFunc` closure capture drift across the three adapters | Phase 7: `TestOpencodeJob_DistinctPerCall` (and claudecode/codex variants) assert two consecutive `Job()` calls return distinct pointers. |
| 5 | Coverage drop in `internal/experience/jobs/` | Coverage gate runs in `gentle-ai review finalize`; PR #14 cannot merge below 94%. |
| 6 | Migration breaks the receipt gate from `session-2026-07-28-hito10-codex.md` | The migration is small (~30 LOC SQL) and isolated; the `min(200, ceil(changed_lines/2))` correction budget covers the full Hito 11 diff at 1,250 LOC. |
| 7 | `gentle-ai finalize` strict schema rejects reviewer-emitted fields | The reviewer agents emit only `id`/`location`/`severity`/`claim`/`proof_refs` + `findings`/`evidence` (`evidence` is `[]string`); the orchestrator reshapes before `gentle-ai review capture-result` (per `hito10-codex-review-fixes.md`). |
| 8 | Reasoning or `function_call_output` text leaks via the audit hook (Hito 10 SEVERE) | Phase 4: `TestAuditHook_DoesNotLeakTranscriptText` uses fixture `LEAK_CANARY_USER` / `LEAK_CANARY_ASSISTANT` sentinels; the `jobPayload` allow-list enforces the engine-only key set. |
| 9 | Hito 3 (`--watch`) silently flips `Enabled = true` when the schema gains the columns | Phase 5: `TestUpsertRegistryEntry_PopulatesThreeColumns` asserts `Enabled == false`; the per-adapter `Job()` accessors pass `Enabled: false` from the unexported `newIngestJobRegistryEntry` helper. |
| 10 | Per-adapter `jobs.go` rewrite breaks Hito 10 contract tests | Phase 7: the legacy `JobRegistryEntry()` remains as an unexported helper; `TestAdapter_ImplementsContract` stays untouched. |
| 11 | Skipped-counter drift across the three adapters | Out of scope: Hito 11 does not unify `ScanResult`; the per-adapter shape stays. Follow-up tracked in `docs/26` §5. |
| 12 | Cursor parsing fork (OpenCode `int64` vs Claude Code/Codex string-UUID) | Out of scope: Hito 11 does not touch `cursor.go`. |

## Pre-Apply Checklist

- [ ] Working tree clean (`git status` shows no uncommitted changes
      outside `openspec/changes/hito11-semantic/`).
- [ ] Branch cut from `origin/main` (not local `main`) per
      `docs/lessons.md` entry #4 (the
      `royo-learn-deploy` push recipe).
- [ ] Tracker branch `feat/hito11-semantic` created from `origin/main`
      and pushed as draft (`gh pr create --draft`).
- [ ] PR base verified: `git rev-list --count origin/main..main` returns
      `0` — the orchestrator must call this before PR #13 base check.
- [ ] `openspec/changes/hito10-codex/` archived (verify-report created
      and task moved to `openspec/changes/archive/2026-07-28-hito10-codex/`).
      **Blocker if not archived** — the `experience-hito10-codex-v1`
      receipt must be terminal before the new `experience-hito11-semantic-v1`
      lineage is bootstrapped.
- [ ] CI green: `go vet ./...`, `go test -race ./...`, `gofmt -l .`
      empty.
- [ ] Coverage baseline measured before any change —
      `bash scripts/coverage_check.sh` produces the recorded baseline
      for `internal/experience/jobs/` (94%+) and three per-adapter
      packages.
- [ ] `internal/experience/semantic/` does not exist yet (the new
      package is additive; any pre-existing file is a blocker).
- [ ] `migrations/008_job_semantics.sql` does not exist yet (the path
      `internal/storage/migrations/008_job_semantics.sql` is unused).
- [ ] `internal/experience/opencode/jobs.go` does not exist yet (Hito 11
      introduces it; pre-existing file is a Pre-Apply blocker).
- [ ] `internal/experience/cli/` package does not exist yet (the new
      subpackage is additive; pre-existing package is a blocker).
- [ ] Operator can run
      `GIT_SSH_COMMAND="ssh -i /tmp/royo-learn-deploy-key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new" git push -u origin feat/hito11-semantic`
      per the `royo-learn-deploy-key` recipe in
      `docs/lessons.md` + `AGENTS.md` § "Workflow de push a GitHub".
