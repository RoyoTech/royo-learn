# Spec: job-semantic-engine

## Purpose

Define the neutral `Job`/`JobResult`/`JobFunc` runtime contract, the
`JobIntent`/`JobScope`/`JobRiskClass` taxonomy, the `job_run_log` audit
table, and the audit-hook emission rule for the four lifecycle events
(`job_pending`, `job_running`, `job_succeeded`, `job_failed`) emitted
exactly once per transition by `internal/experience/jobs.Service.RunOne`.
This capability realigns the three experience adapters (`opencode`,
`claudecode`, `codex`) behind one symmetric runtime shape so the operator
can ask one question about "what is the job engine doing right now across
all three sources" (Hito 11, PR #14, `docs/26` §5).

## Requirements

### Requirement: Job / JobResult / JobFunc contract lives in `internal/experience/semantic`

The system SHALL define `Job`, `JobResult`, and `JobFunc` in
`internal/experience/semantic/job.go`. `Job` binds a `JobRegistryEntry`
(name, intent, scope, risk class, source) to a runtime `JobFunc`. A
`JobFunc` MUST have the shape
`func(ctx context.Context, deps Deps) (JobResult, error)`, MUST respect
`context` cancellation and timeout, and MUST return a `JobResult` whose
`Source` field equals the binding's `Source`. No adapter may execute
content from the transcript (per `docs/22` §2).

#### Scenario: Job contract compiles with the documented signature

- GIVEN the new package `internal/experience/semantic/`
- WHEN `go build ./internal/experience/semantic/...` runs on
  Windows/Linux/macOS
- THEN it compiles without warnings
- AND the `JobFunc` signature matches the proposal Approach §2.

#### Scenario: JobFunc respects context cancellation

- GIVEN a `JobFunc` whose body waits on `ctx.Done()`
- WHEN the caller cancels the context before the body returns
- THEN the body returns the cancellation error
- AND no audit event is emitted outside `jobs.Service.RunOne`.

### Requirement: Taxonomy enums `JobIntent`, `JobScope`, `JobRiskClass` are exhaustive

The system SHALL define three enum types in
`internal/experience/semantic/types.go`:

- `JobIntent` with at least the values `"ingest"`, `"promote"`,
  `"rebuild"`, `"cleanup"`.
- `JobScope` with at least the values `"project"`, `"global"`.
- `JobRiskClass` with at least the values `"low"`, `"medium"`,
  `"high"`.

Unknown values MUST be rejected at upsert time. The values used by the
three static ingest jobs are fixed by `experience-adapters` REQ-EA-2.

#### Scenario: Enum constants are exported and documented

- GIVEN the new package is imported
- WHEN a caller references `semantic.JobIntentIngest`,
  `semantic.JobScopeProject`, `semantic.JobRiskClassLow`
- THEN the constants are exported and equal the literal strings
  `"ingest"`, `"project"`, `"low"`.

#### Scenario: Unknown enum value is rejected at upsert

- GIVEN a `JobRegistryEntry` upsert with `intent = "unknown_intent"`
- WHEN `jobs.Repository.Upsert` is called
- THEN it returns a typed validation error
- AND no row is written to `job_registry`.

### Requirement: Audit-event constants are emitted exactly once per transition from `RunOne`

The system SHALL define the four audit-event name constants
(`"job_pending"`, `"job_running"`, `"job_succeeded"`, `"job_failed"`)
in `internal/experience/semantic/events.go`. The system SHALL emit each
constant **exactly once** per job run from
`internal/experience/jobs/service.go::RunOne`, inside the same SQLite
transaction that updates `job_state` and writes `job_run_log`. Re-runs
of the same logical job MUST emit a fresh `run_id` and a new event
sequence; duplicate emission within one run is forbidden.

#### Scenario: Happy path emits four events in order

- GIVEN a fresh lease on `experience_ingest:opencode` with one envelope
- WHEN `jobs.Service.RunOne` runs end-to-end
- THEN `audit_events` carries four rows in this order:
  `job_pending`, `job_running`, `job_succeeded`
- AND the failed event is NOT present.

#### Scenario: Failing JobFunc emits `job_failed` exactly once

- GIVEN the `JobFunc` returns a non-nil error
- WHEN `jobs.Service.RunOne` runs
- THEN `audit_events` carries `job_pending`, `job_running`,
  `job_failed` exactly once each
- AND no `job_succeeded` row is written.

#### Scenario: Per-adapter code never emits the four events directly

- GIVEN the per-adapter `JobFunc` body in
  `internal/experience/{opencode,claudecode,codex}/jobs.go`
- WHEN a static review searches for the literals `job_pending`,
  `job_running`, `job_succeeded`, `job_failed`
- THEN only `internal/experience/jobs/service.go` matches
- AND no per-adapter file contains a direct
  `storage.RecordEventTx` call with a `job_*` operation.

### Requirement: `job_run_log` table and idempotent migration

The system SHALL ship `migrations/008_job_semantics.sql` adding the
`intent TEXT`, `scope TEXT`, `risk_class TEXT` columns to
`job_registry` and a new `job_run_log` table with the columns
`run_id`, `job_name`, `state`, `started_at`, `finished_at`,
`error_code`, `error_message`, `attempt`. The migration MUST be
idempotent (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN`
guarded by a name check) and reversible through `migrations/down.sql`.

#### Scenario: Migration applies idempotently on a fresh DB

- GIVEN a fresh SQLite database
- WHEN `migrations/008_job_semantics.sql` runs twice consecutively
- THEN both runs succeed
- AND `job_run_log` exists with the documented schema
- AND `job_registry` carries the three new columns.

#### Scenario: Down migration reverses the change cleanly

- GIVEN a database where `008` has been applied
- WHEN `migrations/down.sql` runs the rollback block for `008`
- THEN `job_run_log` is dropped
- AND the three columns are removed from `job_registry`.

### Requirement: Four lifecycle events share one `run_id`

The system SHALL bind the four lifecycle events of one job run to the
same `run_id` sourced from a single row written to `job_run_log` in
the same transaction that emits the events. The `run_id` MUST be a
deterministic UUID generated by the engine (not by the per-adapter
code) and MUST appear in each `audit_events` row's JSON payload under
the key `run_id`.

#### Scenario: All four events carry the same `run_id`

- GIVEN one end-to-end `RunOne` call
- WHEN the audit query selects the four `job_*` events
- THEN every row's payload `"run_id"` field equals the same UUID.

#### Scenario: A second run gets a fresh `run_id`

- GIVEN the first `RunOne` finished successfully
- WHEN a second `RunOne` is invoked for the same job
- THEN its four events carry a new `run_id`
- AND the new value differs from the previous run.

### Requirement: Audit hook MUST NOT leak transcript text

The system SHALL ensure that the JSON payload of each `job_*` event
contains zero bytes from `ExperienceEnvelope.UserText`,
`ExperienceEnvelope.AssistantText`,
`ExperienceEnvelope.ToolCalls[].OutputHint`, or any
`ExperienceEnvelope` field that carries transcript content. The
payload MUST only carry the engine-owned fields: `job_name`,
`run_id`, `source`, `state`, `attempt`, `error_code`,
`error_message` (when present). The Hito 10 SEVERE trace-leak
invariant (`hito10-codex-review-fixes.md`,
`docs/24-EXPERIENCE-THREAT-MODEL.md` §6) is preserved.

#### Scenario: TestAuditHook_DoesNotLeakTranscriptText passes

- GIVEN a fixture envelope whose `UserText` and `AssistantText` carry
  a sentinel substring `"LEAK_CANARY_USER"` and
  `"LEAK_CANARY_ASSISTANT"`
- WHEN `RunOne` runs against the fixture
- THEN the test inspects each of the four `audit_events` payloads
- AND asserts the sentinel substrings are absent.

#### Scenario: Audit payload field allow-list is enforced

- GIVEN the JSON payload marshalling helper used by `RunOne`
- WHEN it serializes the payload for a `job_*` event
- THEN it emits only the documented keys
- AND any attempt to add a free-form field (e.g., a `details` map)
  fails a unit test.

## References

- `docs/21-EXPERIENCE-DOMAIN.md` §1, §8
- `docs/22-ADAPTER-CONTRACT.md` §1, §3
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §4
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14
