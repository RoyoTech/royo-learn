# Design: Hito 12 — Drift / Release Hardening

## Technical Approach

Add a publication-level drift detector (SQL migration 009 + new package
`internal/publish/drift/`), wire it into the Hito 11 symmetric job engine
via a new `publication_drift_check` registry row, and expose it through
a unified `experience drift` CLI subcommand and a matching
`experience_drift_status` MCP tool that emit one stable JSON envelope
(`{ "sources": [...], "publications": [...] }`). Reconcile the
cross-adapter drift policy by removing the advisory-excerpt branch in
`internal/experience/claudecode/resolve_trace.go` and pinning parity with
two new contract tests in each adapter package. Close the pre-v1.0.0
release gaps by emitting an SPDX-JSON SBOM in `.goreleaser.yml`, shipping
a self-contained `RELEASE.md` runbook, and backfilling
`CHANGELOG.md` through Hito 11. The change fits in one PR (~710 LOC);
the `gentle-ai review` correction budget is
`min(200, ceil(710/2)) = 200`.

Reference points used to write this design (all paths relative to repo
root):

- `internal/experience/jobs/service.go:512` — `Service.RunOne` signature
  verified (`func (s *Service) RunOne(ctx context.Context, projectID
  domain.ProjectID, jobName, owner string, j *semantic.Job)
  (JobRunOutcome, error)`).
- `internal/experience/semantic/types.go:27-46` — `JobIntent` enum
  currently exports `JobIntentIngest`, `JobIntentPromote`,
  `JobIntentRebuild`, `JobIntentCleanup`; `JobIntentDrift` does **not**
  exist and MUST be added in this change. The `IsValidIntent` switch
  must be extended to include it (otherwise the upsert path at
  `internal/experience/jobs/repository.go:271` rejects the value via
  `validateTaxonomy`).
- `internal/experience/jobs/service.go:59` — `Service.Register(ctx,
  projectID, JobRegistryEntry)` is the idempotent static registrar the
  drift `Job()` accessor binds through (mirrors the per-adapter `Job()`
  pattern from Hito 11).
- `internal/experience/claudecode/resolve_trace.go:100-119` — advisory
  excerpt branch on `locator.SourceHash != ""` mismatch returns
  `TraceResult{Excerpt, Redacted, SourceChanged: true,
  Code: "trace_source_changed"}`. The block is the parity gap the
  delta closes.
- `internal/storage/migrations/001_init.sql:137-149` — `publications`
  table does **not** have direct `target_path` or `source_hash`
  columns; those values live inside `targets_json` and
  `verification_json` as JSON-encoded blobs. **Design delta**: the
  drift job must JSON-decode these per row instead of selecting them
  as SQL columns (see §Architecture Decisions → Delta D1).
- `internal/storage/db.go:29-32` — `PRAGMA journal_mode = WAL`,
  `synchronous = NORMAL`, `foreign_keys = ON`, `busy_timeout = 5000`
  are already applied at connection open; no per-job pragma work.
- `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or
  disappears" (currently Codex-only §11) — the canonical source for
  the adapter contract; `openspec/specs/experience-adapters/spec.md`
  does not exist for this project, and `docs/22` remains the source of
  truth (per R-S1).
- `openspec/changes/hito11-semantic/design.md` — the style baseline
  this document follows (Architecture Decisions, Data Flow, File-Level
  Changes, Test Strategy, Migration Plan, Threat Matrix, Open
  Questions).

## Architecture Decisions

### Decision D1 — Gate is encoded in the JobFunc body, not in SQL WHERE alone

**Choice (R-S3, resolved)**: Option A — the `status = 'published'`
gate is encoded **in the `JobFunc` body** (Go) in addition to any SQL
filter. The `publication_drift_check` `JobFunc` walks the
`publications` row set returned by the SELECT, then iterates and
explicitly skips rows whose `status` is anything other than
`'published'`:

```go
// internal/publish/drift/jobs.go (sketch — full body in §Component
// Design → Job registration)
func runPublicationDriftCheck(d semantic.Deps) (semantic.JobResult, error) {
    rows, err := d.DB.QueryContext(d.Ctx, `
        SELECT id, source, targets_json, verification_json, status
          FROM publications
         WHERE target_path_candidate IS NOT NULL
            OR json_array_length(targets_json) > 0`)
    if err != nil { return result, err }
    defer rows.Close()

    var checked, skipped int
    for rows.Next() {
        var (
            id, source, targetsJSON, verificationJSON, status string
        )
        if err := rows.Scan(&id, &source, &targetsJSON, &verificationJSON, &status); err != nil {
            return result, err
        }
        // GATE — encoded in Go, not in SQL WHERE alone.
        if status != "published" {
            skipped++
            continue
        }
        // ... decode targets_json + verification_json, call Checker.Check, upsert
        checked++
    }
    return semantic.JobResult{ /* ... */ }, rows.Err()
}
```

**Alternatives considered**:

- *Option B — SQL `WHERE status = 'published'` only*: rejected. The
  spec (`publication-drift-check` REQ-PDC-3) explicitly requires the
  gate to be testable by reading the Go source: "the gate is
  implemented in the `JobFunc` body, not in the SQL `WHERE` clause
  alone, so the test `TestPublicationDriftCheck_SkipsInProgress` can
  prove the gate by inserting an `in_progress` row alongside a
  `published` row and asserting `publication_drift_state` only gains
  one new row, not two." A pure-SQL gate would be invisible to the
  static-review contract the spec mandates.
- *Two queries (one for `published`, one for `in_progress`)*: rejected
  — adds a second round-trip and a second code path; the in-process
  gate covers both intents in one pass.

**Rationale**:

1. **Spec conformance**: REQ-PDC-3 says the gate MUST be visible to a
   static review of the `JobFunc` body. Option A satisfies this; Option
   B does not.
2. **Defense in depth**: even if the SQL is later refactored and the
   WHERE clause is dropped, the Go-side gate still protects
   `publication_drift_state` from being polluted by in-progress rows.
3. **Test visibility**: the parity test
   `TestPublicationDriftCheck_SkipsInProgress` inserts both rows and
   asserts the `publication_drift_state` row count grows by exactly
   one. With Option A the test proves the Go gate; with Option B it
   could only prove the SQL gate (and would silently pass if the Go
   body later drifted away from the SQL).
4. **Operational debuggability**: when an operator investigates a
   "drift detector says no rows checked" incident, they can grep the
   `JobFunc` body and immediately see the `status != "published"`
   skip; with Option B they have to correlate the SQL with the test
   fixtures.

### Decision D2 — `target_path` and `source_hash` are decoded from JSON blobs (not direct columns)

**Choice (R-S4, delta surfaced)**: The drift job **does not** SELECT
`target_path` and `source_hash` as SQL columns because they do not
exist as columns on `publications`. They live inside the
`targets_json` and `verification_json` JSON blobs written by
`internal/storage/repo_publications.go::SavePublication` (lines 7-12,
38-52). The drift job decodes them per row using the standard
`encoding/json` package:

```go
// internal/publish/drift/jobs.go (sketch)
type targetsBlob struct {
    Targets []struct {
        Path       string `json:"path"`
        SHA256     string `json:"sha256"`
        Kind       string `json:"kind"` // "file" | "directory"
    } `json:"targets"`
}

type verificationBlob struct {
    SourceHash string `json:"source_hash"`
    // ... other fields elided
}
```

**Alternatives considered**:

- *Add `target_path TEXT` and `source_hash TEXT` columns to
  `publications` via a new migration (009b)*: rejected — out of scope
  per the proposal's "Forward-only migration 009" decision. Adding
  columns to `publications` would also require a data backfill (every
  existing row's `targets_json` would need to be decoded and
  re-encoded into the new columns) which the change explicitly does
  not ship.
- *Compute hash on-the-fly from the published JSON envelope*: rejected
  — `expected_hash` must be the hash captured **at publish time**,
  not a re-hash of the current source. The JSON blobs carry the
  recorded hash; re-hashing defeats the drift detector's purpose.

**Rationale**: the JSON-blob shape is the existing schema and changing
it is a separate migration. The drift job must work with what is on
disk today. If the JSON decode fails for a row, the job records
`target_unreadable` with the underlying `json.Unmarshal` error and
continues to the next row (the row is not fatal — the detector is
designed to fail soft per row, hard only when the entire DB is
unreachable).

### Decision D3 — New `JobIntentDrift` constant is added to the `JobIntent` enum

**Choice (R-S4, delta surfaced)**: Add the constant
`JobIntentDrift JobIntent = "drift"` to
`internal/experience/semantic/types.go`, plus extend the
`IsValidIntent` switch and the `TestJobIntent_KnownValues` test case
to include it. Without this change, the upsert path
(`Repository.UpsertRegistryEntry` →
`validateTaxonomy(intent, scope, riskClass)` at
`internal/experience/jobs/repository.go:271-327`) rejects the new
value with `domain.NewValidationError(domain.ErrInvalidArgument, ...)`.

```go
// internal/experience/semantic/types.go (extend)
const (
    JobIntentIngest  JobIntent = "ingest"
    JobIntentPromote JobIntent = "promote"
    JobIntentRebuild JobIntent = "rebuild"
    JobIntentCleanup JobIntent = "cleanup"
    JobIntentDrift   JobIntent = "drift"   // Hito 12
)

func IsValidIntent(i JobIntent) bool {
    switch i {
    case JobIntentIngest, JobIntentPromote, JobIntentRebuild,
        JobIntentCleanup, JobIntentDrift:
        return true
    default:
        return false
    }
}
```

**Alternatives considered**:

- *Reuse `JobIntentCleanup` for the drift job*: rejected — `cleanup`
  signals a destructive operation to the operator; drift detection is
  read-only (`docs/24-EXPERIENCE-THREAT-MODEL.md` §6 invariant).
  Conflating the two hides risk class from the operator dashboard.
- *Add the drift intent as an unexported constant*:
  rejected — every other `JobIntent` value is exported and surfaced
  in audit-event payloads; an unexported value would not appear in the
  `job_pending` / `job_running` JSON, breaking the audit pipeline's
  ability to filter drift jobs.

**Rationale**: the Hito 11 symmetric engine treats `intent` as the
operator-facing category. Drift is a distinct operational concern
(read-only monitor) that does not fit any of the existing four
buckets. Adding the fifth constant keeps the enum exhaustive without
re-opening `semantic` for future drift-flavoured work.

### Decision D4 — `Checker.Check` is strictly read-only via a static-review contract test

**Choice**: The `Checker.Check(ctx, target, expectedHash) (Result,
error)` contract uses **only** `os.Stat` + `os.Open` + `sha256.New()`.
The package's `contract_test.go` snapshots `Mode()`, `ModTime()`, and
`Size()` before and after the call and asserts byte-identical state.
A pre-commit `grep -nE 'os\.WriteFile|os\.Create|ioutil\.WriteFile'
internal/publish/drift/` MUST return zero matches; this rule is
encoded as a CI grep step (the existing
`TestRunOne_PerAdapterPathNoDirectAuditCall` pattern at
`internal/experience/jobs/service_runone_test.go:424-472` is the
reference shape).

```go
// internal/publish/drift/contract_test.go (sketch)
func TestChecker_IsReadOnly(t *testing.T) {
    dir := testutil.TempDir(t)
    target := filepath.Join(dir, "art.bin")
    require.NoError(t, os.WriteFile(target, []byte("hello world"), 0o644))

    before, err := os.Stat(target)
    require.NoError(t, err)

    var c Checker
    _, _ = c.Check(context.Background(), target, sha256Hex("hello world"))

    after, err := os.Stat(target)
    require.NoError(t, err)
    require.Equal(t, before.Mode(), after.Mode())
    require.Equal(t, before.ModTime(), after.ModTime())
    require.Equal(t, before.Size(), after.Size())
}
```

**Alternatives considered**:

- *Compile-time lint via `go vet` custom check*: rejected — the
  existing project uses grep-based pre-commit rules for similar
  guarantees; a `go vet` analyzer would require a separate module and
  build step.
- *Wrapper interface that forbids write calls*: rejected — `os.Stat`
  and `os.Open` are the right primitives; a wrapper interface adds
  indirection without preventing the underlying calls.

**Rationale**: the spec (`publication-drift-check` REQ-PDC-4)
mandates the lint rule and the contract test. Both are required
defences because the first catches the introduction of a write call
at review time and the second catches the introduction of an
accidental mutation (e.g. via `os.Chtimes` or temp-file cleanup) at
test time.

### Decision D5 — `experience drift` CLI handler is shared with the MCP tool

**Choice**: A single handler function `driftHandler(ctx, db, sourceFilter)`
in a new `internal/experience/cli/drift.go` produces the JSON
envelope. The `cmd/royo-learn/experience.go::runExperienceDrift`
function calls it and writes the JSON to `stdout`. The
`internal/mcp/experience.go::experience_drift_status` tool handler
calls the same `driftHandler`. Both surfaces emit byte-equal JSON
(modulo field ordering — sorted in a stable order before equality
check). This satisfies the spec's `drift-cli-mcp` REQ-DC-4 "MCP
envelope matches CLI envelope" scenario.

**Alternatives considered**:

- *Two independent handlers*: rejected — the spec's golden-fixture test
  pins one envelope shape; two handlers invite drift.
- *Generate MCP handler from CLI via codegen*: rejected — the
  project does not use codegen for handler plumbing; the explicit
  call is simpler and the unit test `TestExperienceDrift_GoldenEnvelope`
  covers parity.

**Rationale**: a shared handler is the smallest surface that satisfies
both the spec and the Hito 10 SEVERE invariant (no transcript text in
any audit / response payload). The handler enforces the
`filepath.Base(target_path)` redaction at the point of JSON
construction, so neither caller can accidentally emit the full path.

### Decision D6 — Unified CLI dispatcher preserves per-source `experience <source> scan` for one minor version

**Choice**: `cmd/royo-learn/experience.go` keeps the three
per-source `runExperience<Source>Scan` functions as **thin wrappers**
around `runExperienceUnified` (mirroring the Hito 11 pattern at
`internal/experience/cli/collapse.go`). The new
`runExperienceDrift` subcommand is additive; it does NOT replace any
existing subcommand. When `--experimental-cli-collapse` is OFF, the
per-source dispatchers print a `DEPRECATED:` line to `stderr` and
delegate; when ON they are hidden by the help text but reachable for
one minor version (the Hito 11 rollback policy).

**Rationale**: the proposal's rollback §3 says the drift subcommand
is additive and not a replacement; this decision implements that
explicitly and aligns with the Hito 11 `ExperimentalCLICollapse()`
flag that already gates the unified-vs-legacy CLI behaviour.

## Data Flow

```
operator types:
  `royo-learn experience drift --all-sources`
                                  │
                                  ▼
       cmd/royo-learn/experience.go:runExperienceDrift
                                  │
                                  │ parse flags (--all-sources,
                                  │ --source=<opencode|claudecode|codex>)
                                  │ reject mutually-exclusive combination
                                  ▼
       internal/experience/cli/drift.go:driftHandler(ctx, db, sourceFilter)
                                  │
                                  │ 1. open jobs.Service
                                  │ 2. register publication_drift_check
                                  │    (semantic.JobIntentDrift) idempotent
                                  │ 3. service.RunOne(ctx, projectID,
                                  │      "publication_drift_check", owner, job)
                                  │
                                  ▼
       jobs.Service.RunOne ─────────────────────────────────────────┐
                                  │                                  │
                                  │ 1. tx.Begin                      │
                                  │ 2. recordEventTx(job_pending)    │ ─► audit_events
                                  │ 3. repo.RecordRunLog(stub)       │ ─► job_run_log
                                  │ 4. service.AcquireLease          │
                                  │ 5. recordEventTx(job_running)    │ ─► audit_events
                                  │ 6. repo.UpdateRunLogAttempt(1)   │ ─► job_run_log
                                  │ 7. semantic.Job.JobFunc(deps)    │
                                  │      │                           │
                                  │      │  SELECT publications      │
                                  │      │   WHERE target_path_candidate
                                  │      │      IS NOT NULL
                                  │      │      OR json_array_length
                                  │      │         (targets_json) > 0
                                  │      │                           │
                                  │      │  FOR EACH row:            │
                                  │      │   if status != 'published': SKIP  ← GATE
                                  │      │   decode targets_json → target_path
                                  │      │   decode verification_json → expected_hash
                                  │      │   Checker.Check(ctx, target, expected)
                                  │      │     ├─ os.Stat
                                  │      │     ├─ os.Open + sha256
                                  │      │     └─ returns {ok|drifted|target_missing|target_unreadable}
                                  │      │   UPSERT publication_drift_state
                                  │      │                           │
                                  │      ▼                           │
                                  │  semantic.JobResult              │
                                  │ 8. (ok)  job_succeeded + ReleaseLease + FinishRunLog
                                  │    (err) job_failed   + ReleaseLease + FinishRunLog
                                  │ 9. tx.Commit                     │
                                  │                                  │
                                  ▼                                  │
       driftHandler reads publication_drift_state ───────────────────┘
                                  │
                                  │  group by source, count statuses
                                  │  redact target_path → filepath.Base(...)
                                  ▼
       JSON envelope { sources: [...], publications: [...] }
                                  │
                                  ▼
       stdout (logs on stderr)
```

The four `audit_events` rows + one `job_run_log` row are emitted in
ONE `*sql.Tx` to guarantee atomicity (the Hito 11 invariant at
`internal/experience/jobs/service.go:512`). The `JobFunc` body opens
its own per-row transactions to upsert `publication_drift_state`; this
preserves the existing per-row isolation pattern from
`internal/experience/service.go:236` (the `IngestEnvelope` shape).

## File-Level Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/storage/migrations/009_publication_drift.sql` | Create | Adds `publication_drift_state` table with CHECK constraint, FK to `publications`, three indexes |
| `internal/storage/migrate_test.go` | Modify (additive) | Adds `TestMigrate_009_Forward`, `TestMigrate_009_Idempotent`, `TestMigrate_009_ChecksumStable` |
| `internal/publish/drift/checker.go` | Create | `Checker`, `Result`, `Status` enum, `NewChecker`, `Check` method |
| `internal/publish/drift/checker_test.go` | Create | Unit tests for all four outcomes + edge cases |
| `internal/publish/drift/repository.go` | Create | `RecordDrift`, `ListDrift`; SQL string constants; per-row transactions |
| `internal/publish/drift/repository_test.go` | Create | Round-trip tests; CHECK constraint rejection test |
| `internal/publish/drift/contract_test.go` | Create | Read-only contract: snapshot Mode/ModTime/Size before/after |
| `internal/publish/drift/integration_test.go` | Create | All four outcomes with `testutil.TempDir` |
| `internal/publish/drift/jobs.go` | Create | `Job() *semantic.Job` accessor + `JobName` constant + `runPublicationDriftCheck` |
| `internal/publish/drift/jobs_test.go` | Create | `TestPublicationDriftCheck_SkipsInProgress`, `TestPublicationDriftCheck_AllFourOutcomes` |
| `internal/experience/semantic/types.go` | Modify | Adds `JobIntentDrift = "drift"` constant; extends `IsValidIntent` switch |
| `internal/experience/semantic/types_test.go` | Modify | Adds `TestJobIntent_DriftAccepted`, extends `TestJobIntent_KnownValues` |
| `internal/experience/jobs/jobs.go` | Referenced | `JobRunOutcome` already exists (Hito 11); no change |
| `internal/experience/jobs/repository.go` | Referenced | `UpsertRegistryEntry` already accepts the new intent via the extended `IsValidIntent`; no change |
| `internal/experience/claudecode/resolve_trace.go` | Modify | Removes advisory excerpt branch at lines 100-119; returns empty Excerpt on source mismatch |
| `internal/experience/claudecode/resolve_trace_test.go` | Modify | Adds `TestResolveTrace_SourceChanged_OmitsExcerpt`; updates the advisory-excerpt comment in `TestResolveTrace_SourceChanged` |
| `internal/experience/opencode/resolve_trace_test.go` | Modify | Adds `TestResolveTrace_SourceChanged_OmitsExcerpt` (passes today; pins the parity) |
| `internal/experience/codex/resolve_trace_test.go` | Modify | Adds `TestResolveTrace_SourceChanged_OmitsExcerpt` (passes today; pins the parity) |
| `internal/experience/cli/drift.go` | Create | Shared `driftHandler` for CLI + MCP; emits the stable JSON envelope; redacts `target_path` to basename |
| `internal/experience/cli/drift_test.go` | Create | Golden envelope test; PII marker test; filter flag test |
| `cmd/royo-learn/experience.go` | Modify | Adds `runExperienceDrift` subcommand with `--all-sources` / `--source=` flags |
| `cmd/royo-learn/drift_test.go` | Create | CLI golden-fixture test (`TestExperienceDrift_GoldenEnvelope`) |
| `internal/mcp/experience.go` | Modify | Registers `experience_drift_status` tool, calls `driftHandler` |
| `internal/mcp/experience_test.go` | Modify | Adds `TestExperienceDriftStatus_Schema` + zero-PII assertion |
| `.goreleaser.yml` | Modify (additive) | Adds `sboms:` block under `archives:` with `formats: ['spdx-json']` |
| `RELEASE.md` | Create | Self-contained release runbook (5 sections) |
| `CHANGELOG.md` | Modify (additive) | Backfills Hito 8/9/10/11 entries from `git log` PR titles; demotes v1.0.0 ⏳ marker |
| `tests/release/goreleaser_snapshot_test.go` | Create | Snapshot test asserting `*.spdx.json` is present in `dist/` |
| `docs/15-OPERATIONS.md` | Modify (additive) | Adds "Release runbook" section linking to `RELEASE.md` |
| `docs/22-ADAPTER-CONTRACT.md` | Modify (additive) | Scenario "Source changes or disappears" lifted to cross-adapter; one sentence tightening |
| `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 | Modify (additive) | Adds Hito 12 acceptance rows for the four drift outcomes, the unified CLI/MCP envelope, the adapter parity |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §5 | Modify | Marks Hito 12 row as "in flight" with link to the proposal |
| `openspec/changes/hito12-drift-release/design.md` | Create | This document |
| `openspec/changes/hito12-drift-release/specs/experience-adapters/spec.md` | Already exists | Delta source of truth (no further edits) |

Total: ~14 created, ~12 modified, 0 deleted.

## Component Design

### A. `internal/publish/drift/checker.go`

```go
package drift

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "os"
)

// Status is the outcome of a single drift check on a target. The
// values are stable strings persisted in publication_drift_state.status
// and emitted in the JSON envelope.
type Status string

const (
    StatusOK              Status = "ok"
    StatusDrifted         Status = "drifted"
    StatusTargetMissing   Status = "target_missing"
    StatusTargetUnreadable Status = "target_unreadable"
)

// ErrTargetMissing is returned wrapped by Result.Err when the target
// path resolves to no file (Status == StatusTargetMissing).
var ErrTargetMissing = errors.New("drift: target missing")

// ErrTargetUnreadable is returned wrapped by Result.Err when the
// target path exists per os.Stat but cannot be opened
// (Status == StatusTargetUnreadable).
var ErrTargetUnreadable = errors.New("drift: target unreadable")

// Result is the typed outcome of one Checker.Check invocation. Status
// is one of the four enum values above. ActualHash is the hex-encoded
// SHA-256 of the bytes on disk, or "" when Status is missing /
// unreadable. Err is nil for OK and Drifted outcomes; for the two
// failure outcomes it carries the wrapped sentinel plus the
// underlying error (see errors.Join).
type Result struct {
    Status     Status
    ActualHash string
    Err        error
}

// Checker computes SHA-256 of a published target and compares it to
// the expected hash captured at publish time. It is strictly
// read-only on the target — see contract_test.go.
type Checker struct {
    // openFn is the function used to open the target. Tests may
    // inject a mock; production callers leave it nil and use the
    // default (os.Open).
    openFn func(name string) (*os.File, error)
}

// NewChecker returns a Checker using os.Open.
func NewChecker() *Checker {
    return &Checker{openFn: os.Open}
}

// Check computes the SHA-256 of the target on disk and compares it to
// expectedHash. The implementation is strictly read-only — see the
// contract test and the pre-commit grep rule documented in the
// proposal.
//
// Decision order:
//   1. os.Stat(target) → target_missing on ENOENT (no further I/O)
//   2. os.Open(target)  → target_unreadable on error after successful stat
//   3. sha256 streaming → drifted on mismatch, ok on match
func (c *Checker) Check(ctx context.Context, target, expectedHash string) (Result, error) {
    if err := ctx.Err(); err != nil {
        return Result{Status: StatusTargetUnreadable, Err: fmt.Errorf("drift: ctx: %w", err)}, nil
    }
    info, err := os.Stat(target)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return Result{Status: StatusTargetMissing, Err: ErrTargetMissing}, nil
        }
        return Result{Status: StatusTargetUnreadable, Err: errors.Join(ErrTargetUnreadable, err)}, nil
    }
    _ = info // contract_test.go asserts the snapshot is unchanged

    open := c.openFn
    if open == nil {
        open = os.Open
    }
    f, err := open(target)
    if err != nil {
        return Result{Status: StatusTargetUnreadable, Err: errors.Join(ErrTargetUnreadable, err)}, nil
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return Result{Status: StatusTargetUnreadable, Err: errors.Join(ErrTargetUnreadable, err)}, nil
    }
    actual := hex.EncodeToString(h.Sum(nil))
    if actual != expectedHash {
        return Result{Status: StatusDrifted, ActualHash: actual}, nil
    }
    return Result{Status: StatusOK, ActualHash: actual}, nil
}
```

Read-only enforcement:

- Package comment in `checker.go` (above the package declaration)
  states the invariant.
- `contract_test.go` snapshots `Mode`/`ModTime`/`Size` before and
  after the call.
- Pre-commit grep (CI step) fails on any of the patterns
  `os.WriteFile`, `os.Create`, `ioutil.WriteFile` under
  `internal/publish/drift/`.

### B. `internal/publish/drift/repository.go`

```go
package drift

// RecordDrift upserts one row into publication_drift_state keyed by
// (publication_id, target_path). The method opens a short-lived tx
// per call (no project-level lock; drift detection is read-only on
// the target and writes only to publication_drift_state).
func (r *Repository) RecordDrift(ctx context.Context, row DriftRow) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer tx.Rollback()
    _, err = tx.ExecContext(ctx, `
        INSERT INTO publication_drift_state
            (publication_id, source, target_path, expected_hash,
             actual_hash, status, checked_at, run_id)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(publication_id, target_path) DO UPDATE SET
            source         = excluded.source,
            expected_hash  = excluded.expected_hash,
            actual_hash    = excluded.actual_hash,
            status         = excluded.status,
            checked_at     = excluded.checked_at,
            run_id         = excluded.run_id`,
        row.PublicationID, row.Source, row.TargetPath,
        row.ExpectedHash, row.ActualHash, string(row.Status),
        row.CheckedAt.UTC().Format(time.RFC3339), row.RunID,
    )
    if err != nil { return err }
    return tx.Commit()
}
```

`ListDrift(ctx, filters)` returns rows for the CLI/MCP envelope;
filters include `source` and `run_id`.

### C. Migration `009_publication_drift.sql`

```sql
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS publication_drift_state (
    publication_id TEXT NOT NULL REFERENCES publications(id),
    source         TEXT NOT NULL CHECK(source IN ('opencode','claudecode','codex')),
    target_path    TEXT NOT NULL,
    expected_hash  TEXT NOT NULL,
    actual_hash    TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL CHECK(status IN
                    ('ok','drifted','target_missing','target_unreadable')),
    checked_at     TEXT NOT NULL,
    run_id         TEXT NOT NULL,
    PRIMARY KEY (publication_id, target_path)
);

CREATE INDEX IF NOT EXISTS idx_drift_status_checked
    ON publication_drift_state(status, checked_at);
CREATE INDEX IF NOT EXISTS idx_drift_run_id
    ON publication_drift_state(run_id);
CREATE INDEX IF NOT EXISTS idx_drift_publication
    ON publication_drift_state(publication_id);
```

Notes:

- `PRIMARY KEY (publication_id, target_path)` allows multiple checks
  per publication (one row per target path) and provides the
  upsert-conflict target.
- The CHECK constraint enforces the four enum values; an insert with
  `status = 'corrupted'` is rejected by SQLite at write time.
- `CREATE TABLE IF NOT EXISTS` provides idempotency at the SQL
  level; the migration runner (`internal/storage/migrate.go:106-140`)
  provides idempotency at the checksum level.

### D. Job registration: `internal/publish/drift/jobs.go`

```go
package drift

import (
    "context"
    "encoding/json"

    "agent-royo-learn/internal/domain"
    "agent-royo-learn/internal/experience/semantic"
)

const JobName = "publication_drift_check"

// Job returns the runtime contract for the publication drift check.
// The job is registered at startup via Service.Register with
// Enabled = false per the Hito 11 invariant; the Hito 3 --watch flip
// is deferred (out of scope for Hito 12).
func Job() *semantic.Job {
    return &semantic.Job{
        Entry: JobRegistryEntry(),  // intent=drift, scope=project, risk_class=low
        JobFunc: runPublicationDriftCheck,
    }
}

// runPublicationDriftCheck is the body invoked by jobs.Service.RunOne.
// The GATE is encoded in Go (Decision D1), not just in SQL WHERE.
func runPublicationDriftCheck(d semantic.Deps) (semantic.JobResult, error) {
    // SELECT publications; iterate; gate on status='published'.
    // See Decision D1 for the full body sketch.
    // ...
}
```

Default `default_interval_sec = 3600` (1h), `default_max_retries = 3`,
`enabled = false`.

### E. CLI: `cmd/royo-learn/experience.go::runExperienceDrift`

Registered as the `experience drift` subcommand. Flags:

- `--all-sources` (bool, default `true`): emit both `sources` and
  `publications` sections, no source filter.
- `--source=<opencode|claudecode|codex>` (string, mutually exclusive
  with `--all-sources`): filter to one source. Passing both flags
  returns exit code 2 with usage on stderr.
- `--json` (bool, default `true`): render the envelope to stdout as
  JSON. When `false`, render a compact human-readable summary.

Output:

```json
{
  "sources": [
    { "source": "opencode",   "drifted": 3, "ok": 12, "missing": 1 },
    { "source": "claudecode", "drifted": 0, "ok":  7, "missing": 0 },
    { "source": "codex",      "drifted": 1, "ok":  4, "missing": 2 }
  ],
  "publications": [
    {
      "publication_id": "01HZX…",
      "source": "opencode",
      "target_path": "foo.jsonl",
      "status": "drifted",
      "expected_hash": "abc123…",
      "actual_hash": "def456…"
    }
  ]
}
```

`target_path` is redacted to `filepath.Base(target_path)` by the
shared handler (Decision D5). Logs go to stderr.

### F. MCP tool: `experience_drift_status`

Registered in `internal/mcp/experience.go`. Schema:

```go
{
    Name:        "experience_drift_status",
    Description: "Returns the drift status across all experience sources.",
    RequiredProfile: "admin",
    InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
    Handler:     mcpHandler(driftHandler),
}
```

The handler calls the same `driftHandler` as the CLI; both surfaces
emit byte-equal JSON (modulo field ordering).

### G. Adapter parity: `internal/experience/claudecode/resolve_trace.go`

Remove the advisory excerpt branch at lines 100-119 of the current
file. The replacement returns the same `TraceResult{SourceChanged:
true, Code: "trace_source_changed"}` shape as the opencode and codex
adapters, with `Excerpt == ""` and `Redacted == false`. The
contract tests in `resolve_trace_test.go` for the three adapters
share the name `TestResolveTrace_SourceChanged_OmitsExcerpt` and
assert the same shape on each adapter package.

### H. Release extras

`.goreleaser.yml` — diff to add under `archives:`:

```yaml
sboms:
  - id: spdx
    artifacts: archive
    formats: ['spdx-json']
```

`RELEASE.md` — new file at repo root with five sections in order:

1. Trigger table (PR merge → ci.yml quality matrix → release.yml
   workflow_run → GoReleaser build → artifact upload).
2. Required CI checks (cross-build Windows/Linux/macOS,
   `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean,
   coverage gates per `docs/25` §4).
3. Tag creation (`vX.Y.Z` for stable, `vX.Y.Z-pre.N` for prereleases;
   `workflow_run` SHA alignment; preconditions that all four Hito 12
   deliverables must be merged AND the CHANGELOG backfilled before
   tagging).
4. Install verification (`install.sh` / `install.ps1` invocations,
   SHA-256 verification recipe).
5. Rollback recipe (`install.sh --uninstall` + reinstall previous
   tag).

`CHANGELOG.md` backfill — script (manual or Go utility) reads `git log
--oneline` for the merge SHAs of Hitos 8, 9, 10, 11, extracts PR
titles, and moves the entries from `[Unreleased]` to
`[0.8.0]` / `[0.9.0]` / `[0.10.0]` / `[0.11.0]` with `[^pr-N]: #N`
footnotes. The `v1.0.0` ⏳ marker is demoted to a "no tag yet"
section referencing `RELEASE.md`.

`docs/15-OPERATIONS.md` — additive "Release runbook" section with a
single Markdown link `[Release runbook](../RELEASE.md)`.

## Data Model

```sql
CREATE TABLE IF NOT EXISTS publication_drift_state (
    publication_id TEXT NOT NULL REFERENCES publications(id),
    source         TEXT NOT NULL CHECK(source IN ('opencode','claudecode','codex')),
    target_path    TEXT NOT NULL,
    expected_hash  TEXT NOT NULL,
    actual_hash    TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL CHECK(status IN
                    ('ok','drifted','target_missing','target_unreadable')),
    checked_at     TEXT NOT NULL,
    run_id         TEXT NOT NULL,
    PRIMARY KEY (publication_id, target_path)
);

CREATE INDEX IF NOT EXISTS idx_drift_status_checked
    ON publication_drift_state(status, checked_at);
CREATE INDEX IF NOT EXISTS idx_drift_run_id
    ON publication_drift_state(run_id);
CREATE INDEX IF NOT EXISTS idx_drift_publication
    ON publication_drift_state(publication_id);
```

Index rationale:

- `idx_drift_status_checked` — operator query "what drifted in the
  last 24h" (filter by `status` + `checked_at`).
- `idx_drift_run_id` — join key for the four-event audit query
  (matches the Hito 11 `idx_job_run_log_run_id` pattern).
- `idx_drift_publication` — per-publication lookup ("did THIS
  publication drift?").

## API Contracts

### CLI JSON envelope

```json
{
  "sources": [
    { "source": "opencode",   "drifted": <int>, "ok": <int>, "missing": <int> },
    { "source": "claudecode", "drifted": <int>, "ok": <int>, "missing": <int> },
    { "source": "codex",      "drifted": <int>, "ok": <int>, "missing": <int> }
  ],
  "publications": [
    {
      "publication_id": "<uuid>",
      "source": "<opencode|claudecode|codex>",
      "target_path": "<basename>",
      "status": "<ok|drifted|target_missing|target_unreadable>",
      "expected_hash": "<sha256-hex>",
      "actual_hash":   "<sha256-hex|empty>"
    }
  ]
}
```

Top-level keys are exactly `sources` and `publications` — no
`excerpt`, `user_text`, `assistant_text`, `tool_calls`, or `actor`
fields. `target_path` is always `filepath.Base(...)`; full paths are
not present in the JSON. The comparator in the golden-fixture test
sorts both arrays by `source`/`publication_id` before equality so the
test does not depend on map iteration order.

### MCP tool schema

```json
{
  "name": "experience_drift_status",
  "description": "Returns drift status across the three experience sources and the publication layer.",
  "inputSchema": { "type": "object", "properties": {} },
  "required_profile": "admin",
  "output": <same envelope as CLI>
}
```

## Concurrency & Locking

- **Does the drift job need a project-level lock?** No. The job is
  read-only on the target (only `os.Stat` + `os.Open`) and writes
  only to `publication_drift_state` (a brand-new table with no
  contention from other writers). `Service.AcquireLease` provides
  the per-(project, job_name) lease that prevents two concurrent
  runs of the same job, which is sufficient.

- **Race between drift job and rollback?** The drift job runs against
  `publications.status = 'published'`. The rollback path (Hito 6
  `rollback_json`) changes a publication back to `approved` (not
  `'in_progress'`, not back to `'pending'`); it does not
  re-introduce the same target. If a publication is moved out of
  `'published'` between SELECT and the per-row gate, the gate
  catches it (`status != 'published'` → SKIP) and no row is written
  to `publication_drift_state`. Documented behaviour.

- **SQLite busy_timeout**: already configured at
  `internal/storage/db.go:32` (`PRAGMA busy_timeout = 5000`); no
  per-job pragma work is needed. The drift job opens short-lived
  transactions for `RecordDrift` and inherits the connection-level
  timeout.

- **Per-row upsert isolation**: `RecordDrift` opens its own
  `*sql.Tx` per call; failures roll back without affecting sibling
  rows. This preserves the "fail soft per row, hard only when the
  entire DB is unreachable" design rule.

- **MCP tool concurrency**: `experience_drift_status` is read-only;
  it can be invoked concurrently from multiple MCP clients. Each
  invocation calls `Service.RunOne` which acquires the
  per-(project, job_name) lease; concurrent invocations either queue
  or see `job_failed` with `code = "job_lease_held"` (the Hito 11
  invariant).

## Test Strategy

### Unit tests

| Test file | Test function | Purpose |
|-----------|---------------|---------|
| `internal/publish/drift/checker_test.go` | `TestChecker_OKOnHashMatch` | `ok` outcome |
| `…/checker_test.go` | `TestChecker_DriftedOnHashMismatch` | `drifted` outcome |
| `…/checker_test.go` | `TestChecker_TargetMissingReturnsErrTargetMissing` | `target_missing` |
| `…/checker_test.go` | `TestChecker_TargetUnreadableWrapsUnderlying` | `target_unreadable` with `errors.Join` |
| `…/checker_test.go` | `TestChecker_RespectsContextCancellation` | `ctx.Done()` returns `target_unreadable` |
| `…/contract_test.go` | `TestChecker_IsReadOnly` | Mode/ModTime/Size snapshot |
| `…/contract_test.go` | `TestChecker_NoWriteAPIsImported` | Static grep for forbidden APIs (mirrors `TestRunOne_PerAdapterPathNoDirectAuditCall`) |
| `internal/publish/drift/repository_test.go` | `TestRecordDrift_RoundTrip` | Insert + read back |
| `…/repository_test.go` | `TestRecordDrift_RejectsUnknownStatus` | CHECK constraint |
| `…/repository_test.go` | `TestRecordDrift_UpsertOnConflict` | Same `(publication_id, target_path)` updates instead of duplicating |
| `internal/publish/drift/jobs_test.go` | `TestPublicationDriftCheck_SkipsInProgress` | Gate (Decision D1) |
| `…/jobs_test.go` | `TestPublicationDriftCheck_GateInJobFuncBody` | Static grep proves `status != "published"` literal in the JobFunc |
| `…/jobs_test.go` | `TestPublicationDriftCheck_AllFourOutcomes` | One row per outcome, `publication_drift_state` populated correctly |
| `internal/experience/semantic/types_test.go` | `TestJobIntent_DriftAccepted` | New constant accepted |
| `internal/experience/opencode/resolve_trace_test.go` | `TestResolveTrace_SourceChanged_OmitsExcerpt` | Parity test (passes today; pins the invariant) |
| `internal/experience/claudecode/resolve_trace_test.go` | `TestResolveTrace_SourceChanged_OmitsExcerpt` | Parity test (NEW; proves the advisory branch is removed) |
| `internal/experience/codex/resolve_trace_test.go` | `TestResolveTrace_SourceChanged_OmitsExcerpt` | Parity test (passes today; pins the invariant) |
| `internal/experience/claudecode/resolve_trace_test.go` | `TestResolveTrace_SourceChanged_OmitsAdvisoryField` | Asserts `result.Advisory == false` after the removal |
| `internal/experience/cli/drift_test.go` | `TestDriftHandler_GoldenEnvelope` | Byte-equal JSON envelope |
| `…/drift_test.go` | `TestDriftHandler_NoPIIInOutput` | Zero matches for `/home/`, `/Users/`, `C:\Users\` |
| `…/drift_test.go` | `TestDriftHandler_FilterBySource` | `--source=claudecode` filters to one entry in `sources` |
| `…/drift_test.go` | `TestDriftHandler_TargetPathIsBasename` | `target_path == filepath.Base(target_path)` |
| `cmd/royo-learn/drift_test.go` | `TestExperienceDrift_GoldenEnvelope` | CLI golden-fixture test |
| `cmd/royo-learn/drift_test.go` | `TestExperienceDrift_FilterFlags` | `--all-sources` vs `--source=` |
| `cmd/royo-learn/drift_test.go` | `TestExperienceDrift_RejectsBothFlags` | Exit 2 + usage |
| `internal/mcp/experience_test.go` | `TestExperienceDriftStatus_Schema` | Tool registered, profile=admin, no inputs |
| `internal/mcp/experience_test.go` | `TestExperienceDriftStatus_EnvelopeParity` | MCP envelope byte-equals CLI envelope (modulo sort) |
| `internal/mcp/experience_test.go` | `TestExperienceDriftStatus_ZeroPII` | PII markers absent |

### Integration tests

| Test file | Test function | Purpose |
|-----------|---------------|---------|
| `internal/publish/drift/integration_test.go` | `TestDrift_AllFourOutcomes` | Real filesystem (`testutil.TempDir`); one case per outcome |
| `…/integration_test.go` | `TestDrift_HashMismatchRoundTrip` | File mutated → `drifted` row written |
| `…/integration_test.go` | `TestDrift_PermissionDenied` | POSIX `chmod 0o000` → `target_unreadable` (skipped on Windows) |
| `tests/release/goreleaser_snapshot_test.go` | `TestGoReleaserSnapshot_ProducesSBOM` | `goreleaser release --snapshot --clean`; asserts `*.spdx.json` exists in `dist/` |
| `tests/release/goreleaser_snapshot_test.go` | `TestReleaseRunbook_HasFiveSections` | Static grep on `RELEASE.md` for the five headers |
| `tests/release/goreleaser_snapshot_test.go` | `TestChangelog_Backfilled` | Static grep — `[Unreleased]` lacks Hito 8/9/10/11 PR titles |
| `tests/release/goreleaser_snapshot_test.go` | `TestChangelog_V100NoTagYet` | `v1.0.0` section contains literal `no tag yet` and references `RELEASE.md` |

### E2E tests

| Test | Purpose |
|------|---------|
| `cmd/royo-learn/e2e_test.go` — new subtest `TestExperienceDrift_AllSources` | Build CLI, run `experience drift` against a fixture, assert envelope shape and PII absence |
| `…/TestExperienceDrift_SourceFilter` | `--source=codex` against a fixture, assert envelope filtered |
| `…/TestExperienceDrift_LegacyFormStillWorks` | When collapse is OFF, `experience codex scan` still works (Hito 11 pattern preserved) |

### Static-review / contract tests

- Pre-commit grep rule (CI step): `grep -nE
  'os\.WriteFile|os\.Create|ioutil\.WriteFile'
  internal/publish/drift/` returns zero matches.
- Pre-commit grep rule: `grep -n "Advisory: true"
  internal/experience/claudecode/resolve_trace.go` returns zero
  matches (locks the parity invariant).
- Pre-commit grep rule: `grep -n "status = 'published'"
  internal/publish/drift/jobs.go` returns at least one match in the
  `runPublicationDriftCheck` function body (locks Decision D1).

## Migration Plan

### Forward (009_publication_drift.sql)

Forward-only, idempotent. The migration runner
(`internal/storage/migrate.go:106-140`) applies 009 once per DB; the
`CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` clauses
keep the SQL-level idempotency, and the runner's SHA-256 checksum
keeps the apply-once invariant.

### Manual rollback recipe (operator use only — runner is forward-only)

```sql
DROP INDEX IF EXISTS idx_drift_publication;
DROP INDEX IF EXISTS idx_drift_run_id;
DROP INDEX IF EXISTS idx_drift_status_checked;
DROP TABLE IF EXISTS publication_drift_state;
DELETE FROM schema_migrations WHERE version = 9;
```

This recipe is documented in `tasks.md` Phase 1 and exercised by a
doc-test (the SQL is checked into the repo so it can be reviewed, not
auto-executed).

### Idempotency story

- The migration runner embeds `migrations/*.sql` and applies them in
  version order. The runner computes a SHA-256 checksum and refuses
  to re-apply if the stored checksum differs.
- `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` keep
  the forward apply idempotent at the SQL level (the runner is the
  primary guard).

### Risk Mitigations

1. **Migration 009 lacks a runner-driven rollback (Medium)**:
   identical pattern to Hito 11's 008 migration; the manual rollback
   recipe is checked into `tasks.md` and the
   `TestMigrate_009_Forward` / `TestMigrate_009_Idempotent` /
   `TestMigrate_009_ChecksumStable` tests guard the apply path.

2. **Drift job mistakenly checks `status = 'in_progress'` (Medium,
   R-Spec R1)**: Decision D1 puts the gate in the `JobFunc` body,
   not the SQL WHERE. `TestPublicationDriftCheck_SkipsInProgress`
   proves the gate by inserting both rows and asserting
   `publication_drift_state` row count grows by exactly one. The
   pre-commit grep rule `grep -n "status = 'published'"
   internal/publish/drift/jobs.go` locks the literal in the source.

3. **`Checker.Check` accidentally writes to the target (Low, R-Spec
   R2)**: the `contract_test.go` snapshot test asserts `Mode`,
   `ModTime`, and `Size` are unchanged. The pre-commit grep fails if
   any of `os.WriteFile`, `os.Create`, `ioutil.WriteFile` is added
   under `internal/publish/drift/`.

4. **Windows Defender / macOS Gatekeeper quarantine the drift test
   artifacts (Low, R-Spec R3)**: all new tests use
   `testutil.TempDir(t)` + `t.Cleanup` (the Hito 5/10 pattern that
   already passes the Windows/Linux/macOS CI matrix). The
   `integration_test.go` documents the cleanup recipe in a header
   comment.

5. **GoReleaser v2.17.0 rejects `sboms.formats: ['spdx-json']`
   (Medium, R-Spec R4)**: format is supported since GoReleaser
   v1.18; v2.17.0 supports `spdx-json` natively. If the snapshot
   build fails, the gap is declared in `RELEASE.md` §"Known
   Limitations" and `docs/11` line 124 SBOM row is updated to
   `"planned"`. The `tests/release/goreleaser_snapshot_test.go`
   test asserts the alternative format actually emitted.

6. **Adapter parity test depends on Hito 10 fixtures (Low, R-Spec
   R5)**: the three new parity tests reuse the same `testdata/`
   fixtures shipped in Hito 10 (the fixtures are content-stable per
   `hito10-codex-review-fixes.md`); no new fixture creation in
   Hito 12.

7. **Unified CLI/MCP leaks PII via `target_path` (Low, R-Spec
   R8)**: Decision D5 puts the `filepath.Base` redaction in the
   shared `driftHandler`. `TestDriftCLI_NoPIIInOutput` greps for
   `/home/`, `/Users/`, `C:\Users\` and asserts zero matches.

## Threat Matrix

N/A for the routing / shell / subprocess / VCS / executable-file
classification / process-integration axis — Hito 12 adds no new
commands, no shell exec, no subprocess, no VCS automation, no
executable classification, and no new process integration beyond the
SBOM emission step (which runs inside the existing
`goreleaser release --snapshot --clean` invocation).

Applicable boundaries:

| Boundary | Applicable | Expected safe behavior | Expected failure behavior | Planned RED tests |
|----------|------------|------------------------|---------------------------|-------------------|
| `filesystem-read` | Yes | `Checker.Check` reads the target once via `os.Open` + streaming SHA-256; no write/chmod/chown/chtimes/rename/remove | `Checker.Check` modifies `Mode`/`ModTime`/`Size`; an `os.WriteFile` or `os.Chtimes` is added to `internal/publish/drift/` | `TestChecker_IsReadOnly`, `TestChecker_NoWriteAPIsImported`, `TestChecker_OKOnHashMatch`, `TestChecker_TargetMissingReturnsErrTargetMissing` |
| `sql-migration` | Yes | 009 applies on a fresh DB; re-applies cleanly (idempotent); runner's SHA-256 checksum stable; CHECK constraint rejects unknown statuses; FK to `publications(id)` rejects unknown IDs | 009 fails on a fresh DB (DDL bug); CHECK constraint accepts `status = 'corrupted'`; FK rejects valid publication | `TestMigrate_009_Forward`, `TestMigrate_009_Idempotent`, `TestMigrate_009_ChecksumStable`, `TestRecordDrift_RejectsUnknownStatus`, `TestRecordDrift_RoundTrip` |
| `audit-event-emission` | Yes | Drift job uses Hito 11 `RunOne`; emits exactly four `job_*` events per run; payload contains only the allow-listed keys; no `target_path` body in any audit payload | Drift job bypasses `RunOne` and emits events directly; payload leaks `/home/alice` or `excerpt` | `TestRunOne_EmitsFourEvents`, `TestRunOne_PayloadAllowList`, `TestRunOne_PerAdapterPathNoDirectAuditCall`, `TestPublicationDriftCheck_AuditPayloadHasNoTargetPath` |
| `cli-flag-validation` | Yes | `experience drift --all-sources` runs and prints the envelope; `--source=claudecode` filters; `--all-sources --source=` returns exit 2; `--source=does_not_exist` returns exit 2; `target_path` is basename | `--source` silently picks a default; `--all-sources` + `--source=` prints envelope anyway; `target_path` leaks full path | `TestExperienceDrift_GoldenEnvelope`, `TestExperienceDrift_FilterFlags`, `TestExperienceDrift_RejectsBothFlags`, `TestDriftHandler_TargetPathIsBasename`, `TestDriftHandler_NoPIIInOutput` |
| `mcp-tool-schema` | Yes | `experience_drift_status` is registered, `RequiredProfile == "admin"`, envelope matches CLI byte-for-byte (modulo sort) | Tool missing from registry; non-admin profile can invoke; envelope diverges from CLI | `TestExperienceDriftStatus_Schema`, `TestExperienceDriftStatus_EnvelopeParity`, `TestExperienceDriftStatus_ZeroPII` |
| `adapter-parity` | Yes (Hito 10 SEVERE trace-leak invariant) | All three adapters return `Excerpt == ""` and `Advisory == false` on `trace_source_changed` and `trace_source_unavailable` | `claudecode` re-introduces the advisory excerpt branch; one of the three adapters emits a non-empty excerpt on drift | `TestResolveTrace_SourceChanged_OmitsExcerpt` (three packages), `TestResolveTrace_SourceChanged_OmitsAdvisoryField`, `grep -n "Advisory: true" internal/experience/claudecode/resolve_trace.go` returns zero matches |
| `release-artifact` | Yes | `goreleaser release --snapshot --clean` produces `*.spdx.json` alongside archives; `RELEASE.md` covers the 5 sections; `CHANGELOG.md` backfilled | SBOM format unsupported and not declared; `RELEASE.md` missing sections; `[Unreleased]` still has Hito 8/9/10/11 entries | `TestGoReleaserSnapshot_ProducesSBOM`, `TestReleaseRunbook_HasFiveSections`, `TestChangelog_Backfilled`, `TestChangelog_V100NoTagYet` |

## Open Questions

- [ ] **`JobIntentDrift` documentation cross-link**: Hito 11's
      `internal/experience/semantic/types.go:23-24` comment says
      "JobIntentIngest is the only value used by the Hito 11 static
      jobs. The Promote/Rebuild/Cleanup constants are reserved for
      future hits and are exported so that later work can extend the
      registry without re-opening this package." After Hito 12 adds
      `JobIntentDrift`, the comment should be updated to reflect the
      new constant. **Decision: update the comment in the same PR
      (additive, scoped to the types.go change).**

- [ ] **`target_path` and `source_hash` extraction from JSON blobs
      (Decision D2)**: `publications.targets_json` and
      `verification_json` are untyped `TEXT NOT NULL` columns. The
      drift job assumes the JSON shape documented in
      `internal/domain/publication.go` (the `Publication.Targets`
      and `Publication.Verification` struct definitions). **Decision
      (orchestrator to confirm before apply): if the JSON shape
      does not match, the drift job records `target_unreadable`
      with the underlying `json.Unmarshal` error and skips the row
      (fail soft per row). This is documented in Decision D2's
      rationale.**

- [ ] **CHANGELOG backfill source — `git log` vs GitHub API**: the
      proposal uses `git log --oneline` to extract PR titles. On a
      fork or detached clone, `git log` may not see all merge SHAs.
      **Decision (orchestrator to confirm): the backfill script
      reads `git log` from the local clone; if the script is run in
      CI it uses `actions/checkout`'s fetch-depth: 0 setting so
      all history is available. The script does NOT call the GitHub
      API (avoids auth-token coupling).**

- [ ] **`release-extras` spec coverage of
      `tests/release/goreleaser_snapshot_test.go`**: the spec's
      coverage gate says "≥ 90 % statement coverage on
      `internal/publish/drift/`". The release-extras tests live in
      `tests/release/` (not under `internal/publish/drift/`) and
      are not subject to that gate. **Decision: the release-extras
      tests are not under the coverage gate; they are
      documentation-style static tests and one snapshot test. The
      coverage gate still applies to `internal/publish/drift/`
      proper.**

- [ ] **`publication_drift_state` retention policy**: the table
      grows by one row per `(publication_id, target_path)` per
      check run (upserts on conflict, so no unbounded growth per
      pair). However, over many runs and many rollbacks, the
      `checked_at` index may grow large. **Decision (defer to
      follow-up): no retention policy in Hito 12; the Hito 13
      "metrics / retention" change will introduce a TTL. Document
      the deferral in `docs/26-IMPLEMENTATION-ROADMAP.md` §5.**

- [ ] **`publications.status` enum values**: the proposal assumes
      `status = 'published'` is one of the documented values. The
      actual values written by the codebase are documented in
      `internal/domain/publication.go` (the `PublicationStatus`
      type). **Decision (orchestrator to confirm): if `'published'`
      is NOT one of the documented values, the gate skips all
      rows and the drift detector records zero outcomes. This is a
      safe default (fail-soft: no false `drifted` outcomes) but
      means the operator gets no signal. Surface this in the
      rollout runbook.**

- [ ] **`sboms:` block placement in `.goreleaser.yml`**: GoReleaser
      v2 places `sboms:` at the **top level** (a sibling of
      `archives:`, `brews:`, etc.), not under `archives:` as the
      proposal's example shows. **Decision: place `sboms:` at the
      top level; the `RELEASE.md` §"Known Limitations" notes the
      placement if GoReleaser rejects the nested form.**