# Tasks: Hito 10 — Codex adapter (PR #12)

> Branch: `feat/hito10-codex` from `origin/main` after PR #11 merges.
> Method: TDD strict (RED → GREEN → REFACTOR), one slice per commit.
> Each slice ends with `go test -race ./...` green and `gofmt`/`go vet` clean.
> Mirrors the Hito 2 OpenCode slice breakdown (`HANDOFF-HITO2-OPENCODE-ONCE.md` §4)
> and the Hito 10 Claude Code slice breakdown (PR #11) so the operator
> has the same reviewable unit shape per PR.

## Conventions

- Conventional commits only. No AI attribution. No `Co-Authored-By`.
- Authored line budget per slice ≤ 80 LOC; total PR budget 350–450
  LOC (fixtures excluded from authored count, included in snapshot
  identity).
- Each slice must reference `docs/22-ADAPTER-CONTRACT.md` and
  `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` in the commit body.
- Slice commits do **not** move the cursor or change `--watch`
  semantics; those belong to Hito 3.
- Do **not** branch from local `main` after PR #11 has been merged;
  always rebase from `origin/main` (lessons §4, `docs/26` §9).

## Slice 10.0 — Scaffold

- [ ] Create `internal/experience/codex/` package skeleton:
  `doc.go`, `adapter.go` (interface + `Adapter` struct mirroring
  opencode + claudecode layout), `contract.go` (types +
  `SchemaTag = "codex/rollout-v1"`).
- [ ] Write the contract-test table (`opencode_test.go` analog):
  `TestSchemaTag`, `TestAdapter_ImplementsContract`,
  `TestAdapter_Name`, `TestAdapter_RespectsContextCancellation`,
  `TestNewAdapter_Defaults`. Confirm RED before GREEN.
- [ ] Run `go build ./...` + `go vet ./...` — must stay green.

Acceptance: contract tests pass; package builds; no production logic
yet.

## Slice 10.1 — Discover

- [ ] Implement `Discover(ctx, projectRoot)`:
  - Canonicalize `projectRoot` via `internal/project.Canonicalize`.
  - Walk both
    `~/.codex/sessions/YYYY/MM/DD/rollout-<id>.jsonl` and
    `~/.codex/archived_sessions/rollout-<id>.jsonl` reachable from
    `projectRoot`.
  - Skip `session_index.jsonl` (and any other index file); the
    adapter does **not** trust upstream indexes.
  - Reject via `IsProtectedPath` and `IsInsideRoot` per
    `docs/24-EXPERIENCE-THREAT-MODEL.md` T4.
  - Depth cap = 8 (matches opencode `maxDiscoveryDepth`).
  - Sort output by `RolloutPath` for determinism.
- [ ] Tests:
  - session in `sessions/YYYY/MM/DD/` is surfaced.
  - session in `archived_sessions/` is surfaced.
  - `session_index.jsonl` is **not** surfaced.
  - symlink to outside root → typed error.
  - rollout file outside project root → not surfaced.
  - empty root → empty result, no error.
  - cancelled context → `context.Canceled` returned (not wrapped).
  - determinism: two scans with the same layout return the same
    order.

Acceptance: `docs/25` Hito 2 security row satisfied; `go test -race
./internal/experience/codex/...` green.

## Slice 10.2 — Health

- [ ] Implement `Health(ctx, instance)`:
  - `os.Stat` the rollout path; `os.IsNotExist` → `degraded` +
    `experience_source_not_found`.
  - Open the file, read the first 1 KiB, run `json.Decoder` to
    confirm at least one object with `type == "session_meta"` and
    the expected `payload.codex_session_id` / `payload.cwd` /
    `payload.cli_version` fields.
  - Status mapping mirrors opencode: `ok` / `degraded` / `error`.
  - `ctx.Err()` honored on entry.
- [ ] Tests:
  - missing file → `degraded` + `experience_source_not_found`.
  - non-JSONL file → `degraded` +
    `experience_source_schema_unsupported`.
  - valid first-1-KiB header with `session_meta` → `ok`.
  - cancelled context → `error` + `timeout`.

Acceptance: `docs/22` §6 mapping stable; `docs/25` Hito 2 health
row satisfied.

## Slice 10.3 — Scan

- [ ] Implement `Scan(ctx, req)`:
  - Open the rollout JSONL with `bufio.Scanner` configured with a
    1 MiB initial buffer.
  - For each line:
    - Skip empty / malformed lines; increment `SkippedMalformed`.
    - Top-level `type` discriminator:
      - `session_meta`: anchor the session's `external_session_id`,
        `cwd`, `cli_version`. Emit **no** envelope (meta is
        per-session context, not per-turn).
      - `turn_context`: anchor the per-turn `cwd` / `model`.
      - `event_msg`: parse `payload.type`:
        - `agent_message` → `AssistantText`.
        - `user_message` → `UserText`.
        - `token_count` → metadata only, not envelope text.
        - `task_started` / `task_complete` / `web_search_end` →
          metadata only.
      - `response_item`: parse `payload.type`:
        - `message` → text into the current turn.
        - `function_call` → `SafeToolCall{Name, Arguments, Outcome}`.
        - `function_call_output` → metadata (output already
          captured upstream); never persist the output verbatim.
        - `reasoning` → **drop** (no LLM reasoning in v1 per
          AGENTS.md regla 9; ADR-0001).
        - `web_search_call` → metadata only.
  - Turn completion: a turn is `Complete=true` when a `task_complete`
    follows in the same session, **or** when a subsequent
    `user_message` appears. Incomplete → `SkippedIncomplete++`.
  - Build `ExperienceEnvelope` with
    `Locator.Kind = "rollout"`,
    `Locator.SourceHash = sha256(file bytes)`,
    `Actor.Kind = "agent"`, `Actor.Name = "codex"`.
  - Sort envelopes by `(Session.ExternalID, Turn.Sequence)`.
- [ ] Tests (using `internal/experience/codex/testdata/fixtures/`):
  - anonymized real rollout JSONL with `session_meta`,
    `turn_context`, `event_msg.agent_message`,
    `response_item.message`, `response_item.function_call`,
    `response_item.reasoning`.
  - `reasoning` line → dropped, not in envelope.
  - malformed line → skipped + counter.
  - incomplete turn → skipped + counter.
  - fixture must NOT contain secrets (CI gates via
    `internal/evidence.Redact` round-trip).

Acceptance: `docs/22` §3 envelope shape preserved; `docs/25` Hito 2
fixture + reinicio rows satisfied; redaction tests in
`internal/evidence` still pass.

## Slice 10.4 — Idempotency

- [ ] Cursor shape:
  `{ "last_session_id": string, "last_turn_uuid": string }`
  persisted by the core `service.IngestEnvelope` after each commit
  (INV-16 — cursor never ahead of commit, `docs/21` §7).
- [ ] Decoder accepts a `string` for the
  turn-UUID field (forward-compat with sub-agent round-trips;
  opencode lesson).
- [ ] Second scan with the same fixture against the same DB:
  - `(source, external_session_id, external_turn_id)` uniqueness in
    `internal/experience.Service` produces `Idempotent = true` on
    every envelope; `IngestedTurns == 0`.
- [ ] Tests:
  - re-scan after ingest → zero new turns, zero errors.
  - cursor persistence: cursor survives a process restart.
  - re-scan after the file grew (one new line appended) → exactly
    one new envelope, no duplicates for the existing turns.

Acceptance: `docs/21` §2/§4 uniqueness invariant; `docs/25` Hito 2
reinicio row satisfied.

## Slice 10.5 — ResolveTrace

- [ ] Implement `ResolveTrace(ctx, locator, bounds)`:
  - Validate `locator.Kind == "rollout"`, `locator.Path`,
    `locator.TurnID` required.
  - `defaultTraceMaxBytes = 1024`, override via `bounds.MaxBytes`.
  - Hash file; if `locator.SourceHash` set and differs → return
    `trace_source_changed`.
  - Stream-scan JSONL, find the line whose external turn ID
    matches `locator.TurnID`, concatenate the
    `event_msg.agent_message` / `response_item.message` text (drop
    `reasoning` and `function_call_output`), run `evidence.Redact`,
    truncate to `bounds.MaxBytes`, append `TraceExcerptSuffix = "..."`
    when trimmed.
  - File missing or turn missing → `trace_source_unavailable`.
  - Cancelled context → `timeout`.
- [ ] Tests:
  - happy path: bounded excerpt, `Redacted=true` when secret
    present.
  - source mutated: returns `trace_source_changed`, no excerpt.
  - missing turn: returns `trace_source_unavailable`, no excerpt.
  - oversized content: truncated, suffix present.

Acceptance: `docs/24` §4 + T12; `docs/25` Hito 4 trace rules
satisfied for read-side.

## Slice 10.6 — CLI

- [ ] Add `codex` subcommand dispatcher in
  `cmd/royo-learn/experience.go`:
  - `runExperienceCodex(args, stdout, stderr)` mirrors
    `runExperienceOpencode`.
  - `runExperienceCodexScan(args, stdout, stderr)`:
    flags `--project-root` (required), `--fixture <path>` (optional),
    `--once` (default true; present for forward compat with Hito 3).
- [ ] Output JSON shape (mirrors opencode):
  ```json
  {
    "source": "codex",
    "status": "ok|degraded|error",
    "instances": [
      {
        "rollout_path": "...",
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
  inside `project_root`, `.jsonl` extension only.
- [ ] Tests (`cmd/royo-learn/experience_codex_test.go`, new file):
  - happy path: `--fixture` against a real anonymized rollout
    JSONL produces expected `IngestedTurns > 0`,
    `Duplicates == 0` on first run.
  - second run: `IngestedTurns == 0`, `Duplicates > 0`.
  - missing `--project-root` → `invalid_argument` exit code.
  - `--fixture` outside project root → typed error.
  - `session_index.jsonl` fixture → no envelopes (filtered at
    discover).

Acceptance: `docs/04-CLI-SPEC.md` additive update lands in the
same PR; `docs/05-MCP-SPEC.md` note about deferred MCP routing
added; JSON envelope stable (no field renames).

## Slice 10.7 — Job registry + acceptance

- [ ] Register `experience_ingest:codex` in
  `internal/experience/jobs/registry` via `Service.Register`:
  - `JobName = "experience_ingest:codex"`.
  - `Description = "Incremental ingest of Codex rollout JSONL transcripts."`.
  - `DefaultIntervalSec = 300` (5 min, matches opencode + claude
    code precedent).
  - `DefaultMaxRetries = 3`.
  - `Enabled = false` (Hito 3 flips it; this PR only registers so
    the job name is single-source).
- [ ] Idempotency: re-Registering the same name is a no-op (already
  covered by `TestJobs_Register_Idempotent`).
- [ ] Acceptance run:
  - `go test -race ./...` green.
  - `go test -cover ./internal/experience/codex/` ≥ 85%
    (`docs/25` §4 row `internal/adapters >= 85%`).
  - Cross-build: `GOOS=windows go build`, `GOOS=linux go build`,
    `GOOS=darwin go build` all green.
  - Migration test: no DB migration introduced; migrations 001–007
    cover the schema (`docs/26` §6 — "no migrations" gate).
- [ ] Lessons file: append a row to `docs/IMPLEMENTATION-NOTES.md`
    with any new pattern discovered during the slice (e.g.
    `session_meta` anchor handling, `reasoning` drop policy).

Acceptance: `docs/25` §2 Hito 8 jobs row satisfied (registry wired);
`docs/25` §4 coverage target met; cross-build green; no migration.

## Done = all slices green + PR opened

- [ ] All eight slices merged into `feat/hito10-codex` with
  conventional commits.
- [ ] PR body references this `proposal.md`, the four spec
  delta files, and `docs/25` rows it closes.
- [ ] CI gates (`docs/26` §6): `gofmt`, `go vet`, `go test ./...`,
  `go test -race -p 1 ./...`, cross-build, migration tests, e2e
  fixtures, security tests, coverage gates — all green without
  `continue-on-error`.
- [ ] No `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` committed; no AI
  attribution in any commit.