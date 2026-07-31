# Delta for experience-adapters (Hito 11, PR #14)

> This delta MODIFIES the existing `experience-adapters` capability that
> the Hito 2 OpenCode implementation freezes and that Hito 10 (Codex,
> PR #12) extended. It is **not additive**: two existing requirements
> change shape (per-adapter `JobRegistryEntry` constructor signature and
> the static `JobRegistryEntry` upsert payload).
>
> Source contracts being extended:
>
> - `docs/22-ADAPTER-CONTRACT.md` §1 (interface), §3 (envelope),
>   §6 (errors), §7 (schema tag), §9 (CLI rules).
> - `docs/21-EXPERIENCE-DOMAIN.md` §1 (`ExperienceSource` enum),
>   §8 (`JobState`).
> - `openspec/changes/hito10-codex/specs/experience-adapters/spec.md`
>   (the prior delta this delta MODIFIES).

## MODIFIED Requirements

### Requirement: JobRegistryEntry registration is migrated to `Job() *semantic.Job`

The system SHALL replace the per-adapter static `JobRegistryEntry`
constructor with a `Job() *semantic.Job` accessor in each of
`internal/experience/{opencode,claudecode,codex}/jobs.go`. The
accessor MUST return a `*semantic.Job` whose `Source` equals the
adapter's `Name()` and whose `JobFunc` body wraps the existing
`ExperienceAdapter` calls (`Discover → Health → Scan →
service.IngestEnvelope`). The compile-time contract test
`TestAdapter_ImplementsContract` MUST keep passing without change
because the adapter interface is unchanged (`docs/22` §1). The legacy
static `JobRegistryEntry` constructor signature stays as a helper used
**inside** the `JobFunc` body — it is no longer the public surface.
(Previously: each per-adapter `jobs.go` exposed a static
`JobRegistryEntry` constructor returning a `JobRegistryEntry` value;
the `JobFunc` runtime shape did not exist.)

#### Scenario: Each adapter exposes the new accessor

- GIVEN the three adapters `opencode`, `claudecode`, `codex`
- WHEN a compile-time check asserts
  `var _ func() *semantic.Job = (*opencode.Adapter)(nil).Job`
- THEN the assertion succeeds for all three adapters
- AND the return type matches `*semantic.Job` for all three.

#### Scenario: Accessor returns distinct jobs per call

- GIVEN the adapter instance `a`
- WHEN the caller invokes `a.Job()` twice in succession
- THEN the two returned pointers are distinct
- AND each call produces a fresh closure bound to the current
  `JobRegistryEntry` (stateless receiver).

#### Scenario: Compile-time contract test stays green

- GIVEN `TestAdapter_ImplementsContract` in
  `internal/experience/codex/adapter_test.go` (and the parallel
  tests for `opencode` and `claudecode`)
- WHEN `go test ./internal/experience/...` runs on linux/amd64
- THEN the test passes
- AND no signature change is required to the 5-method
  `ExperienceAdapter` interface (`docs/22` §1).

#### Scenario: Legacy static constructor remains as an internal helper

- GIVEN the previous `NewIngestJobRegistryEntry(source)` constructor
  in each per-adapter `jobs.go`
- WHEN the caller inspects the file
- THEN the constructor is renamed to `newIngestJobRegistryEntry`
  (unexported) and is invoked from inside the `JobFunc` body
- AND no external caller imports the legacy exported name.

### Requirement: JobRegistryEntry upsert populates the new taxonomy columns for the three static jobs

The system SHALL set `intent = "ingest"`, `scope = "project"`,
`risk_class = "low"` on the upsert of the three static
`JobRegistryEntry` rows (`experience_ingest:opencode`,
`experience_ingest:claude_code`, `experience_ingest:codex`) executed
by `jobs.Repository.Upsert` (the upsert path is unchanged for all
other rows). The three rows MUST remain `Enabled = false`; the
Hito 3 (`--watch`) flip is owned by PR #10 and is not in scope for
Hito 11 (`docs/26` §5). The `DefaultIntervalSec` (`300`) and
`DefaultMaxRetries` (`3`) values from Hito 10 stay unchanged.
(Previously: the three rows carried no `intent`/`scope`/`risk_class`
fields because those columns did not exist; the columns are added by
`migrations/008_job_semantics.sql` in `job-semantic-engine`
REQ-JSE-4.)

#### Scenario: Upsert populates the three new columns

- GIVEN the CLI/MCP startup calls
  `jobs.Service.Register` for
  `experience_ingest:opencode`
- WHEN the upsert completes
- THEN `job_registry.intent == "ingest"`,
  `job_registry.scope == "project"`,
  `job_registry.risk_class == "low"`
- AND `job_registry.Enabled == false`.

#### Scenario: All three static jobs share the same taxonomy values

- GIVEN the CLI/MCP startup registers the three static rows
- WHEN a query reads `job_registry` for the three names
- THEN all three rows carry
  `intent = "ingest"`, `scope = "project"`,
  `risk_class = "low"`
- AND no row carries any other value.

#### Scenario: Unknown enum value at upsert is rejected

- GIVEN a future change that registers a job with
  `intent = "scrape"` (not in `semantic.JobIntent`)
- WHEN `jobs.Repository.Upsert` is called
- THEN it returns the typed validation error documented by
  `job-semantic-engine` REQ-JSE-2
- AND no row is written.

#### Scenario: `Enabled` never flips to `true` from this path

- GIVEN the Hito 11 upsert path runs at startup
- WHEN the static review inspects the per-adapter `jobs.go` files
- AND the upsert helper in `internal/experience/jobs/repository.go`
- THEN no code path sets `Enabled = true`
- AND `RunDue` continues to skip the three static jobs in v1.

## ADDED Requirements

### Requirement: Per-adapter runtime tests cover the `Job()` accessor

The system SHALL add a unit test in each of
`internal/experience/{opencode,claudecode,codex}/jobs_test.go` that
asserts the `Job()` accessor returns a non-nil `*semantic.Job` whose
`Source` equals the adapter's `Name()`. The test is part of the
`docs/25` §4 coverage gate for `internal/experience` (≥ 90%).

#### Scenario: OpenCode `Job()` accessor test passes

- GIVEN `opencode.NewAdapter().Job()` is invoked
- WHEN the test runs
- THEN the result is non-nil
- AND `result.Source == domain.SourceOpenCode`.

#### Scenario: Claude Code `Job()` accessor test passes

- GIVEN `claudecode.NewAdapter().Job()` is invoked
- WHEN the test runs
- THEN the result is non-nil
- AND `result.Source == domain.SourceClaudeCode`.

#### Scenario: Codex `Job()` accessor test passes

- GIVEN `codex.NewAdapter().Job()` is invoked
- WHEN the test runs
- THEN the result is non-nil
- AND `result.Source == domain.SourceCodex`.

#### Scenario: Coverage gate passes

- GIVEN the per-adapter tests run with `-cover`
- WHEN the CI gate evaluates `docs/25` §4
- THEN `internal/experience` reports ≥ 90% statement coverage
- AND `internal/experience/semantic` (the new package from
  `job-semantic-engine`) reports ≥ 90% on its own.

## References

- `docs/22-ADAPTER-CONTRACT.md` §1, §7, §9
- `docs/21-EXPERIENCE-DOMAIN.md` §1, §8
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit invariant)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §4 (coverage gate)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 PR #14
- `openspec/changes/hito11-semantic/specs/job-semantic-engine/spec.md`
