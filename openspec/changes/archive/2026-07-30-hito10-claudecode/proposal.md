# Proposal: Hito 10 — Claude Code adapter (PR #11)

## Why

OpenCode was the pilot that froze `docs/22-ADAPTER-CONTRACT.md` (Hito 0) and the
Hito 2 read-only path. Claude Code is the next source the platform must
ingest without changing the core experience.Service, the envelope shape, or
the CLI/MCP surface. Without this PR the local-only experience layer is
incomplete for any contributor who actually drives sessions in Claude Code
instead of OpenCode, so the operator cannot audit what the assistant learned
from sessions that did not run in OpenCode.

This is one of two PRs in Ola 2 that close the "experience discovery across
harnesses" gap (`docs/26-IMPLEMENTATION-ROADMAP.md` §4 PR #11 + PR #12). The
two PRs are independent on purpose: each adapter needs its own review
budget, its own fixtures, and its own Ola 2 trigger when `--watch` lands.

This PR is the **read-side** half of the Claude Code integration. The
write-side (setup install --agent) already exists in
`internal/setup/claudecode.go` and is **not** touched here. The trigger
`v0.3.0` is the last PR before the Ola 2 tag; cutting it depends on this PR
merging on `feat/hito10-claudecode`.

## What changes

| Area | Type | Description |
|---|---|---|
| `internal/experience/claudecode/` | new | Adapter implementing `ExperienceAdapter` (4 methods) over JSONL transcripts |
| `internal/experience/jobs/registry` | additive | Register `experience_ingest:claude_code` static entry so Hito 3 (`--watch`) is single-source |
| `cmd/royo-learn/experience.go` | additive | New `claude-code` subcommand dispatcher and `experience claude-code scan` orchestrator reusing the OpenCode envelope shape |
| `docs/04-CLI-SPEC.md` | additive | Document `experience claude-code scan --once --project-root [--fixture]` |
| `docs/05-MCP-SPEC.md` | additive | Note that the new subcommand is CLI-only in v1; MCP routing is deferred to Ola 3 |

`docs/21`, `docs/22`, `docs/26` are frozen contracts from Hito 0 and are
referenced, not edited. Setup layer (`internal/setup/claudecode.go`) is not
touched — that is the write-side, owned by an earlier PR.

## Impact

| Spec / file | Impact | What |
|---|---|---|
| `docs/22-ADAPTER-CONTRACT.md` §1, §7 | referenced | Add a new instance of the four-method contract; schema tag `claude-code/jsonl-v1` per §7 |
| `docs/21-EXPERIENCE-DOMAIN.md` §1, §3 | referenced | Reuse `SourceClaudeCode` enum (already declared) and add `jsonl` to `TranscriptLocator.Kind` |
| `docs/24-EXPERIENCE-THREAT-MODEL.md` | referenced | Same threat model: T1 (prompt injection), T4 (path traversal), T7 (secret multichannel), T8 (truncated JSONL), T12 (source mutated) apply identically |
| `docs/04-CLI-SPEC.md` | additive only | Add the `claude-code scan` subcommand table |
| `docs/05-MCP-SPEC.md` | additive only | One-line note that `claude-code` stays CLI in Ola 2 |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §4 | referenced | PR #11 row references this proposal as the deliverable |

**No breaking changes.** No envelope field is renamed. No CLI exit code
changes. No JSON envelope field is removed. The new subcommand is additive
in the same dispatcher (`experience <source> scan`).

## Out of scope

- `--watch` mode on Claude Code — owned by Hito 3 / PR #10 (Ola 2
  re-watch on the OpenCode slice); not duplicated here.
- Setup install/remove for Claude Code — already shipped; this PR is read-side only.
- Activation auto-trigger — deferred until at least OpenCode `--watch`
  ships (Hito 3).
- MCP tool exposure for `experience claude-code scan` — defer to Ola 3 so
  the MCP surface stays minimal in v1.
- Schema-bump tooling — `claude-code/jsonl-v1` is the only schema we ship;
  when upstream changes, the adapter must return
  `experience_source_schema_unsupported` per `docs/22` §7. No silent
  fallback, no LLM-driven schema inference.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Claude Code upstream changes JSONL schema mid-cycle | Medium | Adapter validates first 1 KiB JSONL head in `Health()` and emits `experience_source_schema_unsupported` (`docs/22` §7); test pins the JSONL header shape |
| Fixture realism is too synthetic | Medium | Fixtures are anonymized real JSONL (per AGENTS.md regla 14), one file per session, matching upstream layout; CI scans for secrets via existing `internal/evidence` redaction tests |
| Windows 8.3 short paths collide in `~/.claude/projects/<encoded>` | Low | Discovery uses `projectpath.Canonicalize` + `IsInsideRoot`; canonicalization calls `filepath.EvalSymlinks` so short paths are resolved before the locator is built |
| Idempotency regresses under mid-scan append | Low | Cursor checkpoint is `(last_session_id, last_turn_uuid)` and is committed by the core service after `IngestEnvelope` returns, never by the adapter (INV-16) |
| No-LLM rule violated by "helpful" reasoning about transcript | Low | Adapter is descriptive; `thinking` content blocks are dropped at envelope-build time and never reach the core service; design.md §Scan enumerates the redacted-fields list explicitly |
| Review budget overshoot (350–450 LOC) | Medium | Slice 10.7 (acceptance + docs) stays under the budget; large real fixtures are excluded from the authored line count but stay inside the snapshot identity per `skills/_shared/sdd-phase-common.md` §E |
| `internal/experience/jobs/registry` change breaks the engine | Low | Registry upsert is idempotent (`ON CONFLICT(job_name) DO UPDATE`); the new entry mirrors the existing `experience_ingest:opencode` shape; covered by the existing `TestJobs_Register_Idempotent` test |

## Alternatives considered

- **Single combined PR (Claude Code + Codex).** Rejected. `docs/26` §4
  explicitly says PR #11 and PR #12 are two PRs because (a) the review
  budget would overflow the 400-line threshold and (b) the operator wants
  independent rollback windows if one harness breaks the ingest pipeline.
- **Reuse the OpenCode SQLite parser.** Rejected. Claude Code uses
  append-only JSONL with a different schema (no SQLite). The OpenCode
  adapter's `verifyOpenCodeSchema` would always return false; the shared
  `ExperienceAdapter` interface is the only reusable surface, and that is
  what the new adapter implements.
- **Wire `--watch` directly here.** Rejected. Hito 3 (PR #10) owns the
  watch loop on OpenCode; cloning the loop for Claude Code would
  duplicate the parent-pid lease handling. Once Hito 3 lands, the same
  loop reuses the registered `experience_ingest:claude_code` job name.
- **Decode the URL-encoded project slug to find the project root.**
  Rejected. The slug is opaque and lossy. The CLI/MCP caller must pass
  `--project-root` explicitly so `Canonicalize` + `IsInsideRoot` reject
  any session file outside the trust boundary.

## Slices

The slice pattern mirrors the Hito 2 OpenCode pattern
(`HANDOFF-HITO2-OPENCODE-ONCE.md` §4) so the operator has the same
reviewable unit shape per PR. Each slice is small, shippable, and
produces testable artifacts. TDD strict: RED first, GREEN second.

| # | Sub-slice | What it ships | Acceptance reference |
|---|---|---|---|
| **10.0** | Scaffold | `internal/experience/claudecode/` package with `ExperienceAdapter` interface duplication (mirroring opencode), `SchemaTag = "claude-code/jsonl-v1"`, contract tests table | `docs/22` §1; contract tests compile + fail RED; coverage gate empty |
| **10.1** | `Discover()` | Walk `~/.claude/projects/<encoded>/<session-uuid>.jsonl` reachable from the caller-supplied `project_root`; reject symlinks, paths outside root, protected paths; depth cap 8; sorted output | `docs/24` T4; `docs/25` Hito 2 security row |
| **10.2** | `Health()` | `os.Stat` + read first 1 KiB of JSONL, validate at least one object with `type`/`uuid`/`sessionId`; return `ok`/`degraded`/`error` with stable error code | `docs/22` §6; `docs/25` Hito 2 health row |
| **10.3** | `Scan()` | Iterate JSONL line by line, skip malformed lines (counter exposed), build `ExperienceEnvelope` with `UserText` / `AssistantText` / `ToolCalls[]`, redact `thinking` blocks at envelope build, drop `SourceRevision = uuid:revision` | `docs/22` §3 + §6; `docs/25` Hito 2 fixture row |
| **10.4** | Idempotency | Cursor `{last_session_id, last_turn_uuid}`; second scan with same fixture produces zero duplicates via `(source, external_session_id, external_turn_id)` uniqueness in the core service | `docs/21` §2 + §4 (uniqueness invariant); `docs/25` Hito 2 reinicio row |
| **10.5** | `ResolveTrace()` | Bounded excerpt (1 KiB default), redacted via `evidence.Redact`, returns `trace_source_changed` when locator `SourceHash` differs, `trace_source_unavailable` when file/turn missing | `docs/22` §6; `docs/24` §4 + T12; `docs/25` Hito 2 source-mutated row |
| **10.6** | CLI `experience claude-code scan` | `cmd/royo-learn/experience.go` adds the `claude-code` subcommand; orchestrates Discover → Health → Scan → `service.IngestEnvelope`; JSON envelope on stdout matches the OpenCode scan shape with `source: "claude_code"`; `--project-root` required, `--fixture` optional | `docs/04` additive update; `docs/25` Hito 2 reinicio + cobertura row |
| **10.7** | Job registry + acceptance | Register `experience_ingest:claude_code` in `internal/experience/jobs/registry` (idempotent upsert); run the full acceptance matrix; cross-build windows/linux/darwin; coverage ≥ 85% on the new package; lessons file update | `docs/25` §2 Hito 8 jobs row + §4 coverage target; `docs/26` §6 gates |

Total expected: 8 atomic commits on `feat/hito10-claudecode` branched
from `origin/main`. Single PR at close (PR #11 of the roadmap).

## Capability deltas (contract with sdd-spec)

The single capability affected is the additive extension of the existing
adapter contract. There is **no new top-level capability** — the contract
already covers `ExperienceAdapter` and the Hito 2 implementation under
`internal/experience/opencode/` is the reference. The delta spec under
`specs/experience-adapters/spec.md` lists the new requirements as
`## MODIFIED` (extending the contract to a second instance) and
`## ADDED` (the four concrete methods on the Claude Code adapter
package, the schema tag, the locator kind, and the job registry entry).