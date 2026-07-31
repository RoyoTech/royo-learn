# Drift CLI / MCP Specification

## Purpose

Give the operator a single, stable surface to ask "what drifted across
all three experience adapters and the publication layer right now?" —
the unified drift CLI subcommand `experience drift` and the matching MCP
tool `experience_drift_status`. The capability covers
`cmd/royo-learn/experience.go::runExperienceDrift` with the
`--all-sources` (default) and `--source=<opencode|claudecode|codex>`
flags, the `internal/mcp/` `experience_drift_status` tool, the stable
JSON envelope shape `{ "sources": [...], "publications": [...] }`, the
PII redaction rule (`target_path` reduced to `filepath.Base(...)` in
JSON output), and the golden-fixture test that pins the envelope. The
envelope carries aggregated counters per source plus per-publication
drift rows; it never carries `excerpt`, `user_text`, `assistant_text`,
or any other `ExperienceEnvelope` field.

## Requirements

### Requirement: `experience drift` CLI subcommand exists with the two filter flags

The system SHALL add a `runExperienceDrift(args []string) error`
function in `cmd/royo-learn/experience.go` registered as the
`experience drift` subcommand. The function MUST accept two flags:
`--all-sources` (default `true`) and
`--source=<opencode|claudecode|codex>` (mutually exclusive with
`--all-sources`; passing both returns a usage error). When
`--all-sources` is set, the subcommand emits both the `sources` and
`publications` sections of the envelope. When `--source=` is set to a
single source value, the subcommand emits only the `sources` section
filtered to that source plus the `publications` section for that source.
The subcommand delegates to `jobs.Service.RunOne` against
`publication_drift_check` so the Hito 11 audit hook fires the four
`job_*` lifecycle events.

#### Scenario: `--all-sources` (default) emits both envelope sections

- GIVEN a project with three registered sources and two drifted
  publications and one `ok` publication
- WHEN `royo-learn experience drift` runs (no flags)
- THEN the JSON envelope has a populated `sources` array with three
  entries
- AND the envelope has a populated `publications` array with three
  entries (two drifted, one ok)
- AND the envelope is written to `stdout` (logs on `stderr`).

#### Scenario: `--source=claudecode` filters the envelope

- GIVEN a project with three registered sources
- WHEN `royo-learn experience drift --source=claudecode` runs
- THEN the JSON envelope `sources` array contains exactly one entry
  with `"source": "claudecode"`
- AND the `publications` array contains only the publications whose
  `source == "claudecode"`.

#### Scenario: `--all-sources` and `--source=` together return a usage error

- GIVEN the CLI dispatcher
- WHEN `royo-learn experience drift --all-sources --source=opencode`
  runs
- THEN the process exits non-zero with a usage error on `stderr`
- AND no JSON envelope is written.

### Requirement: Stable JSON envelope shape `{ "sources": [...], "publications": [...] }`

The system SHALL emit a JSON envelope with exactly two top-level keys:
`sources` (array) and `publications` (array). The `sources` array
elements MUST have the shape
`{ "source": string, "drifted": int, "ok": int, "missing": int }`.
The `publications` array elements MUST have the shape
`{ "publication_id": string, "source": string, "target_path": string,
"status": "ok"|"drifted"|"target_missing"|"target_unreadable",
"expected_hash": string, "actual_hash": string }`. The envelope MUST
NOT include `excerpt`, `user_text`, `assistant_text`, `tool_calls`,
`actor`, or any other `ExperienceEnvelope` field. The envelope is the
single source of truth for the operator-facing drift view; both the CLI
subcommand and the MCP tool emit the same struct.

#### Scenario: Three-source drift envelope is shape-stable

- GIVEN a fixture project with two drifted publications on `opencode`,
  one ok publication on `claudecode`, and one missing publication on
  `codex`
- WHEN the CLI runs
- THEN the rendered JSON parses with `encoding/json`
- AND the top-level object has exactly two keys: `sources` and
  `publications`
- AND each `sources` entry has the four documented fields
- AND each `publications` entry has the six documented fields.

#### Scenario: Golden fixture test pins the envelope shape

- GIVEN the test file
  `cmd/royo-learn/drift_test.go::TestExperienceDrift_GoldenEnvelope`
- WHEN it runs
- THEN the rendered JSON byte-equals the checked-in fixture under
  `testdata/drift_golden.json` (modulo field ordering, which the
  comparator sorts before equality)
- AND a regression that adds a new top-level key fails the test.

### Requirement: `target_path` is redacted to basename in JSON output

The system SHALL redact every `target_path` value in the JSON envelope
to `filepath.Base(target_path)`. The full path MUST NOT appear in the
envelope. The redaction is applied in the same handler that both the
CLI and the MCP tool call, so both surfaces emit the same redacted
shape. The `target_unreadable` outcome MUST still emit
`target_path = filepath.Base(...)` so the operator can identify the
target even when the file cannot be read.

#### Scenario: Target path with user name is redacted in JSON

- GIVEN a publication whose `target_path` is
  `/home/alice/.claude/sessions/foo.jsonl`
- WHEN the envelope is rendered to JSON
- THEN the only `target_path` value in the JSON is `foo.jsonl`
- AND `/home/alice` does not appear anywhere in the rendered JSON.

#### Scenario: `target_unreadable` still emits a basename

- GIVEN a publication whose `target_path` exists but is unreadable
- WHEN the envelope is rendered to JSON
- THEN `status == "target_unreadable"`
- AND `target_path` equals `filepath.Base(target_path)`
- AND `expected_hash` is the recorded hash
- AND `actual_hash` is the empty string.

### Requirement: MCP tool `experience_drift_status` exposes the same envelope

The system SHALL register an MCP tool named `experience_drift_status`
in `internal/mcp/experience.go`. The tool MUST require the
`admin` profile (matching the existing `experience_*` tool profile
gate) and MUST emit the exact same envelope shape as the CLI
subcommand. The tool handler MUST call the same shared handler as the
CLI, so any drift in the CLI envelope propagates to the MCP surface
within the same release. The tool schema test
(`internal/mcp/experience_test.go::TestExperienceDriftStatus_Schema`)
MUST assert the tool is registered, takes no parameters, and returns
the `{ "sources": [...], "publications": [...] }` envelope.

#### Scenario: MCP tool is registered with admin profile

- GIVEN the MCP tool registry on startup
- WHEN `internal/mcp/experience.go::RegisterTools` runs
- THEN `experience_drift_status` is in the registry
- AND the tool's `RequiredProfile == "admin"`.

#### Scenario: MCP envelope matches CLI envelope

- GIVEN the CLI handler and the MCP tool handler
- WHEN the two are invoked with the same fixture state
- THEN both emit byte-equal JSON envelopes (modulo sort order).

### Requirement: Zero PII in rendered JSON output

The system SHALL guarantee that the rendered JSON envelope contains no
substring drawn from the PII marker set
`{ "/home/", "/Users/", "C:\\Users\\" }`. The test
`TestDriftCLI_NoPIIInOutput` greps the rendered JSON for each marker
and asserts zero matches. The test MUST run against a fixture whose
underlying target paths include `/home/alice/...`,
`/Users/bob/...`, and `C:\\Users\\carol\\...` so a regression that
forgets the `filepath.Base` redaction fails the test on every
platform.

#### Scenario: PII marker test passes

- GIVEN a fixture project whose publication `target_path` values are
  `/home/alice/x.jsonl`, `/Users/bob/y.jsonl`, and
  `C:\Users\carol\z.jsonl`
- WHEN the CLI runs
- THEN the rendered JSON contains zero occurrences of `/home/`,
  `/Users/`, and `C:\Users\`
- AND only the basenames (`x.jsonl`, `y.jsonl`, `z.jsonl`) appear in
  the envelope.

#### Scenario: PII marker test fails on redaction regression

- GIVEN a future regression that emits the full `target_path` instead
  of the basename
- WHEN `TestDriftCLI_NoPIIInOutput` runs
- THEN the test fails on every platform
- AND the failure message names the offending marker substring.

### Requirement: Cross-platform CI parity for the drift CLI

The system SHALL run the drift CLI tests on Windows, Linux, and macOS
in CI. The fixture paths (`/home/alice/...`, `/Users/bob/...`,
`C:\Users\carol\...`) MUST all be present in the fixture so the
redaction test exercises every documented platform path prefix. The
`testutil.TempDir(t)` + `t.Cleanup` pattern (Hito 5/10) MUST be used to
keep Windows Defender / macOS Gatekeeper from quarantining the temp
artifacts.

#### Scenario: Drift CLI tests pass on linux/amd64

- GIVEN the CI workflow `ci.yml`
- WHEN the `go test -race ./cmd/royo-learn/...` job runs on
  linux/amd64
- THEN `TestExperienceDrift_GoldenEnvelope`,
  `TestDriftCLI_NoPIIInOutput`, and
  `TestExperienceDrift_FilterFlags` all pass.

#### Scenario: Drift CLI tests pass on windows-latest and macos-latest

- GIVEN the CI matrix runs
- WHEN the Windows and macOS jobs run the same three tests
- THEN all three pass on both platforms
- AND no quarantine cleanup recipe is required.

## Out of scope

- Returning excerpts or transcript text in the envelope. The drift
  view is metadata-only.
- Cross-publication chain verification (parent/child consistency).
- Drift on `audit_events` table content or on log file rotation. The
  detector only covers the publication layer; cross-cutting drift lives
  in a separate capability.
- Per-source CLI dispatchers for drift (`experience drift opencode`,
  `experience drift claudecode`, etc.). The unified
  `--all-sources` / `--source=` filter covers all three.

## References

- `docs/22-ADAPTER-CONTRACT.md` §6 (typed errors), §9 (CLI rules)
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit invariant),
  §3 (path / PII threat model)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 (Hito 12 acceptance
  rows)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 (Hito 12 row)
- `openspec/changes/hito12-drift-release/specs/publication-drift-check/spec.md`
  (upstream capability whose job emits the data this CLI/MCP surface
  consumes)
