# Handoff — Hito 10 Claude Code adapter (PR #11)

> **For the next session picking up this work.** This document captures the
> full state of `feat/hito10-claudecode` as of 2026-07-27 17:25 ART, plus
> what is needed to land PR #11 and what comes after.

## TL;DR

- **8 atomic commits** on `feat/hito10-claudecode` (slices 10.0–10.7), all
  conventional, no AI attribution.
- **All local gates green**: `go vet`, `gofmt -l`, `go test` across
  `internal/experience/claudecode/`, `internal/experience/jobs/`, and
  `cmd/royo-learn/`. The pre-existing RED on `TestAdapter_RespectsContextCancellation`
  for `ResolveTrace` is now GREEN (turned GREEN in slice 10.5).
- **Push + PR not executed from the agent's session** because the WSL
  sandbox cannot invoke `gh.exe` or the git credential helper ("Exec
  format error"). Scripts and PR body are ready in `/tmp/`.
- **Next PR**: PR #12 (Codex adapter), gated on PR #11 merging.

## Branch state

```
$ git log --oneline -9
0cab86e feat(experience): Hito 10 slice 10.7 - job registry for Claude Code adapter
81c2fa7 feat(cli): Hito 10 slice 10.6 - experience claude-code scan subcommand
b230f72 feat(experience): Hito 10 slice 10.5 - ResolveTrace for Claude Code adapter
95e932e feat(experience): Hito 10 slice 10.4 - Idempotency for Claude Code adapter
84306e6 feat(experience): Hito 10 slice 10.3 - Scan for Claude Code adapter
bc47dbd feat(experience): Hito 10 slice 10.2 - Health for Claude Code adapter
1f40b11 feat(experience): Hito 10 slice 10.1 - Discover for Claude Code adapter
e79e2e1 feat(experience): Hito 10 slice 10.0 - scaffold for Claude Code adapter
d4a3d63 Merge pull request #26 from RoyoTech/feat/hito9-retrieval
```

Base: `origin/main` @ `d4a3d63` (post-merge of Hito 9 PR #26).

Working tree: clean except untracked spec/scratch files
(`PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`, `openspec/changes/hito10-{claudecode,codex}/`).
These are intentional and remain untracked until archive.

## What the slices ship

| Slice | Files added/modified | Key behaviour |
|---|---|---|
| 10.0 | `internal/experience/claudecode/{adapter,contract,doc,claudecode_test}.go` | `ExperienceAdapter` interface, `Adapter` struct, types, contract test table |
| 10.1 | `discover.go`, `discover_test.go` | `Discover(ctx, projectRoot)`; canonical walk, symlink guard, depth cap 8, sorted output |
| 10.2 | `health.go`, `health_test.go`, `adapter.go` | `Health(ctx, instance)`; `os.Stat` + 1-KiB JSONL header probe, `ok`/`degraded`/`error` mapping |
| 10.3 | `scan.go`, `scan_test.go`, `testdata/fixtures/session-001.jsonl`, `adapter.go` | `Scan(ctx, req)`; JSONL→envelope, drop `thinking` blocks, sha256 locator, counters for malformed/incomplete/system |
| 10.4 | `cursor.go`, `idempotency_test.go`, `scan.go` | `cursorCheckpoint` decoder (string/int/int64/float64), cursor filter, service integration test |
| 10.5 | `resolve_trace.go`, `resolve_trace_test.go`, `adapter.go` | `ResolveTrace`; bounded redacted excerpt, `trace_source_changed` (advisory), `trace_source_unavailable` |
| 10.6 | `cmd/royo-learn/experience.go`, `experience_claudecode.go`, `experience_claudecode_test.go` | `experience claude-code scan --once --project-root [--fixture]` orchestrator + JSON envelope |
| 10.7 | `jobs.go`, `jobs_test.go` | `claudecode.JobRegistryEntry()` returning `experience_ingest:claude_code`; idempotent upsert |

## What is needed to land PR #11

### 1. Push and open the PR

Scripts and PR body are prepared in `/tmp/`:

- `/tmp/hito10-pr-body.md` — PR body (slices table, behaviour, acceptance, gates, out-of-scope, references). No AI attribution, no `Co-Authored-By`.
- `/tmp/hito10-push-and-pr.sh` — script for `git push -u origin feat/hito10-claudecode` + `gh pr create --base origin/main --body-file /tmp/hito10-pr-body.md`.

Run from the operator's native PowerShell or working WSL bash (not the
agent's sandbox):

```powershell
bash /mnt/c/Users/angel/AppData/Local/Temp/hito10-push-and-pr.sh
```

The WSL interop for `gh.exe` and the git credential helper fails in
the agent's sandbox ("Exec format error"); see memory
[[go-binary-location]] for context. This is a sandbox limitation, not
a repo problem.

### 2. Review

`gentle_review start` per the orchestrator rules. **Note**: per
lessons.md entry 5, the gentle-ai `validate` gate does not close the
loop on pre-push commits (this is the 4th consecutive gap-acceptance:
Hito 6, 7.1, 9, 10). The operator has two options:

- **Option a (chosen in prior hitos)**: accept the gap. Document the
  receipt gap in the commit message and proceed to push. The review
  budget is then satisfied by the commit-message trail plus
  `docs/IMPLEMENTATION-NOTES.md`.
- **Option b (halt)**: stop and ask the maintainer to investigate why
  `validate` demands a `lineageId` that `inspect` itself does not list
  as a candidate. See lessons.md entry 5 for details.

### 3. CI gates

The following gates are not runnable from the local sandbox and are
owned by CI:

- `race-linux` (`go test -race ./...` — requires `gcc`, not installed locally)
- `cross-build` (windows/linux/darwin amd64)
- `coverage-linux` (≥ 85% on `internal/experience/claudecode/`)
- The standard `e2e` and `integration` suites

If any of these fail, the operator triages and either fixes in this
branch or reverts.

## What comes after PR #11

### PR #12 — Codex adapter

The proposal, design, and tasks for the Codex adapter are already
written in `openspec/changes/hito10-codex/`. Slice breakdown mirrors
PR #11 (12.0 scaffold → 12.7 job registry).

- Branch: `feat/hito10-codex` from `origin/main` **after** PR #11
  merges. **Do not** branch from local `main` after PR #11 merge
  (per lessons.md entry 4).
- Schema tag: `codex/rollout-v1`.
- Discovery walk: `~/.codex/sessions/YYYY/MM/DD/rollout-<id>.jsonl` and
  `~/.codex/archived_sessions/rollout-<id>.jsonl`; skip
  `session_index.jsonl`.
- All other constraints identical to PR #11 (no `openspec/`, no
  `docs/`, no `chore` commits; one slice per commit; clean slice
  boundaries).

### Hito 11 (semántica) and Hito 12 (drift/release)

Deferred to Ola 3 per `docs/26-IMPLEMENTATION-ROADMAP.md`. Both need a
gate review before activation.

## Operational notes for the next session

- **Go toolchain**: `/home/angel/local/go/bin/`. Add to PATH before any
  `go` command. See [[go-binary-location]].
- **`sdd-apply` scope**: agents must not touch `openspec/`, `docs/`, or
  add `chore` commits. The recipe for cleanup is `git reset --mixed
  HEAD~1` until the offending commit is undone, then re-stage the
  code-only files. See [[sdd-apply-scope-spec]].
- **Spec finalization**: `openspec/changes/hito10-claudecode/{proposal,design,tasks}.md`
  stay untracked until archive. The operator finalizes them in
  `sdd-archive` (post-verification), not in apply.
- **Open follow-up**: `experience_ingest:opencode` registration does
  not exist. The proposal referenced it as a precedent; 10.7 introduced
  the first one (claude_code). The opencode one can be added either as
  a separate small PR (a new slice in the opencode package) or as a
  follow-up to PR #11. Decide before opening PR #12 to keep the job
  registry symmetric across sources.
- **Working tree cleanup**: 8 prunable worktrees under
  `Temp/opencode/royo-learn-review-*` from prior review runs are
  unused. Safe to remove with `git worktree prune` when the operator
  wants to reclaim disk.

## Reference index

- Spec: `openspec/changes/hito10-claudecode/{proposal,design,tasks}.md`
- Implementation: `internal/experience/claudecode/`, `cmd/royo-learn/experience_claudecode.go`
- Job registration: `internal/experience/claudecode/jobs.go` → `claudecode.JobRegistryEntry()`
- Contract: `docs/22-ADAPTER-CONTRACT.md` §1–§7
- Threat model: `docs/24-EXPERIENCE-THREAT-MODEL.md` T1, T4, T7, T8, T12
- Acceptance: `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` Hito 2 row + Hito 8 jobs row
- Roadmap: `docs/26-IMPLEMENTATION-ROADMAP.md` §4 PR #11
- Closing notes: `docs/IMPLEMENTATION-NOTES.md` (section "Hito 10 — Claude Code adapter (PR #11)")
- Lessons: `docs/lessons.md` entries 2, 4, 5 (shell detection, PR base, review gap)
- Memories: [[go-binary-location]], [[sdd-apply-scope-spec]]
