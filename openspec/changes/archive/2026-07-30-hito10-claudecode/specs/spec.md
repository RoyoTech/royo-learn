# Spec delta — experience-adapters (Claude Code, PR #11)

> This file extends the existing adapter contract that the Hito 2
> OpenCode implementation freezes. It is **additive only** — no
> existing requirement is renamed, weakened, or removed.
>
> Source contracts being extended:
>
> - `docs/22-ADAPTER-CONTRACT.md` §1 (interface), §3 (envelope),
>   §6 (errors), §7 (schema tag), §9 (CLI rules).
> - `docs/21-EXPERIENCE-DOMAIN.md` §1 (`SourceClaudeCode` enum),
>   §3 (`TranscriptLocator.Kind = "jsonl"`).

## MODIFIED Requirements

### Requirement: ExperienceAdapter is implemented by every platform adapter

`docs/22` §1 is extended to cover a second concrete instance:
`internal/experience/claudecode`. The four-method contract
(`Name`, `Discover`, `Health`, `Scan`, `ResolveTrace`) is mirrored
exactly; no signature changes; no relaxation of the
"context-respecting, resource-closing, never-mutating-source" rules.

#### Scenario: Claude Code adapter satisfies the four-method contract

- **WHEN** `*claudecode.Adapter` is asserted to
  `claudecode.ExperienceAdapter` at compile time
- **THEN** the assertion succeeds and the package compiles
- **AND** `TestAdapter_ImplementsContract` passes

#### Scenario: Claude Code adapter name is the canonical enum value

- **WHEN** `claudecode.NewAdapter().Name()` is called
- **THEN** the result equals `domain.SourceClaudeCode` (`"claude_code"`)
- **AND** no other `ExperienceSource` value is returned

### Requirement: TranscriptLocator accepts `jsonl` as a valid kind

`docs/21` §3 is extended to include `jsonl` as a valid `Kind`. The
existing `sqlite`, `rollout`, `file`, and `api` values remain valid;
no existing value is renamed.

#### Scenario: Claude Code scan builds a locator with `Kind: "jsonl"`

- **WHEN** the adapter builds an `ExperienceEnvelope`
- **THEN** `Session.Locator.Kind == "jsonl"`
- **AND** `Session.Locator.Path` is the canonical absolute path of the
  Claude Code JSONL file
- **AND** `Session.Locator.SourceHash` is the SHA-256 of the file at
  scan time

#### Scenario: Claude Code trace resolver rejects non-`jsonl` locators

- **WHEN** `ResolveTrace` receives a locator with `Kind != "jsonl"`
- **THEN** the result has `Code == "experience_locator_invalid"`
- **AND** no source I/O occurs

## ADDED Requirements

### Requirement: Schema tag for Claude Code is `claude-code/jsonl-v1`

`docs/22` §7 enumerates the per-adapter schema tags. The Claude Code
adapter declares `SchemaTag = "claude-code/jsonl-v1"`. Bumping it is
a breaking change.

#### Scenario: SchemaTag is pinned by a test

- **WHEN** the test runs
- **THEN** `SchemaTag == "claude-code/jsonl-v1"`
- **AND** any future bump breaks the test until the spec is updated

#### Scenario: Schema mismatch surfaces `experience_source_schema_unsupported`

- **WHEN** `Health` opens a JSONL file whose first 1 KiB does not
  contain an object with `type`, `uuid`, and `sessionId`
- **THEN** the result has `Status == "degraded"` and
  `Code == "experience_source_schema_unsupported"`
- **AND** the upstream file is never written to (mtime + size
  invariants preserved)

### Requirement: Claude Code adapter discovers session JSONL files reachable from the caller-supplied project root

`docs/22` §1 (Discover) is implemented by `claudecode.Adapter` for
JSONL layout `~/.claude/projects/<encoded>/<session-uuid>.jsonl`.

#### Scenario: Discover returns sorted SourceInstances

- **WHEN** the caller passes a project root that contains N valid
  JSONL session files
- **THEN** `Discover` returns N `SourceInstance` values
- **AND** the result is sorted by `JSONLPath` ascending

#### Scenario: Discover rejects paths outside the project root

- **WHEN** a session JSONL exists outside the canonical project root
  (including via symlink escape)
- **THEN** it is not surfaced
- **AND** no filesystem I/O occurs beyond the walk

#### Scenario: Discover requires `projectRoot`

- **WHEN** `projectRoot` is empty or whitespace
- **THEN** `Discover` returns a typed validation error
  (`experience_locator_invalid`)
- **AND** no filesystem walk occurs

### Requirement: Claude Code Scan produces neutral ExperienceEnvelopes and a stable NextCursor

`docs/22` §3 (ExperienceEnvelope) is honored; `docs/22` §6
(`experience_turn_incomplete`) drives the skip rules.

#### Scenario: Incomplete turns are skipped and counted

- **WHEN** a turn has no `stop_reason` and no subsequent user turn
  in the same session
- **THEN** it is not emitted as an envelope
- **AND** `ScanResult.SkippedIncomplete` is incremented

#### Scenario: Malformed lines are skipped and counted

- **WHEN** a JSONL line fails to parse
- **THEN** it is not emitted as an envelope
- **AND** `ScanResult.SkippedMalformed` is incremented

#### Scenario: System and unknown turns are skipped and counted

- **WHEN** a JSONL line carries `type: "system"` or any other
  non-user/assistant value
- **THEN** it is not emitted as an envelope
- **AND** the corresponding counter (`SkippedSystem` /
  `SkippedUnknown`) is incremented

#### Scenario: Thinking blocks are dropped at envelope build time

- **WHEN** an assistant message carries `message.content` blocks of
  type `thinking`
- **THEN** those blocks do not appear in `AssistantText`, `ToolCalls`,
  or any envelope field
- **AND** no redaction counter is incremented (the drop is
  unconditional)

#### Scenario: Cursor is opaque and stable

- **WHEN** the scan finishes with at least one emitted envelope
- **THEN** `ScanResult.NextCursor` is a `map[string]any` carrying
  `last_session_id` (string) and `last_turn_uuid` (string)
- **AND** passing the cursor back into a second scan against the
  same fixture produces zero new envelopes (idempotency)

### Requirement: Claude Code ResolveTrace returns bounded, redacted excerpts

`docs/22` §6 (errors) and `docs/24` §4 (Trace security) apply.

#### Scenario: Source mutated returns `trace_source_changed`

- **WHEN** `locator.SourceHash` is set and the current SHA-256 of the
  JSONL file differs
- **THEN** the result has `SourceChanged: true` and
  `Code == "trace_source_changed"`
- **AND** no excerpt is returned

#### Scenario: Missing source or turn returns `trace_source_unavailable`

- **WHEN** the JSONL file does not exist or no line carries
  `uuid == locator.TurnID`
- **THEN** the result has `Code == "trace_source_unavailable"`
- **AND** no excerpt is returned

#### Scenario: Happy-path excerpt is bounded and redacted

- **WHEN** the turn exists and `bounds.MaxBytes == 1024`
- **THEN** the excerpt length is at most 1024 bytes (plus the
  `TraceExcerptSuffix = "..."` if truncated)
- **AND** secrets are replaced by `evidence.Redact` and `Redacted`
  is `true`

### Requirement: CLI subcommand `experience claude-code scan` is additive

`docs/04` and `docs/22` §9 apply.

#### Scenario: Subcommand exists in the dispatcher

- **WHEN** the user runs `royo-learn experience claude-code scan --project-root <path>`
- **THEN** the dispatcher routes to the Claude Code orchestrator
- **AND** missing `--project-root` returns `invalid_argument`

#### Scenario: Output JSON envelope shape is stable

- **WHEN** the scan completes
- **THEN** stdout carries a single JSON object with fields
  `source`, `status`, `instances[]`, `ingested_turns`,
  `duplicates`, `skipped_incomplete`, `skipped_malformed`,
  `skipped_system`, `skipped_unknown`, `envelopes_total`
- **AND** no field is removed from this list in v1

#### Scenario: `--fixture` validates inside `project_root`

- **WHEN** the caller passes a fixture path that is a symlink or
  outside the canonical project root
- **THEN** the orchestrator returns a typed error
  (`symlink_escape` or `path_outside_root`)
- **AND** no scan occurs

### Requirement: Job registry entry `experience_ingest:claude_code` is registered

`docs/21` §8 (JobState) and `docs/25` §2 (Hito 8 row) apply.

#### Scenario: Registration is idempotent

- **WHEN** the CLI/MCP startup calls
  `jobs.Service.Register` with the entry twice in a row
- **THEN** the second call is a no-op
- **AND** `job_registry` carries exactly one row for this name

#### Scenario: Job is registered but disabled in v1

- **WHEN** the entry is registered
- **THEN** `Enabled == false`
- **AND** `DefaultIntervalSec == 300` and `DefaultMaxRetries == 3`
- **AND** `RunDue` skips this job until Hito 3 flips `Enabled`

### Requirement: Coverage target for the new package is ≥ 85%

`docs/25` §4 (`internal/adapters >= 85%`) applies.

#### Scenario: `go test -cover` meets the threshold

- **WHEN** `go test -cover ./internal/experience/claudecode/...`
  runs in CI
- **THEN** the coverage report shows ≥ 85% statement coverage
- **AND** the CI gate fails below the threshold