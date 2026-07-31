# Tasks: Hito 10 — Claude Code adapter (PR #11)

> Branch: `feat/hito10-claudecode` from `origin/main`.
> Method: TDD strict (RED → GREEN → REFACTOR), one slice per commit.
> Each slice ends with `go test -race ./...` green and `gofmt`/`go vet` clean.
> Mirrors the Hito 2 OpenCode slice breakdown (`HANDOFF-HITO2-OPENCODE-ONCE.md` §4)
> so the operator has the same reviewable unit shape per PR.

## Conventions

- Conventional commits only. No AI attribution. No `Co-Authored-By`.
- Authored line budget per slice ≤ 80 LOC; total PR budget 350–450 LOC
  (fixtures excluded from authored count, included in snapshot identity).
- Each slice must reference `docs/22-ADAPTER-CONTRACT.md` and
  `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` in the commit body.
- Slice commits do **not** move the cursor or change `--watch` semantics;
  those belong to Hito 3.

## Slice 10.0 — Scaffold

- [ ] Create `internal/experience/claudecode/` package skeleton:
  `doc.go`, `adapter.go` (interface + `Adapter` struct mirroring opencode
  layout), `contract.go` (types + `SchemaTag = "claude-code/jsonl-v1"`).
- [ ] Write the contract-test table (`opencode_test.go` analog):
  `TestSchemaTag`, `TestAdapter_ImplementsContract`,
  `TestAdapter_Name`, `TestAdapter_RespectsContextCancellation`,
  `TestNewAdapter_Defaults`. Confirm RED before GREEN.
- [ ] Run `go build ./...` + `go vet ./...` — must stay green.

Acceptance: contract tests pass; package builds; no production logic yet.

## Slice 10.1 — Discover

- [ ] Implement `Discover(ctx, projectRoot)`:
  - Canonicalize `projectRoot` via `internal/project.Canonicalize`.
  - Walk `~/.claude/projects/<encoded>/<session-uuid>.jsonl` files
    reachable from `projectRoot`. The caller **must** supply
    `projectRoot`; the adapter never decodes the encoded slug.
  - Reject via `IsProtectedPath` and `IsInsideRoot` per
    `docs/24-EXPERIENCE-THREAT-MODEL.md` T4.
  - Depth cap = 8 (matches opencode `maxDiscoveryDepth`).
  - Sort output by `JSONLPath` for determinism.
- [ ] Tests:
  - symlink to outside root → typed error (`experience_locator_outside_root`).
  - session file outside project root → not surfaced.
  - empty root → empty result, no error.
  - cancelled context → `context.Canceled` returned (not wrapped).
  - determinism: two scans with the same layout return the same order.

Acceptance: `docs/25` Hito 2 security row satisfied; `go test -race
./internal/experience/claudecode/...` green.

## Slice 10.2 — Health

- [x] Implement `Health(ctx, instance)`:
  - `os.Stat` the JSONL path; `os.IsNotExist` → `degraded` +
    `experience_source_not_found`.
  - Open the file, read first 1 KiB, run `json.Decoder` to confirm at
    least one object with non-empty `type` / `uuid` / `sessionId`.
  - Status mapping mirrors opencode: `ok` / `degraded` / `error`.
  - `ctx.Err()` honored on entry.
- [x] Tests:
  - missing file → `degraded` + `experience_source_not_found`.
  - non-JSONL file → `degraded` + `experience_source_schema_unsupported`.
  - valid first-1-KiB header → `ok`.
  - cancelled context → `error` + `timeout`.

Acceptance: `docs/22` §6 mapping stable; `docs/25` Hito 2 health row
satisfied.

## Slice 10.3 — Scan

- [ ] Implement `Scan(ctx, req)`:
  - Open JSONL line by line via `bufio.Scanner` with a 1 MiB line cap.
  - Skip lines that fail to parse, increment
    `ScanResult.SkippedMalformed` (counter is **additive**, not in
    opencode because JSONL never has malformed lines the same way).
  - For each parsed object:
    - Required fields: `type` (`user`|`assistant`|`system`),
      `uuid`, `sessionId`, `timestamp` (RFC3339 string or millis int).
    - User/assistant: extract `message.content` (string or content
      blocks). `thinking` blocks are dropped (no LLM reasoning in v1
      per AGENTS.md regla 9). `text` blocks become `UserText` /
      `AssistantText`. `tool_use` blocks become `SafeToolCall`
      (name, input, id) — never execute.
    - System turns: skip (not user/assistant content); increment
      `SkippedSystem`.
    - `complete`-style flag: when `stop_reason` is set OR a sibling
      `user` turn appears later, mark `Complete=true`; otherwise
      skip and increment `SkippedIncomplete` (mirrors opencode lesson).
  - Build `ExperienceEnvelope` with
    `Locator.Kind = "jsonl"`, `Locator.SourceHash = sha256(file bytes)`,
    `Actor.Kind = "agent"`, `Actor.Name = "claude_code"`.
  - Sort envelopes by `(Session.ExternalID, Turn.ExternalID)`.
- [ ] Tests (using `internal/experience/claudecode/testdata/fixtures/`):
  - anonymized real JSONL with user+assistant+thinking+tool_use.
  - malformed line → skipped + counter.
  - incomplete turn (no `stop_reason`, no sibling) → skipped + counter.
  - fixture must NOT contain secrets (CI gates via
    `internal/evidence.Redact` round-trip).

Acceptance: `docs/22` §3 envelope shape preserved; `docs/25` Hito 2
fixture + reinicio rows satisfied; redaction tests in
`internal/evidence` still pass.

## Slice 10.4 — Idempotency

- [ ] Cursor shape:
  `{ "last_session_id": string, "last_turn_uuid": string }` persisted
  by the core `service.IngestEnvelope` after each commit
  (INV-16 — cursor never ahead of commit, `docs/21` §7).
- [ ] Decoder accepts `string` / `int` / `int64` / `float64` for
  forward-compat with sub-agent round-trips (opencode lesson).
- [ ] Second scan with the same fixture against the same DB:
  - `(source, external_session_id, external_turn_id)` uniqueness in
    `internal/experience.Service` produces `Idempotent = true` on every
    envelope; `IngestedTurns == 0`.
- [ ] Tests:
  - re-scan after ingest → zero new turns, zero errors.
  - cursor persistence: cursor survives a process restart.

Acceptance: `docs/21` §2/§4 uniqueness invariant; `docs/25` Hito 2
reinicio row satisfied.

## Slice 10.5 — ResolveTrace

- [ ] Implement `ResolveTrace(ctx, locator, bounds)`:
  - Validate `locator.Kind == "jsonl"`, `locator.Path`,
    `locator.TurnID` required.
  - `defaultTraceMaxBytes = 1024`, override via `bounds.MaxBytes`.
  - Hash file; if `locator.SourceHash` set and differs → return
    `trace_source_changed`.
  - Stream-scan JSONL, find the line with `uuid == locator.TurnID`,
    run `evidence.Redact` on `message.content` text, truncate to
    `bounds.MaxBytes`, append `TraceExcerptSuffix = "..."` when
    trimmed.
  - File missing or turn missing → `trace_source_unavailable`.
  - Cancelled context → `timeout`.
- [ ] Tests:
  - happy path: bounded excerpt, `Redacted=true` when secret present.
  - source mutated: returns `trace_source_changed`, no excerpt.
  - missing turn: returns `trace_source_unavailable`, no excerpt.
  - oversized content: truncated, suffix present.

Acceptance: `docs/24` §4 + T12; `docs/25` Hito 4 trace rules
satisfied for read-side.

## Slice 10.6 — CLI

- [x] Add `claude-code` subcommand dispatcher in
  `cmd/royo-learn/experience.go`:
  - `runExperienceClaudeCode` mirrors `runExperienceOpencode`.
  - `runExperienceClaudeCodeScan(args, stdout, stderr)`:
    flags `--project-root` (required), `--fixture <path>` (optional),
    `--once` (default true; present for forward compat with Hito 3).
- [ ] Output JSON shape (mirrors opencode):
  ```json
  {
    "source": "claude_code",
    "status": "ok|degraded|error",
    "instances": [
      {
        "jsonl_path": "...",
        "status": "...",
        "code": "...",
        "message": "...",
        "ingested_turns": 0,
        "duplicates": 0,
        "skipped_incomplete": 0,
        "skipped_malformed": 0,
        "envelopes_total": 0
      }
    ],
    "ingested_turns": 0,
    "duplicates": 0,
    "skipped_incomplete": 0,
    "skipped_malformed": 0,
    "envelopes_total": 0
  }
  ```
- [ ] `--fixture` validates path via the same
  `buildFixtureInstance`-style guard as opencode: no symlinks,
  inside `project_root`, JSONL extension only.
- [x] Tests (`cmd/royo-learn/experience_claudecode_test.go`,
  new file):
  - happy path: `--fixture` against a real anonymized JSONL produces
    expected `IngestedTurns > 0`, `Duplicates == 0` on first run.
  - second run: `IngestedTurns == 0`, `Duplicates > 0`.
  - missing `--project-root` → `invalid_argument` exit code.
  - `--fixture` outside project root → typed error.

Acceptance: `docs/04-CLI-SPEC.md` additive update lands in the same
PR; `docs/05-MCP-SPEC.md` note about deferred MCP routing added;
JSON envelope stable (no field renames).

## Slice 10.7 — Job registry + acceptance

- [ ] Register `experience_ingest:claude_code` in
  `internal/experience/jobs/registry` via `Service.Register`:
  - `JobName = "experience_ingest:claude_code"`.
  - `Description = "Incremental ingest of Claude Code JSONL transcripts."`.
  - `DefaultIntervalSec = 300` (5 min, matches opencode precedent).
  - `DefaultMaxRetries = 3`.
  - `Enabled = false` (Hito 3 flips it; this PR only registers so the
    job name is single-source).
- [ ] Idempotency: re-Registering the same name is a no-op (already
  covered by `TestJobs_Register_Idempotent`).
- [ ] Acceptance run:
  - `go test -race ./...` green.
  - `go test -cover ./internal/experience/claudecode/` ≥ 85%
    (`docs/25` §4 row `internal/adapters >= 85%`).
  - Cross-build: `GOOS=windows go build`, `GOOS=linux go build`,
    `GOOS=darwin go build` all green.
  - Migration test: no DB migration introduced; migrations 001–007
    cover the schema (`docs/26` §6 — "no migrations" gate).
- [ ] Lessons file: append a row to `docs/IMPLEMENTATION-NOTES.md`
    with any new pattern discovered during the slice (e.g. thinking-block
    redaction, JSONL line-cap choice).

Acceptance: `docs/25` §2 Hito 8 jobs row satisfied (registry wired);
`docs/25` §4 coverage target met; cross-build green; no migration.

## Done = all slices green + PR opened

- [ ] All eight slices merged into `feat/hito10-claudecode` with
  conventional commits.
- [ ] PR body references this `proposal.md`, the four spec
  delta files, and `docs/25` rows it closes.
- [ ] CI gates (`docs/26` §6): `gofmt`, `go vet`, `go test ./...`,
  `go test -race -p 1 ./...`, cross-build, migration tests, e2e
  fixtures, security tests, coverage gates — all green without
  `continue-on-error`.
- [ ] No `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` committed; no AI
  attribution in any commit.