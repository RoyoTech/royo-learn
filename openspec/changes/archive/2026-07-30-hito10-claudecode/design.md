# Design: Hito 10 — Claude Code adapter (PR #11)

> Companion to `proposal.md` and `tasks.md` for the same change folder.
> All four files (`proposal.md`, `tasks.md`, `design.md`,
> `specs/experience-adapters/spec.md`) must land together in the PR.

## 1. Adapter package layout

Mirrors `internal/experience/opencode/` exactly so the operator and
reviewers don't have to context-switch between the two adapters:

```text
internal/experience/claudecode/
├── doc.go                  # package-level scope, boundaries, version
├── adapter.go              # ExperienceAdapter interface mirror, Adapter struct, Name()
├── contract.go             # types + SchemaTag = "claude-code/jsonl-v1"
├── discover.go             # Discover(ctx, projectRoot) ([]SourceInstance, error)
├── health.go               # Health(ctx, SourceInstance) HealthResult
├── scan.go                 # Scan(ctx, ScanRequest) (ScanResult, error)
├── resolve_trace.go        # ResolveTrace(ctx, locator, bounds) TraceResult
├── *_test.go               # one test file per source file
└── testdata/
    └── fixtures/
        └── session-<uuid>.jsonl
```

The `ExperienceAdapter` interface is **duplicated** (not imported) on
purpose: each package owns its contract test table and is free to
evolve independently. The duplication is pinned by
`TestAdapter_ImplementsContract` in each package — drift would fail
RED at `go test`.

## 2. Discover

- `projectRoot` is **required**. The adapter does not auto-resolve the
  user's project root from the encoded Claude Code slug
  (`~/.claude/projects/<encoded>/<session>.jsonl`). The slug is opaque
  and lossy; decoding it would let a malicious upstream trick the
  adapter into accepting a session file from outside the trust
  boundary. The CLI/MCP caller passes `--project-root` explicitly.
- Canonicalize `projectRoot` via `internal/project.Canonicalize`. Map
  errors to `experience_locator_outside_root` per `docs/22` §6.
- Walk with `filepath.WalkDir`:
  - Skip symlinked directories (matches opencode).
  - Skip `IsProtectedPath` entries (`docs/24` T4 + project.ProtectedPaths).
  - Depth cap = 8 (matches opencode `maxDiscoveryDepth`).
  - Files matching `<session-uuid>.jsonl` under the encoded dir are
    surfaced; everything else is skipped.
- `SourceInstance` carries:
  ```go
  Source      domain.ExperienceSource  // always SourceClaudeCode
  ProjectRoot string                    // canonical
  JSONLPath   string                    // canonical, the .jsonl file
  Schema      string                    // SchemaTag
  Discovered  time.Time
  ```
- Output is sorted by `JSONLPath` for deterministic iteration.

## 3. Health

- `os.Stat` the JSONL file. Missing → `degraded` +
  `experience_source_not_found`. Directory → `degraded` +
  `experience_source_not_found`.
- Open the file read-only, read the first 1 KiB, run `json.Decoder` to
  confirm at least one object with `type`, `uuid`, `sessionId` set.
  Fail → `degraded` + `experience_source_schema_unsupported`.
- Context cancellation → `error` + `timeout`.
- The adapter **never** writes to the JSONL file. Tests assert this
  by stat-ing the file before/after and comparing mtime + size.

## 4. Scan

- Open the JSONL file with `bufio.Scanner` configured with a 1 MiB
  initial buffer (`bufio.Scanner` default is 64 KiB; Claude Code
  assistant turns with `tool_use` blocks can exceed that).
- For each line:
  - Skip empty lines and lines that fail `json.Unmarshal`; increment
    `SkippedMalformed`.
  - Parse the object; required fields: `type`, `uuid`, `sessionId`,
    `timestamp` (RFC3339 string or `int64` millis).
  - Filter by `type`:
    - `user`: extract `message.content` → `UserText`.
    - `assistant`: extract `message.content` → `AssistantText`.
      Also extract `tool_use` blocks → `SafeToolCall[]`.
      Drop `thinking` blocks entirely (no LLM reasoning per
      AGENTS.md regla 9; ADR-0001).
    - `system`: skip (not user/assistant); increment `SkippedSystem`.
    - any other type: skip + increment `SkippedUnknown`.
  - Completeness gate (matches opencode's `complete=0` rule):
    - turn is `Complete=true` iff `stop_reason` is non-empty **or**
      a subsequent `user` turn exists in the same session.
    - incomplete turns are skipped + `SkippedIncomplete++`.
- Sort envelopes by `(Session.ExternalID, Turn.ExternalID)` for
  deterministic output.
- NextCursor: `{ "last_session_id": string, "last_turn_uuid": string }`
  for the last emitted envelope. Empty when no envelopes.
- `ScanResult.SkippedMalformed`, `SkippedIncomplete`, `SkippedSystem`,
  `SkippedUnknown` are all surfaced to the caller so the CLI can
  report the gaps (opencode lesson 1 — `docs/26` §9).

## 5. ResolveTrace

- Locator invariants: `Kind == "jsonl"`, `Path` non-empty,
  `TurnID` non-empty.
- `defaultTraceMaxBytes = 1024` matches opencode.
- Compute current SHA-256 of the JSONL file; if `locator.SourceHash`
  is set and differs → return `TraceResult{SourceChanged: true,
  Code: "trace_source_changed"}` without an excerpt.
- Stream-scan the JSONL for the line whose `uuid == locator.TurnID`.
  Concatenate the `text` blocks of `message.content` (drop
  `thinking` blocks), run `evidence.Redact`, truncate to
  `bounds.MaxBytes`, append `TraceExcerptSuffix = "..."` when
  truncated.
- Missing file → `trace_source_unavailable`. Missing turn → same.
- Context cancellation → `timeout`.

## 6. Job registry

- One static registration: `experience_ingest:claude_code`.
- Mirrors the existing `experience_ingest:opencode` entry shape:
  ```go
  jobs.JobRegistryEntry{
      JobName:            "experience_ingest:claude_code",
      Description:        "Incremental ingest of Claude Code JSONL transcripts.",
      DefaultIntervalSec: 300,
      DefaultMaxRetries:  3,
      Enabled:            false, // Hito 3 flips to true
  }
  ```
- Registered via `jobs.Service.Register` at startup. Idempotent on
  `job_name`; re-registering is a no-op (`ON CONFLICT(job_name) DO
  UPDATE`).
- The registration is **CLI/MCP-driven** to keep the ingest pipeline
  consistent with the OpenCode precedent and so Hito 3 can flip
  `Enabled = true` without code changes in this package.

## 7. CLI orchestrator

- `cmd/royo-learn/experience.go` gains:
  - `runExperienceClaudeCode(args, stdout, stderr)` — subcommand
    dispatcher.
  - `runExperienceClaudeCodeScan(args, stdout, stderr)` — orchestrates
    Discover → Health → Scan → `experience.Service.IngestEnvelope`.
  - `buildClaudeCodeFixtureInstance(projectRoot, fixturePath)` — same
    symlink + `IsInsideRoot` guards as opencode's
    `buildFixtureInstance`.
- Flags: `--project-root` (required), `--fixture <path>` (optional),
  `--once` (default true; reserved for Hito 3).
- Output JSON envelope matches the OpenCode scan shape with
  `source: "claude_code"` and the additive `skipped_malformed`,
  `skipped_system`, `skipped_unknown` counters.
- Errors land on stderr through `logging.WriteError` with the
  project's stable error envelope; exit codes via
  `domain.ErrorCode.ExitCode()`.

## 8. Fixtures

- One JSONL file per session, matching the upstream Claude Code
  layout (`{type, uuid, parentUuid, timestamp, sessionId, cwd,
  version, message: {role, content: [...]}, tool_use: {...}}`).
- Anonymized real Claude Code JSONL — secrets removed via
  `internal/evidence.Redact`. AGENTS.md regla 14 forbids raw secrets
  in fixtures.
- Located at `internal/experience/claudecode/testdata/fixtures/`.
  The fixture set is excluded from the authored line count but
  included in the snapshot identity per
  `skills/_shared/sdd-phase-common.md` §E.

## 9. Schema tag

- Constant `SchemaTag = "claude-code/jsonl-v1"`.
- Bumping it is a breaking change; the bump path is documented in
  `docs/22-ADAPTER-CONTRACT.md` §7 and surfaces as
  `experience_source_schema_unsupported` in `Health()`.

## 10. Migration plan

- **No DB migration.** Existing migrations 001–007 cover the schema;
  this adapter reuses the same `ExperienceEnvelope`, `ExperienceSession`,
  `ExperienceTurn`, and `IngestionCursor` tables.
- No new columns, no new tables, no new indexes. The job registry
  entry is a single INSERT into `job_registry`; the engine's
  idempotent upsert makes this safe to run repeatedly.

## 11. Coverage targets

- `internal/experience/claudecode/` ≥ 85% (matches the
  `internal/adapters >= 85%` row in `docs/25` §4).
- `cmd/royo-learn/experience_claudecode_test.go` adds CLI-layer
  coverage so the orchestrator is not untested (opencode lesson 3 —
  `docs/26` §9).

## 12. Cross-build

- `GOOS=windows go build ./...` — Windows path separators handled by
  `filepath` throughout; `bufio.Scanner` line cap covers CRLF
  boundaries.
- `GOOS=linux go build ./...` — case-sensitive FS, no changes.
- `GOOS=darwin go build ./...` — `internal/project.normalizePath`
  already handles case-insensitive FS via
  `runtime.GOOS == "darwin"`.

## 13. Open questions deferred to `sdd-spec`

- Exact wording of the `### Requirement:` blocks in
  `specs/experience-adapters/spec.md` is owned by the spec phase;
  the proposal hands off the capability delta here.
- Whether to add a `Locator.Offset` field for JSONL byte-precise
  addressing (currently the locator carries only `TurnID`) — the
  spec phase can decide based on whether `TraceBounds.Offset` ever
  receives a non-zero value in practice.