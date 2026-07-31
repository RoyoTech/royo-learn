# Proposal: Hito 11 — Semantic / Symmetric Job Engine

## Intent

The three experience adapters (`opencode`, `claudecode`, `codex`) are
mechanically symmetric — same 5-method `ExperienceAdapter` interface,
same envelope shape, same per-source static `JobRegistryEntry`. But
they ship duplicated `SourceInstance`, `ScanRequest`, `ScanResult`,
`TraceBounds`, `TraceResult`, and `HealthResult` types, and the
per-adapter `jobs.go` files are static constructors with no shared
runtime semantics. The result: the operator cannot ask one question
about "what is the job engine doing right now across all three
sources" — every source needs its own console query, and every
transition (`pending → running → succeeded/failed`) lives only in
memory (`RunResult`) instead of in the append-only audit stream
(`docs/24-TM` §6).

This change delivers the "motor de jobs simétrico" promised by
`docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14: one neutral `Job` /
`JobResult` contract in `internal/experience/semantic/` that all
three adapters implement through a shared `JobFunc` return type;
semantic metadata (`intent`, `scope`, `risk_class`) on every job row;
and the four job-lifecycle transitions emitted to the existing
audit sink.

## Scope

### In Scope

- New package `internal/experience/semantic/` holding the enum types
  `JobIntent`, `JobScope`, `JobRiskClass` and audit-event-name
  constants (`job_pending`, `job_running`, `job_succeeded`,
  `job_failed`).
- New neutral `Job` / `JobResult` / `JobTransition` types in
  `internal/experience/semantic/job.go` that bind a `JobRegistryEntry`
  to a runtime `JobFunc` (`func(ctx, deps) (Result, error)`).
- Per-adapter `jobs.go` rewrite: each of
  `internal/experience/{opencode,claudecode,codex}/jobs.go` exposes a
  `Job() *semantic.Job` accessor (replacing the static constructor),
  returning a `Job` whose `Source` matches the adapter.
- Collapse `cmd/royo-learn/experience.go`: replace the three
  per-source subcommands (`experience opencode scan`,
  `experience claudecode scan`, `experience codex scan`) with one
  unified `experience scan --source=<opencode|claudecode|codex>`
  subcommand. Backwards-incompatible (call-out below).
- New SQL migration `migrations/008_job_semantics.sql` adding
  `intent TEXT`, `scope TEXT`, `risk_class TEXT` columns to
  `job_registry` AND a new `job_run_log` table
  (`run_id`, `job_name`, `state`, `started_at`, `finished_at`,
  `error_code`, `error_message`, `attempt`).
- Audit hook: `internal/experience/jobs.Service.RunOne` calls
  `storage.RecordEventTx` with the four event names above, exactly
  once per transition, on the same transaction that updates
  `job_state`. Reuses the existing append-only sink
  (`internal/storage/repo_audit.go`) — no schema change to
  `audit_events`.
- Migration of the static `JobRegistryEntry` rows
  (`experience_ingest:opencode`, `:claude_code`, `:codex`) to
  populate `intent = "ingest"`, `scope = "project"`,
  `risk_class = "low"` at upsert time.

### Out of Scope

- `--watch` flip on any of the three jobs (still `Enabled = false`;
  Hito 3 / PR #10 owns the flip).
- New event-operation names for the audit sink: `job_*` operations
  are added by direct `RecordEventTx` call with `Operation` string
  parameter (the helper is generic).
- MCP tool surface for `experience jobs`: deferred to a follow-up so
  the MCP stays minimal in v1.
- Removal of per-source `cmd/royo-learn/experience_<source>.go`
  helpers — they collapse into `runExperienceUnified` but the
  helpers themselves are renamed, not deleted, to keep `git blame`
  continuous.
- Skipped-counter symmetry (`SkippedIncomplete` /
  `SkippedMalformed` shape): addressed in a follow-up; this change
  does not collapse the per-source `ScanResult` struct.
- Re-architecting the audit invariant from `docs/24-TM` §6: the
  hook reuses `storage.RecordEventTx` verbatim.

## Capabilities

### New Capabilities

- `job-semantic-engine`: covers the `Job` / `JobResult` contract,
  the `JobIntent` / `JobScope` / `JobRiskClass` taxonomy, the
  `job_run_log` migration, and the audit-hook emission of the four
  lifecycle events through `storage.RecordEventTx`.
- `experience-cli-collapse`: covers the unified
  `experience scan --source=<opencode|claudecode|codex>` CLI surface
  and the removal of the per-source subcommands.

### Modified Capabilities

- `experience-adapters` (current delta under `hito10-codex`):
  the per-adapter `jobs.go` constructors stop returning
  `JobRegistryEntry` and start returning `*semantic.Job`; the
  schema tag rule and the 5-method `ExperienceAdapter` interface
  are unchanged. This change produces a delta spec under
  `openspec/changes/hito11-semantic/specs/experience-adapters/`.

## Approach

Per exploration §6 recommendation (Approach A):

1. **Taxonomy**: place `JobIntent`, `JobScope`, `JobRiskClass`, and
   the four audit-event-name constants in a new package
   `internal/experience/semantic/`. Per the locked rule, NOT in
   `internal/domain` (the domain stays neutral) and NOT in
   `internal/experience/jobs/` (that package owns the lease
   engine, not the taxonomy).
2. **Neutral job contract**: introduce
   `internal/experience/semantic/job.go` with the runtime shape
   `JobFunc func(ctx context.Context, deps semantic.Deps) (Result, error)`,
   where `Result` carries `Envelopes []experience.ExperienceEnvelope`,
   `SkippedMalformed int`, `SkippedIncomplete int`, `NextCursor string`,
   and `ErrorCode string`.
3. **Per-adapter wiring**: rewrite
   `internal/experience/{opencode,claudecode,codex}/jobs.go` so each
   exposes `Job() *semantic.Job`. The `JobFunc` body wraps the
   existing 5-method `ExperienceAdapter` calls
   (Discover → Health → Scan → `service.IngestEnvelope`), so the
   per-adapter compile-time contract tests in
   `codex/adapter_test.go` etc. keep passing.
4. **Audit hook**: `internal/experience/jobs/service.go` gains a
   new `RunOne(ctx, project, jobName, owner)` method (sibling to
   the existing `RunDue`). It acquires the lease, calls the
   `JobFunc`, emits the four audit events in a single transaction
   around the lease + `job_state` update, and releases.
   `Repository.RecordRunLog(ctx, tx, log)` writes to the new
   `job_run_log` table.
5. **CLI collapse**: `cmd/royo-learn/experience.go` replaces its
   three `runExperience<Source>` dispatchers with one
   `runExperienceUnified(args)` that parses `--source`, validates
   the value against `domain.ExperienceSource` constants, and
   delegates to the source's `Job()` accessor.
6. **Migration**: `internal/storage/migrations/008_job_semantics.sql`
   is one idempotent forward-only file (`CREATE TABLE IF NOT EXISTS`,
   `ALTER TABLE ... ADD COLUMN` guarded by name check). The existing
   runner (`internal/storage/migrate.go`) is forward-only — it has
   no down-migration mechanism. Rollback is `git revert` plus the
   manual SQL recipe documented in `tasks.md` Phase 1.
7. **Test coverage**: every new package must hit 90%+ per
   `docs/25` §4 (`internal/experience` row).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/experience/semantic/` | New | New package: enum types, audit-event constants, `Job`/`Result`/`JobFunc` contract |
| `internal/experience/jobs/service.go` | Modified | Adds `RunOne(ctx, project, jobName, owner)` and audit-hook emission |
| `internal/experience/jobs/repository.go` | Modified | Adds `RecordRunLog(ctx, tx, log)` and column-aware upsert on `job_registry` |
| `internal/experience/opencode/jobs.go` | Modified | Replaces static `JobRegistryEntry` with `Job() *semantic.Job` |
| `internal/experience/claudecode/jobs.go` | Modified | Same as above |
| `internal/experience/codex/jobs.go` | Modified | Same as above |
| `cmd/royo-learn/experience.go` | Modified | Collapses per-source dispatchers into `runExperienceUnified`; adds `--source` flag |
| `internal/storage/migrations/008_job_semantics.sql` | New | Adds `intent`, `scope`, `risk_class` columns + `job_run_log` table |
| `internal/storage/migrations/manual-rollback-008.sql` (recipe) | Documented | Manual DROP recipe for emergency rollback (runner is forward-only) |
| `internal/storage/repo_audit.go` | Referenced | `RecordEventTx` is reused verbatim; no schema change |
| `docs/04-CLI-SPEC.md` | Modified (additive only) | Documents new `--source` flag and the unified subcommand |
| `docs/05-MCP-SPEC.md` | Referenced | MCP surface unchanged; note deferred to follow-up |
| `docs/14-ACCEPTANCE-CRITERIA.md` §E | Modified (additive only) | Adds acceptance rows for the four audit events + migration test |
| `docs/21-EXPERIENCE-DOMAIN.md` §8 | Referenced | `JobState` contract unchanged; new fields live on `JobRegistryEntry` only |
| `docs/22-ADAPTER-CONTRACT.md` §1 | Referenced | `ExperienceAdapter` interface unchanged |
| `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 Hito 11 | Modified (additive only) | Adds acceptance rows for `job-semantic-engine` |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14 | Modified | Marks PR #14 row as "in flight" with link to this proposal |
| `openspec/specs/experience-adapters/` (delta) | Modified | Adds delta under `openspec/changes/hito11-semantic/specs/` |
| `openspec/specs/job-semantic-engine/` | New | Full spec for the new capability |
| `openspec/specs/experience-cli-collapse/` | New | Full spec for the new capability |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Migration 008 lacks a runner-driven rollback | Medium | The migration is one self-contained `008_job_semantics.sql`; the runner (`internal/storage/migrate.go`) is forward-only and has no down-migration mechanism; `internal/storage/migrate_test.go` asserts forward apply + idempotency + checksum integrity; the manual-rollback SQL is documented in `tasks.md` Phase 1 for operator use |
| Audit-event emission duplicates the existing `experience_turn_ingested` / `experience_session_discovered` events | Medium | The four `job_*` events are emitted **only** from `jobs.Service.RunOne`, never from per-adapter code; covered by `TestRunOne_EmitsExactlyFourAuditEvents` |
| Backwards-incompatible CLI change breaks the operator's muscle memory | Medium | Proposal explicitly calls this out; the unified subcommand is documented in `docs/04` with a `--source` example for each of the three sources; the help output prints a deprecation note for the old per-source subcommand form |
| `JobFunc` closure capture drift across the three adapters | Low | Each adapter's `Job()` accessor returns a fresh `*semantic.Job` per call; the `JobFunc` body is a stateless method receiver closure; covered by `TestJob_ReturnsDistinctJobsPerCall` |
| Coverage drop in `internal/experience/jobs/` when new code is added | Medium | New `internal/experience/semantic/` package must hit 90%+ on its own per `docs/25` §4; CI gate `coverage_check.sh` runs as part of `gentle-ai review finalize` |
| Migration breaks the receipt gate from `session-2026-07-28-hito10-codex.md` (push blocked once before) | Low | The migration is small (~30 LOC SQL) and isolated; the bounded-review correction budget (`min(200, ceil(changed_lines/2))`) covers the full Hito 11 diff even at 400 LOC |
| `gentle-ai finalize` strict schema rejects reviewer-emitted fields | Medium | The reviewer agents emit only `id`/`location`/`severity`/`claim`/`proof_refs` + `findings`/`evidence` arrays (see `hito10-codex-review-fixes.md`); `evidence` is `[]string` not `[]object`; orchestrator reshapes before capture |
| Reasoning or `function_call_output` text leaks via the audit hook (the SEVERE pattern from Hito 10) | Low | The audit hook emits ONLY enum event names + job-name + run-id; `docs/24-TM` §6 forbids full transcript text in audit; new test `TestAuditHook_DoesNotLeakTranscriptText` asserts the JSON payload contains zero bytes from `ExperienceEnvelope.UserText`/`AssistantText` |
| Hito 3 (`--watch`) silently flips `Enabled = true` when the schema gains `intent`/`scope`/`risk_class` | Low | Hito 11 ships the columns and the upsert path with `Enabled = false`; the flip is owned by Hito 3 (PR #10), not Hito 11; the `JobRegistryEntry` upsert never sets `Enabled = true` |
| Per-adapter `jobs.go` rewrite breaks the Hito-10 contract tests | Low | The new `Job()` accessor is additive — the existing static constructor signature stays as a helper used inside the `JobFunc` body; the Hito 10 compile-time contract tests (`TestAdapter_ImplementsContract`) are untouched |
| Skipped-counter drift across the three adapters (different field sets today) | Low | This change does NOT unify `ScanResult`; the per-adapter shape stays; the `JobFunc` body returns the source's own counters; addressed in a follow-up |
| Cursor parsing fork (OpenCode `int64` vs Claude Code/Codex string-UUID) | Low | Out of scope: Hito 11 does not touch `cursor.go`; the existing per-source `cursorCheckpoint` stays |

## Rollback Plan

The change is reversible in three steps:

1. **Revert migration 008**: `git revert` removes the
   `008_job_semantics.sql` file. The runner (`internal/storage/migrate.go`) is forward-only, so the operator
   must apply the manual rollback recipe documented in
   `tasks.md` Phase 1: drop `job_run_log`, drop the three columns
   on `job_registry`, and remove the `schema_migrations` row for
   version 8.
2. **Drop the new package**: `git rm -r internal/experience/semantic/`
   removes the new package and its test files. No other file imports
   it after step 3.
3. **Restore per-source CLI dispatchers**: revert
   `cmd/royo-learn/experience.go` to the three per-source
   `runExperience<Source>` dispatchers (kept in `git log` history);
   revert each per-adapter `jobs.go` to the static
   `JobRegistryEntry` constructor (also kept in `git log` history).

**Backwards-incompatible call-out**: the unified
`experience scan --source=<opencode|claudecode|codex>` subcommand
replaces three existing subcommands (`experience opencode scan`,
`experience claudecode scan`, `experience codex scan`). If shipped
behind a feature flag (`--experimental-cli-collapse`), the old
subcommands remain available and rollback is non-breaking. If
shipped as the default (the recommended path per the PR-title
symmetry), the old subcommands are removed and any operator script
that calls them must switch to `--source=`. The proposal
recommends shipping as the default with a deprecation note in
`docs/04` for one minor version before the cut.

## Dependencies

- `docs/22-ADAPTER-CONTRACT.md` §1 (frozen, `ExperienceAdapter`)
- `docs/21-EXPERIENCE-DOMAIN.md` §8 (frozen, `JobState`)
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit invariant)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §4 (coverage gate)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14 (gate precondition:
  retrieval lexical report)
- `openspec/changes/hito10-codex/` (just shipped, provides the
  three per-adapter packages)
- `internal/storage.RecordEventTx` (existing audit sink)
- `internal/experience/jobs.Service.RunDue` (existing engine entry
  point that `RunOne` is sibling to)

## Success Criteria

- [ ] `go build ./cmd/royo-learn` passes on Windows/Linux/macOS.
- [ ] `go test -race ./...` passes on linux/amd64.
- [ ] `go vet ./...` passes; `gofmt` clean.
- [ ] `internal/experience/semantic/` has ≥ 90% test coverage
      (`docs/25` §4).
- [ ] All three adapters (`opencode`, `claudecode`, `codex`)
      expose a `Job() *semantic.Job` accessor with the same return
      type (compile-time check).
- [ ] A single `experience scan --source=opencode` (or
      `--source=claudecode` / `--source=codex`) invocation runs
      end-to-end and produces the same JSON envelope shape as the
      previous per-source subcommand.
- [ ] `internal/storage/migrations/008_job_semantics.sql` applies
      idempotently on a fresh DB; migration test asserts forward
      apply, idempotency, and checksum integrity. The runner is
      forward-only; manual-rollback SQL is documented in
      `tasks.md` Phase 1.
- [ ] The audit log records the four lifecycle events
      (`job_pending`, `job_running`, `job_succeeded`, `job_failed`)
      for one ingest run, each exactly once, all sharing the same
      `run_id` from the new `job_run_log` table.
- [ ] `internal/experience/jobs/` retains its existing 94%+ test
      coverage after the `RunOne` addition.
- [ ] The Hito 10 SEVERE trace-leak invariant
      (`hito10-codex-review-fixes.md`) is preserved: the audit hook
      does NOT leak transcript text — covered by
      `TestAuditHook_DoesNotLeakTranscriptText`.
- [ ] No `JobRegistryEntry` row ships with `Enabled = true` (the
      Hito-3 flip is deferred per `docs/26` §5).
- [ ] `gentle-ai review finalize` accepts the change with at most
      one bounded correction round within the 200-line correction
      budget.
- [ ] `docs/14-ACCEPTANCE-CRITERIA.md` §E has new acceptance rows
      for the audit events and the migration; `docs/04-CLI-SPEC.md`
      documents the unified subcommand with one example per source.
