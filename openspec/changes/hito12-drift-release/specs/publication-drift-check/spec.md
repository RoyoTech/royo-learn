# Publication Drift Check Specification

## Purpose

Define the post-publication drift detector: a SQLite-backed, read-only
checker that compares the SHA-256 of a published artifact on disk
against the expected hash captured at publish time and records the
outcome in `publication_drift_state`. The capability closes the
publication-level drift gap (`PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
§18) so the operator can ask, after a `royo-learn experience promote`,
"did the published file change?" without hand-comparing hashes. The
checker is shipped behind a job (`publication_drift_check`) that reuses
the `semantic.JobFunc` runtime and `jobs.Service.RunOne` audit hook
introduced in Hito 11, and the drift outcome is gated to run only on
fully published rows (`publications.status = 'published'`), never on
half-written ones (`status = 'in_progress'`). The capability covers the
SQL migration 009, the `internal/publish/drift/` package, the
`Checker.Check` contract with its four outcomes, the
`publication_drift_check` job registration, and the `Status='published'`
gate.

## Requirements

### Requirement: Migration `009_publication_drift.sql` adds the `publication_drift_state` table

The system SHALL ship
`internal/storage/migrations/009_publication_drift.sql` adding the
`publication_drift_state` table with the columns `publication_id TEXT`
(references `publications.id`), `source TEXT`, `target_path TEXT`,
`expected_hash TEXT`, `actual_hash TEXT`,
`status TEXT CHECK(status IN ('ok','drifted','target_missing','target_unreadable'))`,
`checked_at TIMESTAMP`, and `run_id TEXT`. The migration MUST be
idempotent (`CREATE TABLE IF NOT EXISTS`) and forward-only. The
existing runner (`internal/storage/migrate.go`) has no down-migration
mechanism; the manual rollback recipe
(`DROP TABLE publication_drift_state; DELETE FROM schema_migrations WHERE version = 9;`)
MUST be documented in `tasks.md` Phase 1 for operator use.

#### Scenario: Migration applies idempotently on a fresh DB

- GIVEN a fresh SQLite database
- WHEN `migrations/009_publication_drift.sql` runs twice consecutively
- THEN both runs succeed
- AND `publication_drift_state` exists with the documented schema
- AND the `status` CHECK constraint rejects any value outside the four
  enum strings.

#### Scenario: CHECK constraint rejects unknown status

- GIVEN `publication_drift_state` exists
- WHEN an insert is attempted with `status = 'corrupted'`
- THEN the database returns a CHECK constraint violation
- AND no row is written.

#### Scenario: Manual rollback recipe is documented in tasks.md

- GIVEN `openspec/changes/hito12-drift-release/tasks.md` Phase 1
- WHEN a static review searches for "DROP TABLE publication_drift_state"
- THEN the literal SQL rollback recipe is present
- AND the `DELETE FROM schema_migrations WHERE version = 9` line is
  present.

### Requirement: `internal/publish/drift/Checker.Check` returns one of four outcomes

The system SHALL define a `Checker` type in
`internal/publish/drift/checker.go` with the method
`Check(ctx context.Context, target string, expectedHash string) (Result, error)`.
The `Result` struct carries `Status string`, `ActualHash string`, and
`Err error`. `Status` MUST be one of the four enum values
`"ok"`, `"drifted"`, `"target_missing"`, `"target_unreadable"`. The
implementation MUST call `os.Stat(target)` first; if the file is absent
the result is `target_missing` with no error and no further I/O. If
`os.Stat` succeeds, the implementation MUST open the file
(`os.Open(target)`), stream its bytes through `sha256.New()`, and
compare the digest to `expectedHash`. An `os.Open` error after a
successful `stat` (e.g. permission denied, transient I/O error) yields
`target_unreadable`. A digest mismatch yields `drifted`. A digest match
yields `ok`.

#### Scenario: `ok` outcome on hash match

- GIVEN a target file containing `hello world` and
  `expectedHash = sha256("hello world")`
- WHEN `Checker.Check(ctx, target, expectedHash)` runs
- THEN the returned `Result.Status == "ok"`
- AND `Result.ActualHash == expectedHash`
- AND `Result.Err == nil`.

#### Scenario: `drifted` outcome on hash mismatch

- GIVEN a target file whose content has been modified after publication
- WHEN `Checker.Check(ctx, target, expectedHash)` runs
- THEN the returned `Result.Status == "drifted"`
- AND `Result.ActualHash != expectedHash`
- AND `Result.ActualHash` is the hex-encoded SHA-256 of the current
  bytes on disk
- AND `Result.Err == nil`.

#### Scenario: `target_missing` outcome when the file does not exist

- GIVEN a target path that resolves to no file
- WHEN `Checker.Check(ctx, target, expectedHash)` runs
- THEN the returned `Result.Status == "target_missing"`
- AND `Result.ActualHash == ""`
- AND `Result.Err == nil`
- AND no `os.Open` call is attempted.

#### Scenario: `target_unreadable` outcome on permission denied

- GIVEN a target file that exists per `os.Stat` but cannot be opened
  (permission denied, simulated with `os.Chmod(target, 0o000)` on POSIX
  or an equivalent ACL on Windows)
- WHEN `Checker.Check(ctx, target, expectedHash)` runs
- THEN the returned `Result.Status == "target_unreadable"`
- AND `Result.ActualHash == ""`
- AND `Result.Err` is the underlying `os.Open` error wrapped via
  `errors.Join`.

### Requirement: `Checker.Check` is strictly read-only on the target

The system SHALL ensure that `Checker.Check` performs no write,
truncate, chmod, chown, chtimes, rename, or removal operation on the
target path. The implementation MUST use only `os.Stat` and `os.Open`
plus `sha256.New().Write(...)` from the resulting reader. A pre-commit
lint rule (`grep -nE 'os\.WriteFile|os\.Create|ioutil\.WriteFile' internal/publish/drift/`)
MUST return zero matches. A `contract_test.go` test MUST snapshot
`targetInfo.Mode()`, `ModTime()`, and `Size()` before and after
`Check(ctx, target, expectedHash)` and assert all three are
byte-identical.

#### Scenario: `contract_test.go` snapshot test passes

- GIVEN a fixture file in `testutil.TempDir(t)` with a known content
- WHEN the test records `Mode()`, `ModTime()`, `Size()` before the call
- AND `Checker.Check` runs
- AND the test re-reads the same three fields after the call
- THEN every field is unchanged.

#### Scenario: Pre-commit grep for write calls returns zero matches

- GIVEN `internal/publish/drift/` source tree
- WHEN
  `grep -rnE 'os\.WriteFile|os\.Create|ioutil\.WriteFile' internal/publish/drift/`
  runs
- THEN the command exits with status `1`
- AND no match line is printed.

### Requirement: `publication_drift_check` job is registered with the Hito 11 audit hook

The system SHALL register a job named `publication_drift_check` in
`job_registry` with `intent = "drift"` (new constant
`semantic.JobIntentDrift`), `scope = "project"`, `risk_class = "low"`,
and `Enabled = false`. The job MUST reuse `semantic.JobFunc`
(Hito 11) and `jobs.Service.RunOne` (Hito 11 audit hook) so the four
lifecycle events (`job_pending`, `job_running`, `job_succeeded`,
`job_failed`) are emitted exactly once per transition. The job body
iterates rows from `publications` where `status = 'published'` and
`target_path` is non-empty, computes `expected_hash` from
`publications.source_hash`, invokes `Checker.Check`, and upserts the
outcome into `publication_drift_state`. Drift outcomes are recorded in
`publication_drift_state` only; the audit-events payload contains no
`excerpt`, `user_text`, `assistant_text`, or `target_path` body.

#### Scenario: Job is registered with the four documented fields

- GIVEN the CLI/MCP startup path
- WHEN `job_registry` is read for the row
  `publication_drift_check`
- THEN `intent == "drift"`, `scope == "project"`,
  `risk_class == "low"`, `Enabled == false`
- AND the constant `semantic.JobIntentDrift` equals the literal
  string `"drift"`.

#### Scenario: Job emits exactly four lifecycle events per run

- GIVEN the job runs end-to-end with at least one publication row
- WHEN `audit_events` is queried for `run_id = <run_id>`
- THEN four rows exist in the order
  `job_pending`, `job_running`, `job_succeeded`
- AND no `job_failed` row exists on the happy path.

#### Scenario: Drift-specific payload is not in audit events

- GIVEN a publication row whose `target_path` contains a user name
  (e.g. `/home/alice/.claude/sessions/foo.jsonl`)
- WHEN the drift job runs
- THEN no `audit_events` row's JSON payload contains the substring
  `/home/alice`
- AND no payload contains the substring `excerpt`,
  `user_text`, or `assistant_text`.

### Requirement: Drift check only runs against fully published rows

The system SHALL gate the `publication_drift_check` job so it processes
only rows where `publications.status = 'published'`. Rows with
`status = 'in_progress'`, `'failed'`, `'archived'`, or any other value
MUST be skipped. The gate is implemented in the `JobFunc` body, not in
the SQL `WHERE` clause alone, so the test
`TestPublicationDriftCheck_SkipsInProgress` can prove the gate by
inserting an `in_progress` row alongside a `published` row and asserting
`publication_drift_state` only gains one new row, not two.

#### Scenario: `in_progress` row is skipped

- GIVEN two rows in `publications`: one with `status = 'published'`
  and one with `status = 'in_progress'`
- WHEN `publication_drift_check` runs
- THEN `publication_drift_state` gains exactly one new row
- AND the new row's `publication_id` is the published row, not the
  in-progress row
- AND the `in_progress` row's `target_path` is never read.

#### Scenario: Gate is encoded in the JobFunc body

- GIVEN the source of `internal/experience/jobs/service.go` and the
  drift job binding
- WHEN a static review searches for the literal `status = 'published'`
- THEN the literal appears in the drift `JobFunc` body
- AND not just in a SQL `WHERE` clause that the test cannot inspect.

### Requirement: Coverage target for `internal/publish/drift/` is at least 90 percent

The system SHALL meet the `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §4
coverage gate of ≥ 90% statement coverage on `internal/publish/drift/`.
The CI gate MUST fail below the threshold. The integration tests for
all four outcomes (`ok`, `drifted`, `target_missing`,
`target_unreadable`) MUST use a real filesystem via `testutil.TempDir`
plus `t.Cleanup`, the Hito 5/10 pattern that already passes the
Windows/Linux/macOS CI matrix.

#### Scenario: Package coverage gate

- GIVEN `go test -cover ./internal/publish/drift/...` runs in CI
- WHEN the coverage report is produced
- THEN statement coverage is at least 90 percent
- AND the CI gate fails below that threshold.

#### Scenario: All four outcomes have integration tests

- GIVEN the integration test file
  `internal/publish/drift/integration_test.go` (or equivalent)
- WHEN it runs
- THEN there is at least one test case per outcome
- AND every test uses `testutil.TempDir(t)` plus `t.Cleanup`.

## Out of scope

- Drift in process memory, in audit log file rotation, or in
  `audit_events` table content. The detector only compares on-disk
  published artifacts against the recorded expected hash.
- Cross-publication hash chain verification (parent/child consistency).
  Each `publication_id` is checked in isolation.
- Recovery actions on `drifted` outcomes (revert, re-publish). The
  detector only records; recovery is a separate operator decision.
- New audit-event names. Drift outcomes are recorded in
  `publication_drift_state`; the Hito 11 audit hook continues to emit
  the four `job_*` lifecycle events with no new operation names.

## References

- `docs/22-ADAPTER-CONTRACT.md` §1, §6
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit invariant)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 (Hito 12 acceptance
  rows), §4 (coverage gate)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 (Hito 12 row)
- `openspec/changes/hito11-semantic/specs/job-semantic-engine/spec.md`
  (runtime + audit hook reused)
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §18 (drift gap origin)
