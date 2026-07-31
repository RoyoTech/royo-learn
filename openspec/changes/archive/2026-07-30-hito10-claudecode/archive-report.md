# Archive report — hito10-claudecode

**Status**: COMPLETE — closed with documented reconciliation gap
**Date**: 2026-07-30
**Archive target**: `openspec/changes/archive/2026-07-30-hito10-claudecode/`
**Artifact store**: OpenSpec

## Native Review Receipt Gate

- Result: NOT_APPLICABLE — no dedicated review transaction exists for this change
- Rationale: Claude Code adapter (PR #11) was the reference implementation that
  preceded the formal bounded-review workflow on this branch. The mirror review
  performed for `experience-hito10-codex-v1` (PR #12) validated the symmetry and
  detected no regressions in the Claude Code package.
- Reference review: `.git/gentle-ai/review-transactions/v2/experience-hito10-codex-v1/`
- Final verification: `go test ./...` and `go test ./internal/experience/claudecode/...`
  green; coverage ≥ 85% target met.

## Task Completion Gate

Result: PASS — all eight implementation slices (10.0–10.7) represented by
conventional commits on `feat/hito10-claudecode`, merged to `main` via PR #11.

### Reconciled implementation evidence

- Slice 10.0: Claude Code package scaffold, schema tag `claude-code/jsonl-v1`,
  interface assertion, contract tests, build and vet evidence.
- Slice 10.1: safe deterministic discovery of `session-<uuid>.jsonl` files under
  `.claude/projects/`, root/symlink guards, cancellation tests.
- Slice 10.2: read-only Health validation, typed degraded/error mappings.
- Slice 10.3: streaming JSONL scan, anchors, neutral envelopes, malformed/incomplete
  counters, thinking-block drop, bounded function-output representation.
- Slice 10.4: stable cursor and service-level idempotency with restart coverage.
- Slice 10.5: bounded redacted trace resolution, source-change and unavailable
  mappings.
- Slice 10.6: additive CLI `experience claude-code scan`, stable JSON envelope,
  required project root, guarded fixture path.
- Slice 10.7: idempotent job registration `experience_ingest:claude_code`,
  race/test/cross-build evidence.

### exceptional_repair.reason

> Sustitutos de `apply-progress`/`verify-report`: evidencia de git log, git diff
> y working tree actuales. Claude Code precedió el flujo formal de bounded-review
> en esta rama; el review del espejo Codex (`experience-hito10-codex-v1`) validó
> la simetría y no encontró regresiones en el paquete Claude Code.

This authorization applies to stale-checkbox reconciliation. No CRITICAL
verification issue remains: cross-package review via the Codex mirror transaction
confirmed symmetry and the implementation is merged.

## Source-of-truth decision

- Delta source: `openspec/changes/hito10-claudecode/specs/experience-adapters/spec.md`.
- Canonical merge target: `docs/22-ADAPTER-CONTRACT.md`.
- Merge mode: additive; existing contract content preserved.
- `openspec/specs/experience-adapters/spec.md` was not created.
- `openspec/config.yaml` was not created.
- No `openspec/specs/` bootstrap was performed because the operator explicitly
  selected `docs/22-ADAPTER-CONTRACT.md` as the existing source of truth.

## Specs synchronized

`docs/22-ADAPTER-CONTRACT.md` now includes the Claude Code JSONL adapter contract
extension (merged during Ola 2 / Hito 10 closure):

- concrete `ExperienceAdapter` implementation and canonical source name;
- JSONL locator behavior;
- `claude-code/jsonl-v1` schema handling;
- safe deterministic discovery;
- neutral scan envelopes, counters, cursor, and idempotency;
- bounded redacted trace resolution;
- additive CLI contract;
- idempotent job registration.

## Final verification

- Main contract updated additively: PASS.
- Forbidden `openspec/specs/experience-adapters/spec.md` absent: PASS.
- Active change folder removed by move: PASS.
- Archive contains proposal, design, tasks, delta spec, and this archive report: PASS.
- Final status: archived and closed with the documented gap (no dedicated
  review transaction; covered by Codex mirror review).

## Engram traceability

Engram is unavailable in this OpenSpec-only executor environment. Observation IDs:
none. The persisted audit trail is this archive report, the archived change
folder, and the mirrored Codex review transaction.
