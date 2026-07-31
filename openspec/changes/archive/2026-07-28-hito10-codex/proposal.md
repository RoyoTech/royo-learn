# Proposal: Hito 10 — Codex adapter (PR #12)

## Why

OpenCode was the pilot that froze `docs/22-ADAPTER-CONTRACT.md` (Hito 0)
and the Hito 2 read-only path. Claude Code is the second source (PR #11).
Codex is the third. Without this PR the local-only experience layer is
incomplete for any contributor who actually drives sessions in OpenAI's
Codex CLI/Desktop/IDE instead of OpenCode, so the operator cannot audit
what the assistant learned from Codex sessions that did not run in
OpenCode.

This is the second of two PRs in Ola 2 that close the "experience
discovery across harnesses" gap (`docs/26-IMPLEMENTATION-ROADMAP.md` §4
PR #11 + PR #12). The two PRs are independent on purpose: each
adapter needs its own review budget, its own fixtures, and its own
Ola 2 trigger when `--watch` lands.

This PR is the **read-side** half of the Codex integration. The
write-side (setup install --agent) already exists in
`internal/setup/codex.go` and is **not** touched here. The trigger
`v0.3.0` is the last PR before the Ola 2 tag; cutting it depends on
this PR merging on `feat/hito10-codex` after PR #11.

## What changes

| Area | Type | Description |
|---|---|---|
| `internal/experience/codex/` | new | Adapter implementing `ExperienceAdapter` (5 methods) over Codex rollout JSONL transcripts |
| `internal/experience/jobs/registry` | additive | Register `experience_ingest:codex` static entry so Hito 3 (`--watch`) is single-source |
| `cmd/royo-learn/experience.go` | additive | New `codex` subcommand dispatcher and `experience codex scan` orchestrator reusing the OpenCode envelope shape |
| `docs/04-CLI-SPEC.md` | additive | Document `experience codex scan --once --project-root [--fixture]` |
| `docs/05-MCP-SPEC.md` | additive | Note that the new subcommand is CLI-only in v1; MCP routing is deferred to Ola 3 |

`docs/21`, `docs/22`, `docs/26` are frozen contracts from Hito 0 and
are referenced, not edited. Setup layer (`internal/setup/codex.go`)
is not touched — that is the write-side, owned by an earlier PR.

## Impact

| Spec / file | Impact | What |
|---|---|---|
| `docs/22-ADAPTER-CONTRACT.md` §1, §7 | referenced | Add a third instance of the four-method contract; schema tag `codex/rollout-v1` per §7 |
| `docs/21-EXPERIENCE-DOMAIN.md` §1, §3 | referenced | Reuse `SourceCodex` enum (already declared) and confirm `rollout` is a valid `TranscriptLocator.Kind` |
| `docs/24-EXPERIENCE-THREAT-MODEL.md` | referenced | Same threat model: T1, T4, T7, T8, T12 apply identically |
| `docs/04-CLI-SPEC.md` | additive only | Add the `codex scan` subcommand table |
| `docs/05-MCP-SPEC.md` | additive only | One-line note that `codex` stays CLI in Ola 2 |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §4 | referenced | PR #12 row references this proposal as the deliverable |

**No breaking changes.** No envelope field is renamed. No CLI exit
code changes. No JSON envelope field is removed. The new subcommand
is additive in the same dispatcher (`experience <source> scan`).

## Out of scope

- `--watch` mode on Codex — owned by Hito 3 / PR #10 (Ola 2
  re-watch on the OpenCode slice); not duplicated here.
- Setup install/remove for Codex — already shipped; this PR is read-side only.
- Activation auto-trigger — deferred until at least OpenCode `--watch`
  ships (Hito 3).
- MCP tool exposure for `experience codex scan` — defer to Ola 3 so
  the MCP surface stays minimal in v1.
- Schema-bump tooling — `codex/rollout-v1` is the only schema we ship;
  when upstream changes, the adapter must return
  `experience_source_schema_unsupported` per `docs/22` §7. No silent
  fallback, no LLM-driven schema inference.
- Reading `~/.codex/session_index.jsonl` (or similar) as a source of
  truth — the index is upstream-controlled and may be stale; the
  adapter walks the rollout files directly.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Codex upstream changes rollout schema mid-cycle | Medium | Adapter validates first 1 KiB JSONL head in `Health()` and emits `experience_source_schema_unsupported` (`docs/22` §7); test pins the JSONL header shape including `session_meta` and `event_msg` event types |
| Codex Desktop vs CLI vs IDE diverge in schema | Medium | Single `SourceCodex` enum covers CLI + Desktop + IDE until schemas diverge; the threat model T16 already classifies ambiguous harness output as `host_llm`-equivalent (descriptive, not authoritative) |
| Sessions archived mid-scan cause partial reads | Low | Cursor is `(last_session_id, last_turn_uuid)`; a second scan picks up after the last emitted turn; `SkippedIncomplete` mirrors the opencode lesson (`docs/26` §9) |
| Reasoning blocks leak into `AssistantText` | Low | `reasoning` event subtypes are dropped at envelope-build time, not redacted; design.md §Scan enumerates the redacted-fields list explicitly |
| Walker descends into non-rollout files | Low | Discovery filters on the `rollout-<id>.jsonl` filename pattern; everything else is skipped at the walk step |
| `~/.codex/archived_sessions/` becomes enormous | Low | Walker caps discovery by depth; sort + pagination deferred to Ola 3; CLI accepts `--fixture` for test ergonomics |
| Review budget overshoot (350–450 LOC) | Medium | Slice 10.7 (acceptance + docs) stays under the budget; large real fixtures are excluded from the authored line count but stay inside the snapshot identity per `skills/_shared/sdd-phase-common.md` §E |
| `internal/experience/jobs/registry` change breaks the engine | Low | Registry upsert is idempotent; the new entry mirrors the existing `experience_ingest:opencode` shape; covered by the existing `TestJobs_Register_Idempotent` test |

## Alternatives considered

- **Single combined PR (Claude Code + Codex).** Rejected. `docs/26` §4
  explicitly says PR #11 and PR #12 are two PRs because (a) the
  review budget would overflow the 400-line threshold and (b) the
  operator wants independent rollback windows if one harness breaks
  the ingest pipeline.
- **Reuse the OpenCode SQLite parser.** Rejected. Codex uses
  append-only rollout JSONL with a different schema (no SQLite). The
  OpenCode adapter's `verifyOpenCodeSchema` would always return
  false; the shared `ExperienceAdapter` interface is the only
  reusable surface, and that is what the new adapter implements.
- **Read `session_index.jsonl` as a fast-path.** Rejected. The index
  is upstream-controlled and may list sessions whose rollout files
  have been rotated or archived. Walking the actual files keeps the
  adapter robust against index drift.
- **Treat `agent_message` as the only source of `AssistantText`.**
  Rejected. Codex assistant output is split across `event_msg`
  subtypes (`agent_message`) and `response_item` subtypes
  (`message`). The adapter must read both event families to build a
  faithful envelope, then drop `reasoning` subtypes entirely.
- **Wire `--watch` directly here.** Rejected. Hito 3 (PR #10) owns
  the watch loop on OpenCode; cloning the loop for Codex would
  duplicate the parent-pid lease handling. Once Hito 3 lands, the
  same loop reuses the registered `experience_ingest:codex` job name.

## Slices

The slice pattern mirrors the Hito 2 OpenCode pattern
(`HANDOFF-HITO2-OPENCODE-ONCE.md` §4) so the operator has the same
reviewable unit shape per PR. Each slice is small, shippable, and
produces testable artifacts. TDD strict: RED first, GREEN second.

| # | Sub-slice | What it ships | Acceptance reference |
|---|---|---|---|
| **10.0** | Scaffold | `internal/experience/codex/` package with `ExperienceAdapter` interface duplication (mirroring opencode + claudecode), `SchemaTag = "codex/rollout-v1"`, contract tests table | `docs/22` §1; contract tests compile + fail RED; coverage gate empty |
| **10.1** | `Discover()` | Walk `~/.codex/sessions/YYYY/MM/DD/rollout-<id>.jsonl` AND `~/.codex/archived_sessions/rollout-<id>.jsonl` reachable from the caller-supplied `project_root`; skip `session_index.jsonl`; reject symlinks, paths outside root, protected paths; depth cap 8; sorted output | `docs/24` T4; `docs/25` Hito 2 security row |
| **10.2** | `Health()` | `os.Stat` + read first 1 KiB of one rollout JSONL, validate at least one `session_meta` object; return `ok`/`degraded`/`error` with stable error code | `docs/22` §6; `docs/25` Hito 2 health row |
| **10.3** | `Scan()` | Iterate JSONL line by line, parse `session_meta` / `turn_context` / `event_msg` / `response_item`, build `ExperienceEnvelope`; drop `reasoning` subtypes; extract `function_call` + `function_call_output` into `ToolCalls[]`; count skipped lines | `docs/22` §3 + §6; `docs/25` Hito 2 fixture row |
| **10.4** | Idempotency | Cursor `{last_session_id, last_turn_uuid}`; second scan with same fixture against same DB produces zero duplicates via `(source, external_session_id, external_turn_id)` uniqueness | `docs/21` §2 + §4; `docs/25` Hito 2 reinicio row |
| **10.5** | `ResolveTrace()` | Bounded excerpt (1 KiB default), redacted via `evidence.Redact`, returns `trace_source_changed` when locator `SourceHash` differs, `trace_source_unavailable` when file/turn missing | `docs/22` §6; `docs/24` §4 + T12; `docs/25` Hito 4 trace rules for read-side |
| **10.6** | CLI `experience codex scan` | `cmd/royo-learn/experience.go` adds the `codex` subcommand; orchestrates Discover → Health → Scan → `service.IngestEnvelope`; JSON envelope on stdout matches the OpenCode scan shape with `source: "codex"`; `--project-root` required, `--fixture` optional | `docs/04` additive update; `docs/05` deferred MCP note; `docs/25` Hito 2 reinicio + cobertura row |
| **10.7** | Job registry + acceptance | Register `experience_ingest:codex` in `internal/experience/jobs/registry` (idempotent upsert); run the full acceptance matrix; cross-build windows/linux/darwin; coverage ≥ 85% on the new package; lessons file update | `docs/25` §2 Hito 8 jobs row + §4 coverage target; `docs/26` §6 gates |

Total expected: 8 atomic commits on `feat/hito10-codex` branched
from `origin/main` after PR #11 merges. Single PR at close
(PR #12 of the roadmap).

## Capability deltas (contract with sdd-spec)

The single capability affected is the additive extension of the
existing adapter contract. There is **no new top-level capability** —
the contract already covers `ExperienceAdapter` and the Hito 2
implementation under `internal/experience/opencode/` is the
reference. The delta spec under `specs/experience-adapters/spec.md`
lists the new requirements as `## MODIFIED` (extending the contract
to a third instance) and `## ADDED` (the four concrete methods on
the Codex adapter package, the schema tag, the locator kind
confirmation, and the job registry entry).