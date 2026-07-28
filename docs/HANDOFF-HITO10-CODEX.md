# Handoff — Hito 10 Codex adapter (PR #12)

> **For the next session picking up this work.** This document captures the
> full state of `feat/hito10-codex` as of 2026-07-28 11:30 ART, plus what is
> needed to land PR #12 and what comes after.

## TL;DR

- **12 atomic commits** on `feat/hito10-codex` (slices 10.0–10.7, coverage
  boost, 3 bounded-review correction commits).
- **All local gates green**: `go build ./...`, `go vet ./...`, `gofmt -l`
  clean, `go test ./internal/experience/codex/...` ok,
  coverage 94.4% (≥ 85% target).
- **4R review completed** with `gentle-ai review start --base-ref
  origin/main --workspace-overlay --lineage
  experience-hito10-codex-v1`. SEVERE blocker resolved; other findings
  either fixed or classified as base-only (mirror of claudecode).
- **Push + PR + gentle-ai finalize not executed from the agent session**.
  Operator handles these (per `docs/lessons.md` entry 2: WSL interop
  blocks `gh` / credential helper from the sandbox).
- **Next PR**: Ola 3 / Hito 11 (semántica) and Hito 12 (drift/release),
  gated on both PR #11 and PR #12 merging.

## Branch state

```
$ git log --oneline -12
c79c5cf refactor(experience): Hito 10 Codex JobRegistryEntry uses injectable clock for determinism
87f40f4 feat(experience): Hito 10 Codex scan extracts Actor.Model from turn_context
f72ceb4 fix(experience): Hito 10 Codex ResolveTrace drops reasoning and function_call_output
b3bc8cf test(experience): Hito 10 Codex coverage ≥ 85% on internal/experience/codex
abe351b feat(experience): Hito 10 slice 10.7 - job registry for Codex adapter
e0f4021 feat(experience): Hito 10 slice 10.6 - experience codex scan CLI
1f605ca feat(experience): Hito 10 slice 10.5 - ResolveTrace for Codex adapter
8a69ea6 feat(experience): Hito 10 slice 10.4 - Cursor idempotency for Codex adapter
a282ed8 feat(experience): Hito 10 slice 10.3 - Scan for Codex adapter
e5d99c4 feat(experience): Hito 10 slice 10.2 - Health for Codex adapter
6d7a7aa feat(experience): Hito 10 slice 10.1 - Discover for Codex adapter
0ce6e7f feat(experience): Hito 10 slice 10.0 - scaffold for Codex adapter
```

Base: `origin/main` @ `b3c9fd1` (Hito 10 closure + Ola 2 bridge phrase +
gitignore update; PR #11 still unmerged on remote).

Working tree: clean except untracked `openspec/changes/hito10-codex/`
(intentional, remains untracked until archive).

## Symmetry with PR #11 (Claude Code reference)

| Concern | Claude Code (PR #11) | Codex (PR #12) |
|---|---|---|
| Schema tag | `claude-code/jsonl-v1` | `codex/rollout-v1` |
| Source enum | `domain.SourceClaudeCode` | `domain.SourceCodex` |
| Session root | `.claude/projects/<uuid>` | `.codex/sessions/...` |
| File pattern | `session-<uuid>.jsonl` | `rollout-<id>.jsonl` |
| Job name | `experience_ingest:claude_code` | `experience_ingest:codex` |
| Adapter interface | 5 methods (`Name`, `Discover`, `Health`, `Scan`, `ResolveTrace`) | identical |
| Cursor | `{last_session_id, last_turn_uuid}` string-UUID | identical |
| SafeToolCall fields | `Name, Arguments, Outcome, ...` | identical |
| Actor.Name | `"claude_code"` | `"codex"` |
| Drop-at-scan rule | `thinking` blocks | `reasoning` + `function_call_output` |

Codex-specific extras:

- `Actor.Model = turn_context.model` (extracted in scan.go; commit
  `87f40f4`).
- ResolveTrace double-filters `response_item` with
  `payload.type in {reasoning, function_call_output}` so the trace never
  re-exposes content the scan dropped (commit `f72ceb4`).

## What is needed to land PR #12

### 1. `gentle-ai review finalize` (manual)

The agent-session capture-result hit the strict schema. The orchestrator
rules forbid improvising flags, so finalize was not auto-bound. Schema
required per finding:

- `id` (kebab-case slug)
- `location` (file:line)
- `severity` (`blocker`/`severe`/`warning`/`suggestion`)
- `claim` (one-sentence neutral claim)
- `proof_refs` (array of file:line refs)

Top-level required:

- `findings` (array)
- `evidence` (array of strings — NOT objects)

Fields the reviewers emit but the strict decoder REJECTS as unknown:
`category`, `file`, `line`, `summary`, `failure_scenario`, `confidence`,
`causal_disposition`, `evidence_class`, `proposed_fix`.

Findings inventory:

| Lens | Count | Blockers |
|---|---|---|
| review-risk | 1 suggestion | none |
| review-resilience | 5 warnings + 3 suggestions | none (mostly base-only) |
| review-readability | 0 findings | — |
| review-reliability | 1 SEVERE + 5 warn/sugg | SEVERE fixed (`f72ceb4`), other 5 fixed |

Operator runs:

```bash
gentle-ai review finalize \
  --cwd /mnt/c/wordpress-lab/wp-content/proyectos/agent-royo-learn-codex-spec \
  --lineage experience-hito10-codex-v1 \
  --correction-lines 137 \
  --validation <scoped-validator-output> \
  --evidence <final-go-test-output>
```

### 2. Push and open the PR

The Codex PR body should mirror the Claude Code PR template
(`/tmp/hito10-pr-body.md` for the precedent). Key sections:

- Symmetry table (above).
- Slice breakdown (10.0 → 10.7).
- Bounded-review correction note (commit trail of `f72ceb4`,
  `87f40f4`, `c79c5cf`).
- Known-unrelated failure: `internal/publish/TestPublish_RollbackFailureObserved`
  is pre-existing on `origin/main`, WSL perms (not a Codex regression).
- Acceptance: coverage 94.4% ≥ 85% gate.

Run from native PowerShell or working WSL bash (not the agent sandbox,
per [[go-binary-location]] and lessons.md entry 2).

### 3. CI gates (operator-monitors)

- `race-linux` — needs `gcc` (CI only).
- `cross-build` — windows/linux/darwin amd64.
- `coverage-linux` — confirm 94.4% on `internal/experience/codex/`.

## What comes after PR #12

### Ola 3 — Hito 11 (semántica) and Hito 12 (drift/release)

Blocked on both PR #11 and PR #12 merging. New change artifacts under
`openspec/changes/hito11-*/` and `openspec/changes/hito12-*/`. Per
`docs/26-IMPLEMENTATION-ROADMAP.md`, the job engine becomes symmetric
across the three adapters (opencode + claudecode + codex) and the
release channel adds drift detection.

## Operational notes for the next session

- **Go toolchain**: `/home/angel/local/go/bin/`. See [[go-binary-location]].
- **`sdd-apply` scope**: agents must not touch `openspec/`, `docs/`, or
  add `chore` commits. See [[sdd-apply-scope-spec]].
- **gentle-ai finalize schema gotcha**: documented in
  [[hito10-codex-review-fixes]] for future bounded reviews.
- **Spec finalization**: `openspec/changes/hito10-codex/{proposal,design,tasks,specs}/`
  stay untracked until `sdd-archive` (post-verification).
- **Working tree cleanup**: 8 prunable worktrees under
  `Temp/opencode/royo-learn-review-*` from prior review runs are
  unused. Safe to remove with `git worktree prune`.

## Bridge phrase

> **Continuamos desde el cierre de Hito 10 (PR #12 Codex listo,
> 12 commits en `feat/hito10-codex`, 4R review cerrado, SEVERE
> trace-leak resuelto). Próximo paso: operator corre
> `gentle-ai review finalize` con schema estricto, push, y PR #12.
> Cuando ambas PR (Claude Code + Codex) estén mergeadas, arranca
> Ola 3 — Hito 11 semántica + Hito 12 drift/release con motor de
> jobs simétrico entre los tres adapters.**

## Reference index

- Spec: `openspec/changes/hito10-codex/{proposal,design,tasks}.md`
- Spec delta: `openspec/changes/hito10-codex/specs/experience-adapters/spec.md`
- Implementation: `internal/experience/codex/`, `cmd/royo-learn/experience_codex.go`
- Job registration: `internal/experience/codex/jobs.go` → `codex.JobRegistryEntry()`
- Contract: `docs/22-ADAPTER-CONTRACT.md` §1–§7
- Threat model: `docs/24-EXPERIENCE-THREAT-MODEL.md`
- Acceptance: `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` Hito 10 row
- Roadmap: `docs/26-IMPLEMENTATION-ROADMAP.md` §4 PR #12, §5 Ola 3
- Closing notes: `docs/IMPLEMENTATION-NOTES.md` (append "Hito 10 — Codex adapter (PR #12)")
- Lessons: `docs/lessons.md` entries 2, 4, 5
- Memories: [[go-binary-location]], [[sdd-apply-scope-spec]],
  [[hito10-codex-review-fixes]], [[session-2026-07-27-hito10]]