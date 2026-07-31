# Archive report — hito11-semantic

**Status**: COMPLETE — intentional with documented follow-ups
**Date**: 2026-07-30
**Archive target**: `openspec/changes/archive/2026-07-30-hito11-semantic/`
**Artifact store**: OpenSpec

## Native Review Receipt Gate

- Result: PASS
- Lineage: `experience-hito11-semantic-v1`
- Receipt: `.git/gentle-ai/review-transactions/v2/experience-hito11-semantic-v1/review-receipt.json`
- Terminal state: `approved`
- Risk level: `high` (4R full sweep)
- Selected lenses: `review-risk`, `review-resilience`, `review-readability`,
  `review-reliability`
- Generation: 1
- Base tree: `7c8ee0dcdfab316388380ae498359961d2d744e1`
- Final candidate tree: `567168dee4b4d892b430897864390fe527b98813`
- Finalize journal confirms `review/complete-verification` published the
  terminal receipt at the same revision.

## Task Completion Gate

Result: PASS — all implementation slices represented by conventional commits
on `feat/hito11-semantic`, merged to `main` via three chained PRs (PR #13
semantic package, PR #14 audit hook, PR #15 CLI collapse).

### Reconciled implementation evidence

- Slice 11.0: semantic package scaffold (`internal/experience/semantic/`) with
  enum types `JobIntent`, `JobScope`, `JobRiskClass` and audit-event-name
  constants (`job_pending`, `job_running`, `job_succeeded`, `job_failed`).
- Slice 11.1: neutral `Job` / `JobResult` / `JobTransition` types binding
  `JobRegistryEntry` to a runtime `JobFunc`.
- Slice 11.2: per-adapter `Job()` accessors in
  `internal/experience/{opencode,claudecode,codex}/jobs.go` replacing static
  constructors.
- Slice 11.3: SQL migration `migrations/008_job_semantics.sql` adding
  `intent`, `scope`, `risk_class` columns to `job_registry` plus new
  `job_run_log` table (`run_id`, `job_name`, `state`, `started_at`,
  `finished_at`, `error_code`, `error_message`, `attempt`).
- Slice 11.4: `jobs.RunOne` audit hook emitting the four job-lifecycle
  transitions to the append-only audit stream.
- Slice 11.5: CLI collapse — `experience scan --source=<opencode|claudecode|codex>`
  replacing the three per-source subcommands. Backwards-incompatible call-out
  documented in proposal.md.

### Repository taxonomy

- `jobs.Repository` exposed with explicit taxonomy so operators can answer
  "what is the job engine doing right now across all three sources" with one
  query instead of three.

### exceptional_repair.reason

> Sustitutos de `apply-progress`/`verify-report`: evidencia de git log, git diff
> y working tree actuales. El terminal receipt ya cubre la verificación
> estructural del cambio; la cobertura de paquetes se evidencia por los runs
> de CI en las PR #13, #14 y #15.

## Source-of-truth decision

- Delta sources:
  - `openspec/changes/hito11-semantic/specs/experience-adapters/spec.md`
  - `openspec/changes/hito11-semantic/specs/experience-cli-collapse/spec.md`
  - `openspec/changes/hito11-semantic/specs/job-semantic-engine/spec.md`
- Canonical merge targets:
  - `docs/22-ADAPTER-CONTRACT.md` (per-adapter `Job()` accessor contract)
  - `cmd/royo-learn/experience.go` (CLI collapse)
  - `internal/jobs/` (RunOne audit hook, repository taxonomy)
- Merge mode: additive; existing contract content preserved.
- `openspec/specs/*` were not created.
- `openspec/config.yaml` was not created.
- No `openspec/specs/` bootstrap was performed because the operator explicitly
  selected the existing `docs/`-based source of truth.

## Specs synchronized

- `docs/22-ADAPTER-CONTRACT.md` — per-adapter `Job()` accessor contract.
- `cmd/royo-learn/experience.go` — unified `experience scan --source=<…>`.
- `internal/jobs/repository.go` — explicit taxonomy for cross-source queries.

## Final verification

- Terminal receipt `approved`: PASS.
- 4R full sweep selected by START for high-risk profile: PASS.
- Final candidate tree matches merged state: PASS.
- Forbidden `openspec/specs/*` absent: PASS.
- Active change folder removed by move: PASS.
- Archive contains proposal, design, tasks, three delta specs, and this
  archive report: PASS.
- Final status: archived and closed.

## Engram traceability

Engram is unavailable in this OpenSpec-only executor environment. Observation IDs:
none. The persisted audit trail is this archive report, the archived change
folder, and the gentle-ai review transaction
`experience-hito11-semantic-v1`.
