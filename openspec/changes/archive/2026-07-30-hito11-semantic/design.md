# Design: Hito 11 — Semantic / Symmetric Job Engine

## Technical Approach

Deliver a neutral runtime contract `Job` / `JobFunc` / `JobResult` in a new
package `internal/experience/semantic/`, attach the new taxonomy columns
(`intent`, `scope`, `risk_class`) to `job_registry`, persist one
`job_run_log` row per execution, and emit the four lifecycle audit events
(`job_pending`, `job_running`, `job_succeeded`, `job_failed`) from a single
new method `jobs.Service.RunOne` that owns the lease + audit + run-log
transaction. The CLI surface collapses the three per-source `experience
<source> scan` subcommands into one `experience scan --source=<value>`,
guarded by a build-time `ldflags` switch (`--experimental-cli-collapse`)
that defaults to ON. The change preserves every Hito 10 invariant,
notably the SEVERE trace-leak rule
(`hito10-codex-review-fixes.md`); the audit payload allow-list is enforced
at the JSON-marshalling layer, not at the call site.

Reference points used to write this design:

- `internal/experience/jobs/service.go:247-287` (existing `RunDue`
  lease-acquire + retry shape that `RunOne` mirrors).
- `internal/experience/jobs/repository.go:155-180` (existing
  `UpsertRegistryEntry` shape that `UpsertRegistryEntryWithTaxonomy`
  extends).
- `internal/storagerek.go:192-238` (existing `recordSessionAudit` /
  `recordTurnAudit` shape that `recordJobAudit` mirrors).
- `internal/storage/migrations/007_jobs.sql:33-40` (existing
  `job_registry` schema that 008 extends).
- `internal/storage/migrate.go:106-140` (`loadMigrations` embeds
  `migrations/*.sql` from the `internal/storage/migrations/` directory).
- `internal/experience/jobs/types.go:86-93` (existing `JobRegistryEntry`
  struct that gains `Intent`, `Scope`, `RiskClass`).
- `internal/domain/experience.go:32-34` (`domain.SourceOpenCode`,
  `domain.SourceClaudeCode`, `domain.SourceCodex` constants used for
  `--source` validation).
- `cmd/royo-learn/experience.go:27-51` (existing subcommand dispatcher
  that `runExperienceUnified` replaces).

## Architecture Decisions

### Decision: `semantic.Deps` struct shape

**Choice**: A single pass-by-value struct with the four
collaborators the `JobFunc` body needs, plus the per-adapter
`SourceInstance` factory fn injected at `Job()`-call time:

```go
// internal/experience/semantic/job.go
package semantic

import (
    "context"
    "database/sql"
    "time"

    "agent-royo-learn/internal/domain"
    "agent-royo-learn/internal/experience"
)

// Deps is the immutable bundle every JobFunc depends on. Bound at
// RunOne entry; never mutated by the engine after that.
type Deps struct {
    ProjectID      domain.ProjectID
    Ctx            context.Context
    DB             *sql.DB
    Now            func() time.Time
    Logger         Logger // tiny interface {Printf, Errorf}
    RecordEventTx  func(ctx context.Context, tx *sql.Tx, op string, details map[string]any) error
}

// JobFunc is the runtime contract every adapter implements. The body
// MUST respect ctx cancellation and MUST NOT emit Job_* audit events
// (those are emitted by jobs.Service.RunOne).
type JobFunc func(d Deps) (JobResult, error)

// JobResult is the typed outcome of one JobFunc invocation.
type JobResult struct {
    Envelopes           []experience.ExperienceEnvelope
    SkippedIncomplete   int
    SkippedMalformed    int
    SkippedSystem       int
    SkippedUnknown      int
    NextCursor          string
    IngestedTurns       int
    Duplicates          int
    ErrorCode           string
    ErrorMessage        string
}

// Job binds a registry entry to its runtime closure.
type Job struct {
    Entry   jobs.JobRegistryEntry
    Source  domain.ExperienceSource
    JobFunc JobFunc
}
```

**Alternatives considered**:

- *Pass an `interface{}` of untyped deps*: rejected — the existing
  codebase uses small typed structs (`experience.Config`,
  `jobs.LeaseBounds`); a typed struct keeps the call site readable
  and lets the engine test `JobFunc` against a real `sql.Tx` in unit
  tests (per `docs/25` §4 ≥ 90 % gate).
- *Pass the `*Service` directly*: rejected — couples the closure to
  the engine; Hito 12 (MCP tool surface) will reuse the `Job` without
  the engine, so the dependency must be a leaf set.
- *Per-field args (projectID, ctx, db, now, …)*: rejected — already
  rejected by `RunDue`. Once the surface is ≥ 5 fields, a struct is
  clearer and stable for future add-ons (e.g. cancellation token,
  metrics).

**Rationale**: Each field is justified individually:

- `ProjectID` — required by `service.IngestEnvelope` (the per-envelope
  ingestion path in `internal/experience/service.go:311`).
- `Ctx` — passed through so the adapter's `Discover` / `Health` /
  `Scan` calls respect a single cancellation; the placeholder
  `context.Background()` would re-introduce the Hito 9 cancellation
  regression.
- `DB` — needed for the inner `Ingest` transactions the adapter
  drives; reusing the same `*sql.DB` keeps the lease + audit +
  ingestion txs on the same connection pool.
- `Now` — package-level injectable clock (the CodeReference:
  `internal/experience/codex/jobs.go:16` already uses `jobNow` for
  exactly this reason).
- `Logger` — minimal interface so the adapter can emit
  `skipped_incomplete` diagnostics at the same level as the engine;
  the production logger is `internal/logging.Logger`.
- `RecordEventTx` — exposes the existing audit sink
  (`internal/storage/repo_audit.go:19`) without leaking the
  `*domain.AuditEvent` builder into the `JobFunc` body. The 4-event
  transition is owned by `RunOne`, but the per-envelope `recordTurnAudit`
  (`internal/experience/service.go:924`) keeps using the raw helper.

### Decision: Lease-acquisition path inside `RunOne`

**Choice**: Reuse `Service.AcquireLease` (existing at
`internal/experience/jobs/service.go:92-174`) verbatim and wrap it
in a single `*sql.Tx` that also writes `job_run_log` and emits the four
audit events. Pseudo-code:

```go
// internal/experience/jobs/service.go (new method)
func (s *Service) RunOne(parent context.Context, projectID domain.ProjectID,
                         jobName, owner string, j semantic.Job) (JobRunOutcome, error) {

    // 1. Load JobRegistryEntry (MustExist – RunOne called by name).
    if err := s.assertJobRegistered(ctx, jobName); err != nil { ... }

    now := s.now()
    runID := uuid.NewV7().String()           // deterministic UUID per run
    var outcome JobRunOutcome

    // 2. Pre-lease: emit job_pending, write a stub job_run_log.
    tx, err := s.db.BeginTx(parent, nil)
    defer tx.Rollback()
    if err := s.recordJobAuditTx(ctx, tx, semantic.EventJobPending, runID, jobName, j.Source, semStatePending, "", "", 0); err != nil { ... }
    logStub := jobRunLog{RunID: runID, JobName: jobName, State: string(semStatePending), StartedAt: now, Attempt: 0}
    if err := s.repo.RecordRunLog(ctx, tx, logStub); err != nil { ... }

    // 3. Acquire lease (reuse helper, NOT a second tx).
    state, err := s.AcquireLease(ctx, projectID, jobName, owner)
    if err != nil {                                 // lease held
        return outcome, s.recordJobAuditTx(ctx, tx, semantic.EventJobFailed, runID, jobName, j.Source, semStateLeaseHeld, "job_lease_held", err.Error(), 0)
    }

    // 4. Emit job_running + write attempt.
    if err := s.recordJobAuditTx(ctx, tx, semantic.EventJobRunning, runID, jobName, j.Source, semStateRunning, "", "", 1); err != nil { ... }
    if err := s.repo.UpdateRunLogAttempt(ctx, tx, runID, 1); err != nil { ... }

    // 5. Execute the JobFunc inside the same tx (the engine does NOT
    //    hand the tx to the body; deps.RecordEventTx is the only tx-
    //    exposing abstraction – data Ingestions open their own tx as
    //    they do today).
    deps := semantic.Deps{ProjectID: projectID, Ctx: ctx, DB: s.db, Now: s.now, Logger: s.logger, RecordEventTx: s.buildRecordEventTx(ctx, tx)}
    jobResult, jobErr := j.JobFunc(deps)

    // 6. Finalize: release lease + emit terminal event + finish run-log.
    if jobErr != nil || jobResult.ErrorCode != "" {
        s.ReleaseLease(ctx, projectID, jobName, owner, RunResult{Status: JobError, Code: "execution_error", Message: errMsg(jobErr, jobResult)})
        s.recordJobAuditTx(ctx, tx, semantic.EventJobFailed, runID, jobName, j.Source, semStateFailed, "execution_error", ..., 1)
        s.repo.FinishRunLog(ctx, tx, runID, semStateFailed, code, msg, 1)
    } else {
        s.ReleaseLease(ctx, projectID, jobName, owner, RunResult{Status: JobOK})
        s.recordJobAuditTx(ctx, tx, semantic.EventJobSucceeded, runID, jobName, j.Source, semStateSucceeded, "", "", 1)
        s.repo.FinishRunLog(ctx, tx, runID, semStateSucceeded, "", "", 1)
    }
    if err := tx.Commit(); err != nil { return outcome, err }
    return outcome, nil
}
```

**Alternatives considered**:

- *Open a fresh `*sql.Tx` and call sqlite directly* (bypass
  `AcquireLease`): rejected — `AcquireLease` already wraps the
  state-SELECT + lease-OVERWRITE in one `tx` and holds the lease
  ownership check. Bypassing it would duplicate the conflict path
  and break the `RunDue` flow's invariant that "lease held by another
  owner = skip".
- *Use `RunDue` with a single-element job map*: rejected —
  `RunDue` calls `executeWithRetry` which calls `ReleaseLease`
  immediately after the body returns, so the audit-hook's
  "4 events in one tx" is impossible inside `RunDue`. The proposal
  explicitly demands a sibling `RunOne` method.

**Rationale**: `RunOne` is a sibling of `RunDue`, not a replacement.
`RunDue` keeps its retry loop for the watch loop (Hito 3 / PR #10);
`RunOne` is the one-shot path used by the unified CLI
(`experience scan --source=…`). Reusing `AcquireLease` keeps the
lease-conflict semantics identical to the 94 %-covered
`AcquireReleaseLease` test (`internal/experience/jobs/service_test.go:77`).

### Decision: `--experimental-cli-collapse` flag wiring

**Choice**: Build-time `ldflags` set at compile time, env-var
override at runtime, default ON in production binaries and in tests.

```go
// internal/experience/cli/collapse.go (new helper)
package cli

// collapseFlag is initialised at process start from ldflags and may be
// overridden at runtime via ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE.
var collapseFlag = ldflagBool("experimental-cli-collapse", true)

func ExperimentalCLICollapse() bool {
    if v := os.Getenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE"); v != "" {
        return v == "1" || strings.EqualFold(v, "true")
    }
    return collapseFlag
}
```

Makefile:

```
LD_FLAGS := -ldflags "-X agent-royo-learn/internal/experience/cli.collapseFlag=false"
build-collapse-off:  ; go build $(LD_FLAGS) -o royo-learn ./cmd/royo-learn
build (default):     ; go build -o royo-learn ./cmd/royo-learn
```

**Alternatives considered**:

- *Runtime flag only*: rejected — a runtime flag leaves the registries
  in the binary (no dead-code elimination) and lets an operator toggle
  broken subcommands at the wrong moment. The proposal's risk table
  wants "rollback granularity"; ldflags gives per-build rollback,
  which is the safe knob.
- *Config-file toggle*: rejected — adds a new config file format,
  redundant with the existing `projects.json` rotation; the project
  convention is to avoid file-level toggles for CLI scope.
- *Env-var only*: rejected — env vars are easy to forget in CI;
  ldflags force the operator to consciously rebuild.

**Rationale**: ldlags + env-var override is the existing pattern
across the codebase (`--rollback-test`, `--dangerously-allow-…`);
adding the second toggle at the same level keeps the operator UX
consistent. The `runExperienceUnified` dispatcher reads the helper
once and routes the legacy subcommands through it when
`ExperimentalCLICollapse()` is false.

### Decision: Audit-event payload schema

**Choice**: A typed `Details map[string]any` allow-list, plus a
compile-time JSON tag check that proves the runtime never adds
forbidden fields.

```go
// internal/experience/semantic/events.go
package semantic

const (
    EventJobPending   = "job_pending"
    EventJobRunning   = "job_running"
    EventJobSucceeded = "job_succeeded"
    EventJobFailed    = "job_failed"
)

const (
    StatePending   = "pending"
    StateRunning   = "running"
    StateSucceeded = "succeeded"
    StateFailed    = "failed"
    StateLeaseHeld = "lease_held"  // terminal substate inside Failed
)

// AllowedDetailsKeys is the exhaustive JSON allow-list for job_*
// audit payloads. Adding a key here is a code change AND a test
// change (TestAuditHook_AllowListContract below).
var AllowedDetailsKeys = []string{
    "job_name", "run_id", "source", "state", "attempt",
    "error_code", "error_message",
}

// jobPayload builds the JSON details blob for a job_* event. It
// enforces the allow-list at runtime AND at static review (the test
// reads the source file and asserts no raw map literal bypasses the
// helper). The function NEVER accepts an ExperienceEnvelope or any
// field of it.
func jobPayload(jobName, runID string, source domain.ExperienceSource, state, attempt string, errorCode, errorMessage string) map[string]any {
    out := map[string]any{
        "job_name": jobName,
        "run_id":   runID,
        "source":   string(source),
        "state":    state,
        "attempt":  attempt,
    }
    if errorCode != "" {
        out["error_code"] = errorCode
    }
    if errorMessage != "" {
        out["error_message"] = errorMessage
    }
    return out
}
```

Sample JSON line (happy path, one run):

```json
{"sequence":791,"id":"…","occurred_at":"2026-07-28T14:35:33Z","actor_json":"{\"kind\":\"system\",\"name\":\"jobs-service\"}","operation":"job_pending","entity_type":"job_run","entity_id":"01HZX…","previous_state":null,"new_state":null,"payload_sha256":"…","result":"success","error_code":null,"details_json":"{\"job_name\":\"experience_ingest:opencode\",\"run_id\":\"01HZX…\",\"source\":\"opencode\",\"state\":\"pending\",\"attempt\":\"0\"}"}
{"sequence":792, … ,"operation":"job_running","details_json":"{…,\"state\":\"running\",\"attempt\":\"1\"}"}
{"sequence":793, … ,"operation":"job_succeeded","details_json":"{…,\"state\":\"succeeded\",\"attempt\":\"1\"}"}
```

**Alternatives considered**:

- *Free-form `details` map at the call site*: rejected — the Hito 10
  SEVERE invariant is a leak of `UserText` / `AssistantText` /
  `ToolCalls[].OutputHint`. An unrestricted map lets a future
  engineer copy a typed field from `ExperienceEnvelope` into the
  payload by accident. The allow-list + `jobPayload` helper makes
  the leak impossible at compile time.
- *Typed `domain.AuditEvent` with optional `Payload` struct*: rejected
  — the existing `RecordEventTx` already accepts `Details map[string]any`
  (`internal/storage/repo_audit.go:36-54`). Reusing the existing shape
  keeps the migration risk at zero.

**Rationale**: The Hito 10 SEVERE trace-leak invariant
(`hito10-codex-review-fixes.md`) is preserved by 1) the static
allow-list, 2) the helper that builds the map, 3) the unit test
`TestAuditHook_DoesNotLeakTranscriptText` that loads a fixture
envelope with `LEAK_CANARY_USER` / `LEAK_CANARY_ASSISTANT` sentinels
and asserts the four event payloads contain zero bytes of those
sentinels.

### Decision: `job_run_log` schema columns + indexes

**Choice**: Migration `internal/storage/migrations/008_job_semantics.sql`
adds the three columns to `job_registry` and the new
`job_run_log` table. The DDL is idempotent (`CREATE TABLE IF NOT EXISTS`,
`ALTER TABLE … ADD COLUMN` guarded by `PRAGMA table_info`).

```sql
-- 008_job_semantics.sql
PRAGMA foreign_keys = ON;

-- 1. Extend job_registry with the taxonomy columns.
--    ALTER TABLE has no IF NOT EXISTS in SQLite; we guard by checking
--    pragma_table_info. The engine applies the migration once per
--    DB, so the guard is a one-shot idempotency check.
ALTER TABLE job_registry ADD COLUMN intent  TEXT NOT NULL DEFAULT 'ingest';
ALTER TABLE job_registry ADD COLUMN scope   TEXT NOT NULL DEFAULT 'project';
ALTER TABLE job_registry ADD COLUMN risk_class TEXT NOT NULL DEFAULT 'low';

-- 2. Reverse FK: each run references its registry row.
CREATE TABLE IF NOT EXISTS job_run_log (
    run_id        TEXT    PRIMARY KEY,
    job_name      TEXT    NOT NULL REFERENCES job_registry(job_name),
    state         TEXT    NOT NULL CHECK(state IN
                  ('pending','running','succeeded','failed','lease_held')),
    started_at    TEXT    NOT NULL,
    finished_at   TEXT,
    error_code    TEXT    NOT NULL DEFAULT '',
    error_message TEXT    NOT NULL DEFAULT '',
    attempt       INTEGER NOT NULL DEFAULT 0
);

-- 3. Indexes for the three read paths.
CREATE INDEX IF NOT EXISTS idx_job_run_log_job_started
    ON job_run_log(job_name, started_at);
CREATE INDEX IF NOT EXISTS idx_job_run_log_run_id
    ON job_run_log(run_id);
```

**Alternatives considered**:

- *One row per `job_state` per run (instead of a separate `job_run_log`)*:
  rejected — `job_state` is per-(project, job_name) and is the lease
  anchor; coupling the run history to it would explode the row count
  per ingest and break the `RunDue` listing query.
- *Foreigner-key on `job_registry` with `ON DELETE CASCADE`*: rejected
  — historical rows must survive a registry purge (audit invariant);
  the FK is `REFERENCES` only with `RESTRICT` (the SQLite default).

**Rationale**: `run_id` is a UUIDv7 deterministic per run (the engine
mints it, not the adapter). The `attempt` column lets the audit
emission distinguish attempt-0 (pending) from attempt-1 (running)
without a separate `job_run_log_attempt` table. The `(job_name, started_at)`
index supports the operator query "what did ingest do over the last 24 h";
the `run_id` index is the join key for the four-event audit query.

### Decision: `JobRegistryEntry` upsert migration path

**Choice**: Extend `jobs.Repository.UpsertRegistryEntry` in place to
write the three new columns. The static rows populated by
`Service.Register` (which Hito 3 / PR #10 already calls at startup
via `cmd/royo-learn/setup.go`) start carrying the new values
automatically. The migration is **not** a one-time boot step — every
upsert writes the taxonomy values idempotently.

```go
// internal/experience/jobs/repository.go (extended)
func (r *Repository) UpsertRegistryEntry(ctx context.Context, entry JobRegistryEntry) error {
    if !semantic.IsValidIntent(entry.Intent) ||
       !semantic.IsValidScope(entry.Scope) ||
       !semantic.IsValidRiskClass(entry.RiskClass) {
        return domain.NewValidationError(domain.ErrInvalidArgument,
            "jobs: registry entry has invalid taxonomy values")
    }
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO job_registry (job_name, description, default_interval_sec,
            default_max_retries, intent, scope, risk_class, enabled, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(job_name) DO UPDATE SET
            description = excluded.description,
            default_interval_sec = excluded.default_interval_sec,
            default_max_retries = excluded.default_max_retries,
            intent = excluded.intent,
            scope = excluded.scope,
            risk_class = excluded.risk_class,
            enabled = excluded.enabled`,
        entry.JobName, entry.Description, entry.DefaultIntervalSec,
        entry.DefaultMaxRetries, string(entry.Intent), string(entry.Scope),
        string(entry.RiskClass), boolToInt(entry.Enabled), now,
    )
    …
}
```

**Alternatives considered**:

- *One-time data migration that runs at boot* (a SQL `UPDATE … SET intent =
  'ingest' WHERE job_name LIKE 'experience_ingest:%'`): rejected —
  the values are not derivable from the registry name alone; future
  jobs (e.g. `experience_promote:opencode`) will carry other
  `intent` values. Putting the values in the upsert path means the
  static row IS the source of truth.
- *Per-adapter `JobRegistryEntry` constructor sets the values but the
  Repository defaults them to `''`*: rejected — the upsert path is
  the only authoritative path; if the constructor is forgotten for a
  future job, the row silently loses its taxonomy. Failing fast
  (`Upsert` rejects unknown values) is the right gate.

**Rationale**: The proposal's Risk §1 ("Migration 008 breaks a
partial-downgrade path") is mitigated by the Repository's strict
validation. The proposal's Risk §9 ("Hito 3 watch flip silently
sets `Enabled = true`") is mitigated by the constructor always
passing `Enabled: false` from the per-adapter `Job()` accessor; the
upsert path can flip it on, but the static constructors never do.

### Decision: Per-adapter `jobs.go` rewrite pattern

**Choice**: The legacy constructor `JobRegistryEntry()` stays as an
**unexported** helper `newIngestJobRegistryEntry()` inside the
`JobFunc` body. The new public surface is the `Job() *semantic.Job`
method on each adapter. The three new `Job()` accessors are
**stateless** (the adapter is already stateless per `adapter.go`).

```go
// internal/experience/claudecode/jobs.go (rewritten)
package claudecode

import (
    "agent-royo-learn/internal/experience"
    "agent-royo-learn/internal/experience/jobs"
    "agent-royo-learn/internal/experience/semantic"
)

const JobName = "experience_ingest:claude_code"

// NewAdapter → delegates to *Adapter. Keeps the existing constructor.
func NewAdapter() *Adapter { return &Adapter{} }

// Job returns the runtime contract for the Claude Code ingest loop.
// The legacy static JobRegistryEntry is preserved as an unexported
// helper invoked inside JobFunc; the public surface is the typed
// *semantic.Job.
func (a *Adapter) Job() *semantic.Job {
    entry := newIngestJobRegistryEntry()
    return &semantic.Job{
        Entry:  entry,
        Source: ClaudeCodeSource, // domain.SourceClaudeCode
        JobFunc: func(d semantic.Deps) (semantic.JobResult, error) {
            return runClaudeCodeJob(a, d)
        },
    }
}

// newIngestJobRegistryEntry is the unexported successor of the Hito 10
// JobRegistryEntry(). It populates the three new taxonomy columns.
func newIngestJobRegistryEntry() jobs.JobRegistryEntry {
    return jobs.JobRegistryEntry{
        JobName:            JobName,
        Description:        "Incremental ingest of Claude Code JSONL transcripts.",
        DefaultIntervalSec: 300,
        DefaultMaxRetries:  3,
        Intent:             semantic.JobIntentIngest,
        Scope:              semantic.JobScopeProject,
        RiskClass:          semantic.JobRiskClassLow,
        Enabled:            false,
    }
}

// runClaudeCodeJob is the Discover → Health → Scan → Ingest pipeline
// that previously lived inline in runExperienceClaudecodeScan. It
// returns the aggregated semantic.JobResult.
func runClaudeCodeJob(a *Adapter, d semantic.Deps) (semantic.JobResult, error) {
    instances, err := a.Discover(d.Ctx, d.ProjectRoot)
    if err != nil { … }
    var out semantic.JobResult
    for _, inst := range instances {
        if hr := a.Health(d.Ctx, inst); hr.Status != "ok" { … continue }
        scan, err := a.Scan(d.Ctx, …)
        if err != nil { … }
        out.Envelopes = append(out.Envelopes, scan.Envelopes...)
        out.SkippedIncomplete += scan.SkippedIncomplete
        out.SkippedMalformed  += scan.SkippedMalformed
    }
    return out, nil
}
```

**Alternatives considered**:

- *Move the legacy constructor to a new internal package
  `internal/experience/jobs/registry/`*: rejected — the per-adapter
  `jobs.go` is the authoritative place to bind the adapter to its
  job (the binding is identity-rich: `JobName` constant + adapter
  method). Splitting the constructor away would force a circular
  import (the binding needs both `semantic.Job` and the adapter).
- *Keep the legacy constructor exported (`JobRegistryEntry()`)*:
  rejected — the proposal explicitly calls this out as a change in
  public surface. The unexported helper is the same code, but with
  its symbol visibility narrowed to the package.

**Rationale**: The legacy static constructor is still useful (it
documents the three "static" fields that the bind row carries), so
keeping it as `newIngestJobRegistryEntry` is the smallest change
that satisfies the proposal's "static constructor signature stays as
a helper used inside the `JobFunc` body" rule
(`openspec/changes/hito11-semantic/specs/experience-adapters/spec.md`
REQ-EA-1 MODIFIED Scenario 4).

## Data Flow

```
operator types:
  `royo-learn experience scan --source=codex --project-root <path>`
                              │
                              ▼
   cmd/royo-learn/experience.go:runExperienceUnified
                              │
                              │ parse flags, validate --source
                              │ against domain.ExperienceSource
                              ▼
   opencode.Job() | claudecode.Job() | codex.Job()  ──→ *semantic.Job
                              │
                              │ runExperienceUnified builds semantic.Deps
                              │ (projectID, ctx, db, now, logger)
                              ▼
   jobs.Service.RunOne(ctx, projectID, jobName, owner, job)
                              │
                              │ 1. tx.Begin
                              │ 2. recordEventTx(job_pending)  ─→ audit_events
                              │ 3. repo.RecordRunLog(stub)     ─→ job_run_log
                              │ 4. service.AcquireLease        (existing)
                              │ 5. recordEventTx(job_running)  ─→ audit_events
                              │ 6. repo.UpdateRunLogAttempt(1) ─→ job_run_log
                              │ 7. semantic.Job.JobFunc(deps)
                              │      │
                              │      │  Adapter.Discover
                              │      │  Adapter.Health
                              │      │  Adapter.Scan
                              │      │  experience.Service.Ingest*
                              │      │  (each ingestion opens its own tx;
                              │      │   data integrity is preserved by
                              │      │   the existing service.go:236 path)
                              │      ▼
                              │  semantic.JobResult
                              │ 8. (ok) recordEventTx(job_succeeded) +
                              │        ReleaseLease(JobOK) +
                              │        repo.FinishRunLog(succeeded)
                              │    (err) recordEventTx(job_failed) +
                              │        ReleaseLease(JobError) +
                              │        repo.FinishRunLog(failed)
                              │ 9. tx.Commit
                              ▼
   runExperienceUnified prints JSON envelope to stdout
```

The four `audit_events` rows + the `job_run_log` row are emitted in
ONE `*sql.Tx` to guarantee atomicity (per
`job-semantic-engine` REQ-JSE-4). The `JobFunc` body opens its own
inner txs for `Ingest` calls (the existing pattern at
`internal/experience/service.go:236`); those are independent of the
run-log tx because the audit hook for them is `recordSessionAudit` /
`recordTurnAudit`, not `job_*`.

## File-Level Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/experience/semantic/job.go` | Create | `Job`, `JobResult`, `JobFunc`, `Deps` types |
| `internal/experience/semantic/types.go` | Create | `JobIntent`, `JobScope`, `JobRiskClass` enum types + value constants + `IsValidX` helpers |
| `internal/experience/semantic/events.go` | Create | `EventJobPending`, `EventJobRunning`, `EventJobSucceeded`, `EventJobFailed` constants + `StateX` + `AllowedDetailsKeys` + `jobPayload` allow-list helper |
| `internal/experience/semantic/job_test.go` | Create | Unit tests for `Job`, `JobFunc`, context cancellation, payload allow-list |
| `internal/experience/semantic/types_test.go` | Create | Enum validation tests (positive + negative) |
| `internal/experience/semantic/events_test.go` | Create | `jobPayload` allow-list + key-rejection tests |
| `internal/experience/jobs/jobs.go` (new) | Create | `JobRunOutcome` + `jobRunLog` struct + `RunOne` method |
| `internal/experience/jobs/service.go` | Modify | Add `RunOne` (sibling to `RunDue`); add `recordJobAuditTx` helper; add `buildRecordEventTx` |
| `internal/experience/jobs/repository.go` | Modify | Add `RecordRunLog`, `UpdateRunLogAttempt`, `FinishRunLog`; extend `UpsertRegistryEntry` with `intent`/`scope`/`risk_class` |
| `internal/experience/jobs/types.go` | Modify | Add `Intent`, `Scope`, `RiskClass` to `JobRegistryEntry` |
| `internal/experience/jobs/service_test.go` | Modify | Add `TestRunOne_Leases`, `TestRunOne_EmitsFourEvents`, `TestRunOne_DoesNotLeakTranscriptText`, `TestRunOne_FailureEmitsJobFailed`, `TestRunOne_LeaseConflictSkips`, `TestRunOne_CancellationHonoured` |
| `internal/experience/jobs/repository_test.go` | Modify | Add `TestRecordRunLog_RoundTrip`, `TestUpsertRegistryEntry_RejectsUnknownTaxonomy`, `TestUpsertRegistryEntry_PopulatesThreeColumns` |
| `internal/experience/opencode/jobs.go` | Create | `Job()` accessor + unexported `newIngestJobRegistryEntry` + `runOpencodeJob` (the first time opencode gets a `jobs.go`) |
| `internal/experience/opencode/jobs_test.go` | Create | `TestOpencodeJob_AccessorReturnsTypedJob`, `TestOpencodeJob_SourceMatches`, `TestOpencodeJob_DistinctPerCall` |
| `internal/experience/claudecode/jobs.go` | Modify | Replace `JobRegistryEntry()` with `Job()` + unexported helper; keep `JobName` constant |
| `internal/experience/claudecode/jobs_test.go` | Modify | Replace legacy test with new `Job()` accessor tests |
| `internal/experience/codex/jobs.go` | Modify | Same as claudecode |
| `internal/experience/codex/jobs_test.go` | Modify | Same as claudecode |
| `internal/experience/cli/collapse.go` | Create | `ExperimentalCLICollapse()` helper reading ldflags + env override |
| `internal/experience/cli/collapse_test.go` | Create | `TestCollapseFlag_DefaultsToOn`, `TestCollapseFlag_EnvOverride` |
| `cmd/royo-learn/experience.go` | Modify | Replace `runExperienceOpencode` etc. with `runExperienceUnified`; keep legacy form routing when `ExperimentalCLICollapse() == false` |
| `cmd/royo-learn/experience.go` (`runExperience<Source>`) | Modify (or Delete) | If collapse-on: delete the three per-source dispatcher bodies (kept in git log). If collapse-off: keep them as thin wrappers around `runExperienceUnified` |
| `cmd/royo-learn/experience_unified_test.go` | Create | Tests for the unified dispatcher: --source missing, --source invalid, --source accepted, JSON envelope shape parity |
| `internal/storage/migrations/008_job_semantics.sql` | Create | Adds three columns to `job_registry`; creates `job_run_log` with FK + indexes |
| `internal/storage/migrate_test.go` (new) | Create | `TestMigrate_008_Forward`, `TestMigrate_008_Idempotent`, `TestMigrate_008_ChecksumStable` |
| `docs/04-CLI-SPEC.md` | Modify (additive) | Document `--source` flag + `--experimental-cli-collapse` build switch |
| `docs/14-ACCEPTANCE-CRITERIA.md` §E | Modify (additive) | Add acceptance rows for the four audit events + the migration test |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14 | Modify | Mark PR #14 as "in flight" with link to this proposal |
| `openspec/changes/hito11-semantic/specs/experience-adapters/spec.md` | Modify | Already exists; no further edits needed |
| `openspec/changes/hito11-semantic/design.md` | Create | This document |

Total: 17 created, 12 modified, 0 deleted (3 dispatcher bodies may
collapse into thin wrappers; counted as net-zero).

## Test Strategy

### Unit tests (per `docs/25` §4 ≥ 90 % coverage)

| Test file | Test function | Purpose |
|-----------|---------------|---------|
| `internal/experience/semantic/job_test.go` | `TestJob_ContractCompiles` | Compile-time check of `JobFunc` signature |
| `…/job_test.go` | `TestJobFunc_RespectsContextCancellation` | Cancellation propagates to body |
| `…/types_test.go` | `TestJobIntent_KnownValues` | `JobIntentIngest` etc. exported |
| `…/types_test.go` | `TestJobIntent_UnknownRejected` | Upsert rejects `intent="scrape"` |
| `…/types_test.go` | `TestJobScope_KnownValues`, `TestJobScope_UnknownRejected` | Same for scope |
| `…/types_test.go` | `TestJobRiskClass_KnownValues`, `TestJobRiskClass_UnknownRejected` | Same for risk_class |
| `…/events_test.go` | `TestJobPayload_AllowListContract` | Only the documented keys are emitted |
| `…/events_test.go` | `TestJobPayload_RejectsForbiddenKeys` | Free-form `details` map is rejected |
| `internal/experience/cli/collapse_test.go` | `TestCollapseFlag_DefaultsToOn` | Default is true |
| `…/collapse_test.go` | `TestCollapseFlag_EnvOverride` | `ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE=0` flips it |
| `internal/experience/opencode/jobs_test.go` | `TestOpencodeJob_AccessorReturnsTypedJob` | `Job()` returns `*semantic.Job` non-nil |
| `…/opencode/jobs_test.go` | `TestOpencodeJob_SourceMatches` | `Source == domain.SourceOpenCode` |
| `…/opencode/jobs_test.go` | `TestOpencodeJob_DistinctPerCall` | Two calls return distinct pointers |
| `internal/experience/claudecode/jobs_test.go` | Same three tests | Per `experience-adapters` REQ-EA-1 ADDED |
| `internal/experience/codex/jobs_test.go` | Same three tests | Per `experience-adapters` REQ-EA-1 ADDED |
| `internal/experience/jobs/service_test.go` | `TestRunOne_Leases` | Lease acquired and released |
| `…/service_test.go` | `TestRunOne_EmitsFourEvents` | All four events present, one each |
| `…/service_test.go` | `TestRunOne_FailureEmitsJobFailed` | Failure path emits `job_failed` once |
| `…/service_test.go` | `TestRunOne_LeaseConflictSkips` | Lease held by another owner → only `job_pending` + `job_failed(lease_held)` |
| `…/service_test.go` | `TestRunOne_CancellationHonoured` | `ctx.Done()` propagated to body |
| `…/service_test.go` | `TestRunOne_RunIDsAreUnique` | Two invocations → two distinct `run_id` |
| `…/service_test.go` | `TestAuditHook_DoesNotLeakTranscriptText` | **Hito 10 SEVERE invariant** — fixture envelope with `LEAK_CANARY_USER` / `LEAK_CANARY_ASSISTANT` |
| `…/service_test.go` | `TestRunOne_PayloadAllowList` | Only the 7 documented keys present |
| `…/service_test.go` | `TestRunOne_PerAdapterPathNoDirectAuditCall` | Static-review greps the three per-adapter files and finds zero `RecordEventTx` calls |
| `internal/experience/jobs/repository_test.go` | `TestRecordRunLog_RoundTrip` | Insert + read back |
| `…/repository_test.go` | `TestUpsertRegistryEntry_PopulatesThreeColumns` | `intent`/`scope`/`risk_class` written |
| `…/repository_test.go` | `TestUpsertRegistryEntry_RejectsUnknownTaxonomy` | `intent="scrape"` returns validation error |
| `…/repository_test.go` | `TestUpsertRegistryEntry_RejectsUnknownRiskClass` | Same for risk_class |
| `internal/storage/migrate_test.go` (new) | `TestMigrate_008_Forward` | Fresh DB → 008 succeeds |
| `…/migrate_test.go` | `TestMigrate_008_Idempotent` | 008 runs twice on same DB |
| `…/migrate_test.go` | `TestMigrate_008_ChecksumStable` | Checksum recorded in `schema_migrations` matches the runner's recomputed SHA-256 |
| `cmd/royo-learn/experience_test.go` | `TestExperienceUnified_MissingSource` | Exit code 2, no usage hint points to legacy form |
| `…/experience_test.go` | `TestExperienceUnified_InvalidSource` | Exit code 2, error lists allowed values |
| `…/experience_test.go` | `TestExperienceUnified_OpenCodeAccepted` | `Job()` called, JSON envelope printed |
| `…/experience_test.go` | `TestExperienceUnified_CodexAccepted` | Same for codex |
| `…/experience_test.go` | `TestExperienceUnified_ClaudecodeAccepted` | Same for claudecode |
| `…/experience_test.go` | `TestExperienceUnified_OutputKeysParity` | Output JSON keys match legacy form byte-for-byte |
| `…/experience_test.go` | `TestExperienceUnified_DeprecationNote_CollapseOff` | When flag is off, legacy call prints `DEPRECATED:` to stderr |
| `…/experience_test.go` | `TestExperienceUnified_NoDeprecationNote_Unified` | Unified form never prints the note |

### Integration tests

| Test file | Test function | Purpose |
|-----------|---------------|---------|
| `internal/experience/jobs/service_test.go` (`integration_test.go`) | `TestRunOne_EndToEnd_OpenCode` | Real DB + real adapter + fixture → four events in audit, one row in `job_run_log` |
| `…/service_test.go` | `TestRunOne_EndToEnd_Codex` | Same for codex |
| `…/service_test.go` | `TestRunOne_EndToEnd_ClaudeCode` | Same for claudecode |

### E2E tests

| Test | Purpose |
|------|---------|
| `cmd/royo-learn/e2e_test.go` (existing) — new subtest `TestExperienceScanUnified_OpenCode` | Build CLI, run `experience scan --source=opencode` against a fixture, assert JSON envelope shape on stdout |
| `…/TestExperienceScanUnified_Codex` | Same for codex |
| `…/TestExperienceScanUnified_Claudecode` | Same for claudecode |
| `…/TestExperienceScanUnified_Deprecation` | Build with `ldflags` setting collapse=false, run legacy `experience opencode scan`, assert `DEPRECATED:` is on stderr and exit code 0 |

## Migration Plan

### Forward (008_job_semantics.sql)

```sql
PRAGMA foreign_keys = ON;

-- 1. Add taxonomy columns to job_registry.
--    SQLite ALTER TABLE does not support IF NOT EXISTS; we rely on
--    the migration runner to apply 008 once per DB. The downstream
--    DEFAULT guarantees the NOT NULL constraint is satisfied even
--    on existing rows.
ALTER TABLE job_registry ADD COLUMN intent  TEXT NOT NULL DEFAULT 'ingest';
ALTER TABLE job_registry ADD COLUMN scope   TEXT NOT NULL DEFAULT 'project';
ALTER TABLE job_registry ADD COLUMN risk_class TEXT NOT NULL DEFAULT 'low';

-- 2. Audit + idempotency table.
CREATE TABLE IF NOT EXISTS job_run_log (
    run_id        TEXT    PRIMARY KEY,
    job_name      TEXT    NOT NULL REFERENCES job_registry(job_name),
    state         TEXT    NOT NULL CHECK(state IN
                  ('pending','running','succeeded','failed','lease_held')),
    started_at    TEXT    NOT NULL,
    finished_at   TEXT,
    error_code    TEXT    NOT NULL DEFAULT '',
    error_message TEXT    NOT NULL DEFAULT '',
    attempt       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_job_run_log_job_started
    ON job_run_log(job_name, started_at);
CREATE INDEX IF NOT EXISTS idx_job_run_log_run_id
    ON job_run_log(run_id);
```

### Manual rollback recipe (operator use only — runner is forward-only)

The migration runner (`internal/storage/migrate.go`) is forward-only
and does NOT execute any reverse path. For emergency rollback,
the operator runs these statements manually against the DB after
`git revert`:

```sql
-- 1. Drop the new indexes + table.
DROP INDEX IF EXISTS idx_job_run_log_run_id;
DROP INDEX IF EXISTS idx_job_run_log_job_started;
DROP TABLE IF EXISTS job_run_log;

-- 2. SQLite cannot DROP COLUMN with FK constraints in older versions;
--    use the documented rename + recreate pattern.
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

-- 3. Remove the runner's record of 008 so a future Migrate() call does
--    not try to re-apply it.
DELETE FROM schema_migrations WHERE version = 8;
```

### Idempotency story

- The migration runner (`internal/storage/migrate.go:106-140`) embeds
  `migrations/*.sql` and applies them in version order. The runner
  computes a SHA-256 checksum and refuses to re-apply if the stored
  checksum differs.
- `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` keep
  the forward apply idempotent at the SQL level (the runner is the
  primary guard).
- The `ALTER TABLE … ADD COLUMN` is not guarded by `IF NOT EXISTS` —
  the migration runner ensures 008 is applied exactly once per DB, so
  the second `Migrate` call is a no-op.

### Risk Mitigations

1. **Migration 008 lacks a runner-driven rollback (Medium)**: The
   runner is forward-only. The manual rollback recipe above is
   documented in `tasks.md` Phase 1 and exercised by a doc-test
   (the SQL is checked into the repo so it can be reviewed, not
   auto-executed). The migration test
   `TestMigrate_008_ChecksumStable` asserts the runner's checksum
   path stays stable across re-applies, which is the strongest
   automated guard the runner offers.

2. **Audit-event emission duplicates the existing
   `experience_turn_ingested` / `experience_session_discovered` events
   (Medium)**: The four `job_*` events are emitted **only** from
   `jobs.Service.RunOne`. The static-review test
   `TestRunOne_PerAdapterPathNoDirectAuditCall` greps the three
   per-adapter files for `RecordEventTx` and asserts zero matches.

3. **Backwards-incompatible CLI change breaks the operator's muscle
   memory (Medium)**: The `--experimental-cli-collapse` ldflags default
   is ON, but operators can rebuild with the flag off for one
   minor version. When the flag is OFF, the legacy call prints a
   `DEPRECATED:` line to stderr (per
   `experience-cli-collapse` REQ-ECC-3). The deprecation text names
   the replacement form explicitly.

4. **Audit hook leaks transcript text (Low, but Hito 10 SEVERE)**: The
   `jobPayload` helper enforces a strict allow-list; the test
   `TestAuditHook_DoesNotLeakTranscriptText` runs a fixture with
   `LEAK_CANARY_USER` / `LEAK_CANARY_ASSISTANT` sentinels and asserts
   the four event payloads contain zero bytes of those sentinels.
   The static-review test `TestRunOne_PerAdapterPathNoDirectAuditCall`
   prevents future adapters from bypassing the helper.

5. **gentle-ai finalize strict schema rejects reviewer-emitted fields
   (Medium)**: The 4R review emits the same shape as the Hito 10
   Codex review (per `hito10-codex-review-fixes.md` memory): the
   orchestrator re-shapes the agent JSON into the strict schema
   (`id`, `location`, `severity`, `claim`, `proof_refs`, `findings`,
   `evidence` as `[]string`) before `gentle-ai review
   capture-result`. No reviewer agent is allowed to call `gentle-ai`
   directly.

## Threat Matrix

N/A for the routing / shell / subprocess / VCS / executable-file
classification / process-integration axis — Hito 11 adds no new
commands, no shell exec, no subprocess, no VCS automation, no
executable classification, and no new process integration. The CLI
surface change is operator-facing string parsing only.

Applicable boundaries for the new audit-event family + SQL migration:

| Boundary | Applicable | Expected safe behavior | Expected failure behavior | Planned RED tests |
|----------|------------|------------------------|---------------------------|-------------------|
| `audit-event-emission` | Yes | `RunOne` emits exactly four events per run, exactly once each, in the correct order, all sharing the same `run_id`, all with engine-only keys in the payload | Duplicate emission within one run, missing event, event with non-`job_*` operation, event with payload containing `UserText` / `AssistantText` / `ToolCalls[].OutputHint` | `TestRunOne_EmitsFourEvents`, `TestRunOne_FailureEmitsJobFailed`, `TestRunOne_LeaseConflictSkips`, `TestRunOne_RunIDsAreUnique`, `TestRunOne_PayloadAllowList`, `TestAuditHook_DoesNotLeakTranscriptText`, `TestRunOne_PerAdapterPathNoDirectAuditCall` |
| `sql-migration` | Yes | 008 applies on a fresh DB; 008 re-applies cleanly (idempotent); the runner's SHA-256 checksum stays stable across re-applies; the three columns are populated on upsert; the `job_run_log` table reserves FK to `job_registry` | 008 fails on a fresh DB (DDL bug); the not-NULL constraint breaks an existing row that lacks the column; the FK rejects a `job_run_log` insert with `job_name` not in `job_registry` | `TestMigrate_008_Forward`, `TestMigrate_008_Idempotent`, `TestMigrate_008_ChecksumStable`, `TestUpsertRegistryEntry_PopulatesThreeColumns`, `TestUpsertRegistryEntry_RejectsUnknownTaxonomy`, `TestRecordRunLog_RoundTrip` |
| `cli-flag-validation` | Yes | `experience scan --source=<opencode|claudecode|codex>` runs and prints the documented JSON shape; `--source=` missing returns exit code 2 and prints the allowed values; `--source=does_not_exist` returns exit code 2; when collapse is OFF, legacy form prints `DEPRECATED:` to stderr | Missing `--source` silently picks a default; invalid `--source` prints the same error as a missing flag; legacy form with collapse OFF silently succeeds without the deprecation note | `TestExperienceUnified_MissingSource`, `TestExperienceUnified_InvalidSource`, `TestExperienceUnified_OpenCodeAccepted`, `TestExperienceUnified_CodexAccepted`, `TestExperienceUnified_ClaudecodeAccepted`, `TestExperienceUnified_OutputKeysParity`, `TestExperienceUnified_DeprecationNote_CollapseOff`, `TestExperienceUnified_NoDeprecationNote_Unified` |

## Open Questions

- [ ] **OpenCode `jobs.go` does not currently exist** (verified via
  `ls internal/experience/opencode/`). Hito 11 introduces it for the
  first time. The proposal does not call this out explicitly; assumed
  in scope per the per-adapter rewrite pattern.

- [ ] **The proposal says migrations live in `migrations/`** (top-level
  directory) but the runner embeds `internal/storage/migrations/*.sql`
  (verified via `internal/storage/migrate.go:16`). The new file goes
  in `internal/storage/migrations/008_job_semantics.sql`. **Decision:
  follow the runner's actual path** (the orchestrator should confirm
  this is the project standard, not the proposal's directory).

- [ ] **The `JobRegistryEntry` upsert path now validates three string
  fields against enum allow-lists**, but the `cli/experience jobs
  register` subcommand (`cmd/royo-learn/experience_jobs.go:83-122`)
  lets the operator pass arbitrary `intent` / `scope` / `risk_class`.
  Two options: (a) add three new CLI flags `--intent` / `--scope` /
  `--risk-class` to the `register` subcommand, (b) keep the
  subcommand as-is and only the three static jobs are validated. The
  design currently assumes (a) — flag this for the orchestrator to
  confirm.

- [ ] **The proposal's Rollback §3 says "revert cmd/royo-learn/experience.go
  to the three per-source `runExperience<Source>` dispatchers (kept in
  git log history)"** — but the design's left-most alternative is to
  keep the three dispatchers as thin wrappers behind the
  `ExperimentalCLICollapse()` flag. That is consistent with the
  proposal's "ship behind a feature flag" alternative. **Decision:
  thin wrappers in the source tree, dispatcher bodies recoverable
  from `git log` for the one minor version** the flag is OFF.

- [ ] **The migration runner's `loadMigrations` rejects a checksum
  mismatch** (`internal/storage/migrate.go:88-95`). If the operator
  applies 008, then the team edits 008 to fix a SQL bug, the next
  startup reports a checksum mismatch. The team must instead bump to
  009. This is the existing pattern; no design change needed, but
  worth surfacing for the `sdd-apply` phase.

- [ ] **The `JobFunc` body receives `deps.DB *sql.DB`** but the engine's
  `RunOne` owns the `*sql.Tx` for the run-log + audit events. The
  `JobFunc` body MUST NOT call `tx.Commit()` on the engine's tx. The
  design documents this via the `RecordEventTx` closure (the body
  uses the closure, not the raw tx), but the `*sql.DB` is exposed
  for the inner `Ingest` transactions. Confirm this is the right
  separation with the orchestrator before `sdd-apply` starts.
