# Design: Hito 10 — Codex adapter (PR #12)

> Companion to `proposal.md` and `tasks.md` for the same change
> folder. All four files (`proposal.md`, `tasks.md`, `design.md`,
> `specs/experience-adapters/spec.md`) must land together in the PR.

## 1. Adapter package layout

Mirrors `internal/experience/opencode/` and `internal/experience/claudecode/`
exactly so the operator and reviewers don't have to context-switch
between adapters:

```text
internal/experience/codex/
├── doc.go                  # package-level scope, boundaries, version
├── adapter.go              # ExperienceAdapter interface mirror, Adapter struct, Name()
├── contract.go             # types + SchemaTag = "codex/rollout-v1"
├── discover.go             # Discover(ctx, projectRoot) ([]SourceInstance, error)
├── health.go               # Health(ctx, SourceInstance) HealthResult
├── scan.go                 # Scan(ctx, ScanRequest) (ScanResult, error)
├── resolve_trace.go        # ResolveTrace(ctx, locator, bounds) TraceResult
├── *_test.go               # one test file per source file
└── testdata/
    └── fixtures/
        └── rollout-<id>.jsonl
```

The `ExperienceAdapter` interface is **duplicated** (not imported)
on purpose: each package owns its contract test table and is free
to evolve independently. The duplication is pinned by
`TestAdapter_ImplementsContract` in each package — drift would fail
RED at `go test`.

## 2. Discover

- `projectRoot` is **required**. The adapter does not auto-resolve
  the user's home directory or trust any upstream `session_index`.
- Canonicalize `projectRoot` via `internal/project.Canonicalize`.
  Map errors to `experience_locator_outside_root` per `docs/22` §6.
- Walk both:
  - `~/.codex/sessions/YYYY/MM/DD/rollout-<id>.jsonl`
  - `~/.codex/archived_sessions/rollout-<id>.jsonl`
- Filename filter: only `rollout-*.jsonl` files are surfaced.
- Index files (`session_index.jsonl` and any other non-rollout
  file) are explicitly skipped — the adapter walks the actual
  rollout files instead of trusting an upstream index that can
  list rotated or deleted sessions.
- `IsProtectedPath` and `IsInsideRoot` per `docs/24` T4.
- Depth cap = 8 (matches opencode + claudecode).
- `SourceInstance` carries:
  ```go
  Source      domain.ExperienceSource  // always SourceCodex
  ProjectRoot string                    // canonical
  RolloutPath string                    // canonical, the .jsonl file
  Schema      string                    // SchemaTag
  Discovered  time.Time
  ```
- Output is sorted by `RolloutPath` for deterministic iteration.

## 3. Health

- `os.Stat` the rollout file. Missing → `degraded` +
  `experience_source_not_found`. Directory → `degraded` +
  `experience_source_not_found`.
- Open the file read-only, read the first 1 KiB, run
  `json.Decoder` to confirm at least one object with
  `type == "session_meta"` and a non-empty `payload.codex_session_id`.
  Fail → `degraded` + `experience_source_schema_unsupported`.
- Context cancellation → `error` + `timeout`.
- The adapter **never** writes to the rollout file. Tests assert
  this by stat-ing the file before/after and comparing mtime + size.

## 4. Scan

- Open the rollout file with `bufio.Scanner` configured with a
  1 MiB initial buffer (assistant turns with `response_item.message`
  blocks can exceed the 64 KiB default).
- Track the current `session_meta` anchor as lines stream past. If
  the file is missing `session_meta`, emit
  `experience_source_schema_unsupported` (degraded) — Codex always
  emits one per file.
- Top-level `type` discriminator:
  - `session_meta`: anchor `external_session_id = payload.codex_session_id`,
    `cwd = payload.cwd`, `cli_version = payload.cli_version`. No
    envelope.
  - `turn_context`: anchor the per-turn `cwd` / `model` (used for
    `Actor.Model` on the envelope). No envelope.
  - `event_msg`: parse `payload.type`:
    - `agent_message` → emit/extend `AssistantText`.
    - `user_message` → emit/extend `UserText`.
    - `token_count` → metadata only.
    - `task_started` / `task_complete` → mark turn completion /
      start boundary.
    - `web_search_end` → metadata only.
  - `response_item`: parse `payload.type`:
    - `message` → text into the current turn.
    - `function_call` → `SafeToolCall{Name: payload.name,
      Arguments: payload.arguments, Outcome: payload.call_id}`.
    - `function_call_output` → metadata only (output already
      captured upstream; never persisted verbatim per
      `docs/22` §3 SafeToolCall rule).
    - `reasoning` → **drop** (no LLM reasoning per AGENTS.md
      regla 9; ADR-0001). Drop is unconditional.
    - `web_search_call` → metadata only.
- Completeness gate (matches opencode + claudecode):
  - turn is `Complete=true` iff a `task_complete` follows in the
    same session **or** a subsequent `user_message` appears.
  - incomplete → `SkippedIncomplete++`.
- Build `ExperienceEnvelope` with
  `Locator.Kind = "rollout"`,
  `Locator.SourceHash = sha256(file bytes)`,
  `Actor.Kind = "agent"`, `Actor.Name = "codex"`,
  `Actor.Model = turn_context.model`.
- Sort envelopes by `(Session.ExternalID, Turn.Sequence)`.
- NextCursor: `{ "last_session_id": string, "last_turn_uuid": string }`
  for the last emitted envelope. Empty when no envelopes.
- `ScanResult.SkippedMalformed`, `SkippedIncomplete` are surfaced
  to the caller so the CLI can report the gaps (opencode lesson 1
  — `docs/26` §9).

## 5. ResolveTrace

- Locator invariants: `Kind == "rollout"`, `Path` non-empty,
  `TurnID` non-empty.
- `defaultTraceMaxBytes = 1024` matches opencode + claudecode.
- Compute current SHA-256 of the rollout file; if
  `locator.SourceHash` is set and differs → return
  `TraceResult{SourceChanged: true,
  Code: "trace_source_changed"}` without an excerpt.
- Stream-scan the rollout for the line whose external turn ID
  matches `locator.TurnID`. Concatenate the `event_msg.agent_message`
  and `response_item.message` text (drop `reasoning` and
  `function_call_output` verbatim), run `evidence.Redact`,
  truncate to `bounds.MaxBytes`, append `TraceExcerptSuffix = "..."`
  when truncated.
- Missing file → `trace_source_unavailable`. Missing turn → same.
- Context cancellation → `timeout`.

## 6. Job registry

- One static registration: `experience_ingest:codex`.
- Mirrors the existing `experience_ingest:opencode` and
  `experience_ingest:claude_code` (PR #11) entry shape:
  ```go
  jobs.JobRegistryEntry{
      JobName:            "experience_ingest:codex",
      Description:        "Incremental ingest of Codex rollout JSONL transcripts.",
      DefaultIntervalSec: 300,
      DefaultMaxRetries:  3,
      Enabled:            false, // Hito 3 flips to true
  }
  ```
- Registered via `jobs.Service.Register` at startup. Idempotent on
  `job_name`; re-registering is a no-op
  (`ON CONFLICT(job_name) DO UPDATE`).
- The registration is **CLI/MCP-driven** to keep the ingest
  pipeline consistent with the opencode + claudecode precedent and
  so Hito 3 can flip `Enabled = true` without code changes in this
  package.

## 7. CLI orchestrator

- `cmd/royo-learn/experience.go` gains:
  - `runExperienceCodex(args, stdout, stderr)` — subcommand
    dispatcher.
  - `runExperienceCodexScan(args, stdout, stderr)` — orchestrates
    Discover → Health → Scan → `experience.Service.IngestEnvelope`.
  - `buildCodexFixtureInstance(projectRoot, fixturePath)` — same
    symlink + `IsInsideRoot` guards as opencode's
    `buildFixtureInstance`. Filename must end in `.jsonl`.
- Flags: `--project-root` (required), `--fixture <path>` (optional),
  `--once` (default true; reserved for Hito 3).
- Output JSON envelope matches the OpenCode scan shape with
  `source: "codex"` and the additive `skipped_malformed`,
  `skipped_incomplete` counters.
- Errors land on stderr through `logging.WriteError` with the
  project's stable error envelope; exit codes via
  `domain.ErrorCode.ExitCode()`.

## 8. Fixtures

- One JSONL file per rollout, matching the upstream Codex layout
  (top-level `session_meta` / `turn_context` / `event_msg` /
  `response_item`).
- Anonymized real Codex rollout JSONL — secrets removed via
  `internal/evidence.Redact`. AGENTS.md regla 14 forbids raw
  secrets in fixtures.
- Located at `internal/experience/codex/testdata/fixtures/`. The
  fixture set is excluded from the authored line count but
  included in the snapshot identity per
  `skills/_shared/sdd-phase-common.md` §E.

## 9. Schema tag

- Constant `SchemaTag = "codex/rollout-v1"`.
- Bumping it is a breaking change; the bump path is documented
  in `docs/22-ADAPTER-CONTRACT.md` §7 and surfaces as
  `experience_source_schema_unsupported` in `Health()`.

## 10. Migration plan

- **No DB migration.** Existing migrations 001–007 cover the
  schema; this adapter reuses the same `ExperienceEnvelope`,
  `ExperienceSession`, `ExperienceTurn`, and `IngestionCursor`
  tables.
- No new columns, no new tables, no new indexes. The job
  registry entry is a single INSERT into `job_registry`; the
  engine's idempotent upsert makes this safe to run repeatedly.

## 11. Coverage targets

- `internal/experience/codex/` ≥ 85% (matches the
  `internal/adapters >= 85%` row in `docs/25` §4).
- `cmd/royo-learn/experience_codex_test.go` adds CLI-layer
  coverage so the orchestrator is not untested (opencode lesson 3
  — `docs/26` §9).

## 12. Cross-build

- `GOOS=windows go build ./...` — Windows path separators handled
  by `filepath` throughout; `bufio.Scanner` line cap covers CRLF
  boundaries.
- `GOOS=linux go build ./...` — case-sensitive FS, no changes.
- `GOOS=darwin go build ./...` — `internal/project.normalizePath`
  already handles case-insensitive FS via
  `runtime.GOOS == "darwin"`.

## 13. Open questions deferred to `sdd-spec`

- Exact wording of the `### Requirement:` blocks in
  `specs/experience-adapters/spec.md` is owned by the spec
  phase; the proposal hands off the capability delta here.
- Whether to add a `Locator.Offset` field for JSONL byte-precise
  addressing (currently the locator carries only `TurnID`) — the
  spec phase can decide based on whether `TraceBounds.Offset` ever
  receives a non-zero value in practice.