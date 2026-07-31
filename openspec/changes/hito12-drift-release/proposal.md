# Proposal: Hito 12 — Drift / Release Hardening

## Intent

The `royo-learn` experience layer (Hitos 0–11) ingests sessions from
OpenCode, Claude Code, and Codex and emits `ExperienceEnvelope`s into
the audit + publication pipeline. Two structural gaps prevent the
operator from trusting this pipeline at the v1.0.0 boundary:

1. **Drift detection is partial**. Each adapter detects source-level
   drift (SHA-256 mismatch / source unavailable) and emits typed error
   codes (`trace_source_changed`, `trace_source_unavailable`,
   `trace_event_unavailable`, `experience_source_not_found`). But
   **publication-level drift** — "the file we just published no longer
   matches the trace we published from" — has **zero** implementation:
   `publication_drift_check` only exists in `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
   §18 (lines 1199, 1505–1508, 1524, 2303); no SQL migration, no
   `job_registry` row, no package. And the operator has no unified
   surface to ask "what drifted across all three sources?" — each
   adapter must be queried by hand.

2. **Drift policy is asymmetric across adapters**. `docs/22-ADAPTER-CONTRACT.md`
   Scenario "Source changes or disappears" implies "no excerpt" for all
   three, but `claudecode/resolve_trace.go:101–117` returns the bounded
   excerpt as *advisory* while `opencode` and `codex` omit it. This is
   real semantic drift between adapters on the same contract scenario.

3. **Release artifacts are incomplete**. `install.sh`/`install.ps1` are
   solid (SHA-256 + atomic replace + cleanup + uninstall), GoReleaser
   builds 6 targets, CI is a 3×2 quality matrix. But the release lacks
   a SBOM (despite `docs/11-INSTALLATION.md:124` listing it), has no
   consolidated `RELEASE.md` runbook, and `CHANGELOG.md` is frozen at
   Hito 6 — the `[Unreleased]` section still misses entries for Hitos
   8/9/10/11 and `v1.0.0` is still marked ⏳ after the PRs have
   already merged.

This change closes all three gaps with **four deliverables that fit
in one PR (~710 LOC total, ~390 against the 200-line correction
budget via `min(200, ceil(changed_lines/2))`)** and ships no
backwards-incompatible CLI/MCP surface.

## Scope

### In Scope

**Deliverable 1 — Publication drift core** (medium risk: filesystem +
SQLite transactional).

- New SQL migration `internal/storage/migrations/009_publication_drift.sql`
  adding the `publication_drift_state` table
  (`publication_id TEXT`, `source TEXT`, `target_path TEXT`,
  `expected_hash TEXT`, `actual_hash TEXT`, `status TEXT CHECK(status IN
  ('ok','drifted','target_missing','target_unreadable'))`,
  `checked_at TIMESTAMP`, `run_id TEXT`).
- New package `internal/publish/drift/` with
  `Checker.Check(ctx, target, expectedHash) (Result, error)` returning
  the four outcomes above. Read-only on the target (stat + sha256, no
  write).
- New job `publication_drift_check` registered in `job_registry`
  (`intent = "drift"`, `scope = "project"`, `risk_class = "low"`),
  reusing the `semantic.JobFunc` runtime introduced in Hito 11 and the
  shared `jobs.Service.RunOne` audit hook.
- Gate: drift check runs only for rows with `publications.status =
  'published'` (never `in_progress`).
- CLI: `experience drift --all-sources` walks the registered
  publications and emits a unified JSON envelope.

**Deliverable 2 — Unified drift CLI/MCP** (low risk).

- `cmd/royo-learn/experience.go` gains a `runExperienceDrift` subcommand
  with `--all-sources` (default) and `--source=<opencode|claudecode|codex>`
  filters.
- New MCP tool `experience_drift_status` exposing the same envelope via
  `internal/mcp/`. Output schema: `{ "sources": [...], "publications":
  [...] }` with **no excerpt bodies, no PII, no transcript text**.
- Stable JSON contract tested via golden fixture.

**Deliverable 3 — Adapter parity** (low risk: refactor of policy).

- `internal/experience/claudecode/resolve_trace.go` reconciles with the
  opencode/codex policy: on `trace_source_changed` /
  `trace_source_unavailable`, the resolver returns **no excerpt** (the
  code path that emitted the bounded advisory excerpt is removed).
- `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or disappears"
  is tightened to forbid excerpt on drift across all three adapters
  (one sentence, contract-equivalent).
- Contract test added in each of the three adapter packages
  (`TestResolveTrace_SourceChanged_OmitsExcerpt`) fixing the parity at
  compile/runtime.

**Deliverable 4 — Release extras** (low risk: docs + YAML).

- `.goreleaser.yml` gains SBOM emission (`brews`, archives
  `formats: tar.gz,zip`). If `sbom` format is unsupported by
  GoReleaser v2.17.0, declare the gap explicitly in `RELEASE.md`.
- New `RELEASE.md` runbook documenting the trigger table (PR merge →
  CI → release workflow), required CI success, tag creation,
  `workflow_run` SHA alignment, atomic install verification, rollback
  recipe (re-run `install.sh --uninstall`, reinstall previous tag).
  Linked from `docs/15-OPERATIONS.md`.
- `CHANGELOG.md` backfill: move Hitos 8, 9, 10, 11 entries from
  `[Unreleased]` to their release sections (synthesized from `git log`
  PR titles), demote `v1.0.0` ⏳ marker to a clear "no tag yet"
  section.

### Out of Scope

- **Code signing / detached signatures** (`gpg`/cosign) for the release
  artifacts. Deferred to a follow-up; install scripts verify SHA-256
  only.
- **Dependabot / Renovate config** for `go.mod` — dependency review
  stays manual until a separate change introduces the policy.
- **v1.0.0 release tagging** itself. This change ships the
  preconditions (SBOM, runbook, changelog) but does **not** create the
  `v1.0.0` tag. Tagging is a separate operator decision once the
  release extras are merged.
- **Backfilling CHANGELOG entries for Hito 8 / 9 / 10 / 11 PR
  descriptions** beyond what `git log` already supplies. The backfill
  uses PR titles only, not bodies.
- **`goreleaser-prerelease: auto`** behavior change for v1.0.0. Kept as
  documented; revisited if/when tagging happens.
- **New audit-event names**. Drift outcomes are recorded in the new
  `publication_drift_state` table, not as new operations on
  `audit_events`.
- **Hito 3 `--watch` flip** on any of the three ingest jobs
  (`Enabled = false` remains the post-Hito-11 invariant).
- **Cross-adapter `ScanResult` shape unification** (skipped counters).
  That is the symmetric-job follow-up explicitly listed as
  out-of-scope in `hito11-semantic/proposal.md`.

## Capabilities

### New Capabilities

- `publication-drift-check`: covers the SQL migration 009, the
  `internal/publish/drift` package, the `Checker` contract with its
  four outcomes, the `publication_drift_check` job registration, and
  the `Status='published'` gate.
- `drift-cli-mcp`: covers the unified `experience drift --all-sources`
  CLI subcommand, the `experience_drift_status` MCP tool, and the
  stable JSON envelope `{ "sources": [...], "publications": [...] }`.
- `release-extras`: covers SBOM emission in `.goreleaser.yml`, the new
  `RELEASE.md` runbook, and the `CHANGELOG.md` backfill to Hito 6.

### Modified Capabilities

- `experience-adapters` (current delta under `hito11-semantic`):
  `claudecode/resolve_trace.go` removes the advisory excerpt branch on
  source mismatch. `opencode` and `codex` are unchanged. This change
  produces a delta spec under
  `openspec/changes/hito12-drift-release/specs/experience-adapters/`.
- `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or
  disappears": one-sentence tightening forbidding excerpt on drift
  across all three adapters.
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2: adds acceptance rows
  for the four publication-drift outcomes, the unified CLI/MCP
  envelope, and adapter parity.

## Approach

Per `sdd/{change}/explore` §6 (drift) and §7 (release):

1. **Migration 009**:
   `internal/storage/migrations/009_publication_drift.sql` is one
   idempotent forward-only file. The runner
   (`internal/storage/migrate.go`) has no down-migration mechanism; the
   manual-rollback recipe (`DROP TABLE publication_drift_state; DELETE
   FROM schema_migrations WHERE version = 9;`) is documented in
   `tasks.md` Phase 1 for operator use, identical pattern to
   Hito 11's 008 migration.

2. **`internal/publish/drift/` package**:
   - `Result{Status, ActualHash, Err}` where `Status` is one of the
     four enum values above.
   - `Checker.Check(ctx, target, expectedHash)`: `os.Stat` first
     (returns `target_missing`), then `os.Open` + `sha256.New()`
     streaming (returns `target_unreadable` on `os.Open` error after
     stat, `drifted` on hash mismatch, `ok` on match).
   - Read-only contract asserted by `contract_test.go` via
     `t.Cleanup`-trapped stat on the target before/after the call
     (mtime + size unchanged).

3. **`publication_drift_check` job**:
   - Uses `semantic.JobFunc` (Hito 11 runtime) + `semantic.Job` with
     `Intent = "drift"` (new constant), `Scope = "project"`,
     `RiskClass = "low"`.
   - Iterates `publications` rows where `status = 'published'` and
     `target_path` is set, computes `expected_hash` from
     `publications.source_hash`, calls `Checker.Check`, upserts into
     `publication_drift_state`.
   - Registered in `job_registry` with `Enabled = false` per Hito 11
     invariant. The Hito 3 `--watch` flip is deferred.

4. **Unified drift CLI/MCP**:
   - `cmd/royo-learn/experience.go` adds `runExperienceDrift(args)`:
     parses `--all-sources` (default true) and `--source=<value>`,
     delegates to the drift job via `jobs.Service.RunOne`.
   - JSON envelope shape:
     ```json
     {
       "sources": [
         { "source": "opencode",   "drifted": 3, "ok": 12, "missing": 1 },
         { "source": "claudecode", "drifted": 0, "ok":  7, "missing": 0 },
         { "source": "codex",      "drifted": 1, "ok":  4, "missing": 2 }
       ],
       "publications": [
         { "publication_id": "...", "source": "opencode",
           "target_path": "...", "status": "drifted",
           "expected_hash": "...", "actual_hash": "..." }
       ]
     }
     ```
     `excerpt`, `user_text`, `assistant_text`, and any
     `ExperienceEnvelope` field are **never** serialized.
   - `internal/mcp/experience.go` registers `experience_drift_status`
     calling the same handler as the CLI.

5. **Adapter parity**:
   - `claudecode/resolve_trace.go` lines 101–117: remove the
     `if excerpt != "" { result.Excerpt = excerpt; result.Advisory =
     true }` block. Replace with the same nil-out pattern as
     `opencode/resolve_trace.go` (`return ResolveTraceResult{Status:
     ...}, nil` without the excerpt).
   - Contract test in each of the three adapter packages asserts
     `result.Excerpt == ""` on `trace_source_changed` /
     `trace_source_unavailable` outcomes. Fixtures reused from Hito 10
     (the SEVERE trace-leak invariant from
     `hito10-codex-review-fixes.md` is preserved).
   - `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or
     disappears" gains one sentence: "All three adapters MUST return no
     excerpt on `trace_source_changed` or `trace_source_unavailable`."

6. **Release extras**:
   - `.goreleaser.yml`: add `sboms:` block under `archives:` with
     `formats: ['spdx-json']`. If GoReleaser v2.17.0 rejects the
     format, the gap is declared in `RELEASE.md` §"Known Limitations".
   - `RELEASE.md`: new file at repo root with the trigger table (PR
     merge → `ci.yml` quality matrix → `release.yml` via `workflow_run`
     → GoReleaser → artifact upload), required checks, tag creation
     procedure, install verification recipe, rollback recipe.
   - `CHANGELOG.md` backfill: read `git log --oneline` for Hitos 8–11
     PR merge SHAs, extract PR titles, move from `[Unreleased]` to
     `[0.8.0]` / `[0.9.0]` / `[0.10.0]` / `[0.11.0]` sections (the
     pre-v1.0.0 versioning scheme documented in
     `docs/26-IMPLEMENTATION-ROADMAP.md`).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/storage/migrations/009_publication_drift.sql` | New | Adds `publication_drift_state` table |
| `internal/storage/migrate.go` | Referenced | Forward-only runner; manual-rollback recipe in `tasks.md` |
| `internal/publish/drift/checker.go` | New | `Checker.Check(ctx, target, expectedHash) (Result, error)` with the four outcomes |
| `internal/publish/drift/contract_test.go` | New | Read-only contract: stat target before/after, assert mtime + size unchanged |
| `internal/publish/drift/sbom_test.go` (placeholder) | Not created | SBOM is YAML, not Go |
| `internal/experience/semantic/types.go` | Modified | Adds `JobIntentDrift = "drift"` constant |
| `internal/experience/jobs/service.go` | Referenced | Reuses `RunOne` from Hito 11 audit hook |
| `internal/experience/claudecode/resolve_trace.go` | Modified | Removes advisory excerpt branch on source mismatch (Deliverable 3) |
| `internal/experience/claudecode/resolve_trace_test.go` | Modified | Adds parity test `TestResolveTrace_SourceChanged_OmitsExcerpt` |
| `internal/experience/opencode/resolve_trace_test.go` | Modified | Adds parity test (already conforms; test asserts the contract) |
| `internal/experience/codex/resolve_trace_test.go` | Modified | Adds parity test (already conforms; test asserts the contract) |
| `cmd/royo-learn/experience.go` | Modified | Adds `runExperienceDrift` subcommand with `--all-sources` / `--source=` flags |
| `cmd/royo-learn/drift_test.go` | New | CLI golden-fixture test for the JSON envelope shape |
| `internal/mcp/experience.go` | Modified | Registers `experience_drift_status` tool, calls same handler as CLI |
| `internal/mcp/experience_test.go` | Modified | Adds MCP tool schema test + zero-PII assertion |
| `.goreleaser.yml` | Modified | Adds `sboms:` block under `archives:` with `formats: ['spdx-json']` |
| `RELEASE.md` | New | Consolidated release runbook; linked from `docs/15-OPERATIONS.md` |
| `CHANGELOG.md` | Modified (additive only) | Backfill of Hito 8/9/10/11 entries from `git log` PR titles; demote `v1.0.0` ⏳ |
| `docs/15-OPERATIONS.md` | Modified (additive only) | Adds "Release runbook" section linking to `RELEASE.md` |
| `docs/11-INSTALLATION.md` | Referenced | SBOM entry already listed; `.goreleaser.yml` now produces it |
| `docs/22-ADAPTER-CONTRACT.md` | Modified (additive only) | Scenario "Source changes or disappears" gains one-paragraph tightening |
| `docs/24-EXPERIENCE-THREAT-MODEL.md` | Referenced | Drift outcomes extend the threat model but do not change §6 audit invariant |
| `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 Hito 12 | Modified (additive only) | Adds acceptance rows for the four drift outcomes + unified CLI/MCP envelope + adapter parity |
| `docs/26-IMPLEMENTATION-ROADMAP.md` §5 Hito 12 row | Modified | Marks the row as "in flight" with link to this proposal |
| `openspec/changes/hito12-drift-release/specs/experience-adapters/` | Modified | Delta spec for the parity contract change |
| `openspec/changes/hito12-drift-release/specs/publication-drift-check/` | New | Full spec for the new capability |
| `openspec/changes/hito12-drift-release/specs/drift-cli-mcp/` | New | Full spec for the new capability |
| `openspec/changes/hito12-drift-release/specs/release-extras/` | New | Full spec for the new capability |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| **R1**: Drift job mistakenly checks `publications.status = 'in_progress'`, reading a half-written target and producing a false `drifted` outcome | Medium | Hard-coded gate `Status='published'` in the `publication_drift_check` `JobFunc` body; covered by `TestPublicationDriftCheck_SkipsInProgress` that inserts a row with `status='in_progress'` and asserts the `publication_drift_state` row count is unchanged |
| **R2**: `Checker.Check` accidentally writes to the target (e.g. via `os.Chtimes`, log rotation, or temp-file cleanup) | Low | `contract_test.go` snapshots `targetInfo.Mode()`, `ModTime()`, `Size()` before and after `Check` and asserts byte-identical; the implementation only uses `os.Stat` + `sha256.New().Write` from `os.Open` reader; lint rule (`go vet` + `grep`-based pre-commit) fails if any `os.WriteFile` / `os.Create` / `ioutil.WriteFile` call is added to `internal/publish/drift/` |
| **R3**: Windows Defender / macOS Gatekeeper quarantine the drift test artifacts on first write, breaking cleanup | Low | All new tests use `testutil.TempDir(t)` + `t.Cleanup` (the Hito 5/10 pattern that already passes Windows CI); `tests/integration/drift_test.go` documents the cleanup recipe |
| **R4**: GoReleaser v2.17.0 does not support `sboms.formats: ['spdx-json']`, causing `goreleaser release --snapshot --clean` to fail | Medium | The format is supported since GoReleaser v1.18; v2.17.0 supports `spdx-json` natively. If the snapshot build fails, the gap is declared in `RELEASE.md` §"Known Limitations" and the SBOM row in `docs/11` is updated to "planned". CI `release.yml` runs `--snapshot --clean` in dry-run mode during `gentle-ai review validate --gate pre-push` |
| **R5**: Cross-adapter parity test depends on Hito 10 fixtures (real OpenCode/Claude Code/Codex session directories); the fixtures drift between Hitos | Low | All three adapter parity tests reuse the same `testdata/` fixtures shipped in Hito 10 (the fixtures are content-stable per `hito10-codex-review-fixes.md`); no new fixture creation in Hito 12 |
| **R6**: Audit-event emission from `publication_drift_check` duplicates the existing `experience_*` events | Low | The job reuses `jobs.Service.RunOne` (Hito 11 audit hook) which emits only the four `job_*` lifecycle events; drift-specific data lives in `publication_drift_state`, not in `audit_events` |
| **R7**: Backfilled CHANGELOG entries misrepresent PR scope (git log PR title vs. PR body) | Low | The backfill uses PR titles only; each entry gains a footnote `[^pr-N]: #N` linking to the GitHub PR; if a PR title is misleading, the operator amends the entry post-merge |
| **R8**: Unified drift CLI/MCP leaks PII via `target_path` field (which contains user names on macOS/Linux e.g. `/home/alice/.claude/...`) | Low | JSON envelope redacts `target_path` to basename (`filepath.Base(target_path)`); the full path lives only in the audit table; `TestDriftCLI_NoPIIInOutput` greps the rendered JSON for `/home/`, `/Users/`, `C:\Users\` and asserts zero matches |
| **R9**: `goreleaser-prerelease: auto` causes v1.0.0 to ship as `v1.0.0-pre.N` if tagged before all extras land | Low | Out of scope: this change does NOT tag v1.0.0; the `RELEASE.md` runbook explicitly requires all four deliverables merged AND the CHANGELOG backfilled before tagging, breaking the prerelease path |
| **R10**: `claudecode` parity change breaks a downstream consumer that depended on the advisory `result.Advisory` field | Low | `result.Advisory` is removed (not just set to `false`); `docs/22` lists it as never-public and the field has been internal-only since Hito 10; no contract test in `hito10-codex` asserts its presence |

## Rollback Plan

The change is reversible in four steps:

1. **Revert migration 009**: `git revert` removes
   `009_publication_drift.sql`. The runner is forward-only; the
   operator applies the manual rollback recipe documented in
   `tasks.md` Phase 1 (`DROP TABLE publication_drift_state; DELETE FROM
   schema_migrations WHERE version = 9;`).
2. **Drop the new package**: `git rm -r internal/publish/drift/`
   removes the package and its test files. No other file imports it
   after step 3.
3. **Restore the per-source CLI dispatchers**: revert
   `cmd/royo-learn/experience.go` to its Hito 11 shape
   (`runExperienceUnified` for scan only, no `runExperienceDrift`);
   revert `internal/mcp/experience.go` to remove the
   `experience_drift_status` tool registration.
4. **Restore claudecode advisory excerpt branch**: revert
   `internal/experience/claudecode/resolve_trace.go` lines 101–117 to
   their pre-Hito-12 shape (kept in `git log` history). `docs/22`
   tightening is reverted to its pre-Hito-12 wording.

For the release extras (`RELEASE.md`, SBOM in `.goreleaser.yml`,
CHANGELOG backfill), rollback is purely additive — `git revert` each
file individually; no migration or schema is involved.

**Backwards-compatibility call-out**: the unified drift CLI is
**additive** (`experience drift` is a new subcommand, not a replacement
of `experience scan`). The MCP tool `experience_drift_status` is also
additive. The claudecode parity change removes an internal-only field
(`result.Advisory`) that is not part of `docs/22`. No public surface
breaks.

## Dependencies

- `docs/22-ADAPTER-CONTRACT.md` §1 (frozen, `ExperienceAdapter`)
- `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or disappears"
  (modified in this change)
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit invariant; reused)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2/§4 (coverage gate; new
  rows added)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 Hito 12 row
- `openspec/changes/hito11-semantic/` (provides `semantic.JobFunc`
  runtime + audit hook + `JobIntent` constants)
- `openspec/changes/hito10-codex/` (provides `codex` adapter package
  + the cross-adapter fixtures reused in Deliverable 3)
- `internal/experience/jobs.Service.RunOne` (Hito 11 audit hook;
  reused by `publication_drift_check`)
- `internal/storage.RecordEventTx` (existing audit sink; no schema
  change)
- `.goreleaser.yml` v2.17.0 (SBOM format support verified pre-merge)
- `docs/11-INSTALLATION.md` line 124 (SBOM entry)
- `docs/15-OPERATIONS.md` (target for `RELEASE.md` cross-link)

## Success Criteria

- [ ] `go build ./cmd/royo-learn` passes on Windows/Linux/macOS.
- [ ] `go test -race ./...` passes on linux/amd64.
- [ ] `go vet ./...` passes; `gofmt` clean.
- [ ] `internal/publish/drift/` has ≥ 90% test coverage
      (`docs/25` §4 row).
- [ ] All four publication-drift outcomes (`ok`, `drifted`,
      `target_missing`, `target_unreadable`) are covered by integration
      tests using a real filesystem (`testutil.TempDir`).
- [ ] `contract_test` asserts the target file's `Mode`, `ModTime`, and
      `Size` are unchanged before/after `Checker.Check`.
- [ ] `TestPublicationDriftCheck_SkipsInProgress` inserts a row with
      `status='in_progress'` and asserts `publication_drift_state` row
      count is unchanged (gate `Status='published'`).
- [ ] JSON envelope from `experience drift --all-sources` has stable
      `sources: [...]` and `publications: [...]` sections, golden
      fixture test passes.
- [ ] `TestDriftCLI_NoPIIInOutput` asserts zero `/home/`, `/Users/`,
      `C:\Users\` substrings in the rendered JSON.
- [ ] MCP tool `experience_drift_status` registered in
      `internal/mcp/`, schema test passes.
- [ ] Each of the three adapter packages has
      `TestResolveTrace_SourceChanged_OmitsExcerpt` asserting
      `result.Excerpt == ""` on the two drift outcomes.
- [ ] `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or
      disappears" tightened with one paragraph forbidding excerpt on
      drift across all three adapters.
- [ ] `.goreleaser.yml` `goreleaser release --snapshot --clean` produces
      `*.spdx.json` SBOM alongside the tar.gz/zip artifacts. If format
      is unsupported, the gap is declared in `RELEASE.md` §"Known
      Limitations" and the snapshot test asserts the alternative
      format.
- [ ] `RELEASE.md` exists at repo root, covers: trigger table,
      required CI checks, tag creation, install verification, rollback
      recipe; linked from `docs/15-OPERATIONS.md` §"Release runbook".
- [ ] `CHANGELOG.md` `[Unreleased]` no longer contains Hito 8/9/10/11
      entries; `v1.0.0` ⏳ marker demoted to a clear "no tag yet"
      section referencing `RELEASE.md`.
- [ ] `gentle-ai review finalize` accepts the change with at most one
      bounded correction round within the 200-line correction budget
      (`min(200, ceil(710/2)) = 200` — full Hito 12 diff fits).
- [ ] `docs/14-ACCEPTANCE-CRITERIA.md` §E has new acceptance rows for
      the four drift outcomes, the unified CLI/MCP envelope, the
      adapter parity tests, the SBOM artifact, and the CHANGELOG
      backfill; `docs/25` §2 Hito 12 has matching rows.