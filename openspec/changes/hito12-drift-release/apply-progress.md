# Apply Progress — Hito 12 Slice 1 (Drift Foundation)

**Branch**: `feat/hito12-drift-core`
**Base**: `main` @ `3f1b112`
**Slice**: 1 of 3 (stacked-to-main chain strategy)

## Commits Applied (T12.6 → T12.1 chronological)

| Hash | Task | Description |
|------|------|-------------|
| `85df91b` | T12.6 | `feat(semantic): add JobIntentDrift constant for publication drift checker` |
| `71676f7` | T12.1 | `feat(publish): add migration 009 + drift package types` |
| `c916945` | T12.3 | `feat(publish): add Checker with four outcomes` |
| `df9606e` | T12.4 | `test(publish): add read-only contract + grep guard for drift package` |
| `fdd7c3e` | T12.2 | `feat(publish): add drift Repository with RecordDrift + ListDrift` |
| `8c9289f` | T12.5 | `feat(publish): add publication_drift_check job + gate in JobFunc body` |

## Files Created

```
internal/publish/drift/
├── checker.go           # Result, Status enum, Checker.Check with 4 outcomes
├── checker_test.go      # Unit tests per outcome + edge cases
├── contract_test.go     # Read-only enforcement (stat before/after)
├── jobs.go              # Job() *semantic.Job accessor with gate in JobFunc body
├── jobs_test.go         # TestPublicationDriftCheck_SkipsInProgress
├── repository.go        # RecordDrift, ListDrift, GetDriftByPublication
└── repository_test.go   # Integration tests with SQLite real
```

```
internal/storage/migrations/
└── 009_publication_drift.sql  # publication_drift_state table + CHECK constraint
```

```
internal/experience/semantic/
└── types.go             # Modified: added JobIntentDrift = "drift"
```

## Verification

| Gate | Result |
|------|--------|
| `gofmt -l internal/publish/drift/` | clean |
| `go vet ./internal/publish/drift/...` | PASS |
| `go test ./internal/publish/drift/...` | PASS (1.021s) |
| Coverage `internal/publish/drift/` | **91.3%** (target ≥ 90%) |

## Acceptance Criteria Resolved

- [x] Migration 009 with `publication_drift_state` table + CHECK constraint (REQ-PDC-1, REQ-PDC-2).
- [x] `Checker.Check(ctx, target, expectedHash)` with 4 outcomes: `ok`, `drifted`, `target_missing`, `target_unreadable` (REQ-PDC-4).
- [x] Read-only enforcement via `contract_test.go` snapshot of stat before/after (REQ-PDC-5).
- [x] Job `publication_drift_check` registered with `intent="drift"`, `scope="project"`, `risk_class="low"` (REQ-PDC-6).
- [x] Gate `Status='published'` encoded in `JobFunc` body, not just SQL WHERE (REQ-PDC-3 — D1 decision in design.md).
- [x] `TestPublicationDriftCheck_SkipsInProgress` covers the gate.
- [x] `JobIntentDrift` constant added to `semantic.JobIntent` enum + `IsValidIntent` switch (REQ-PDC-7).

## Open Items / Follow-ups for Slice 2 or Operator

1. **Cleanup commit pending** (uncommitted in working tree):
   - `checker.go`: removed deprecated `IsReadOnly()` stub.
   - `checker_test.go`: added `TestChecker_IoCopyFailureYieldsUnreadable` (35 lines).
   - `repository_test.go`: added `TestNewRepository_NilNowFn` (38 lines).
   - These add 91.3% coverage with 2 triangulation tests. Operator can commit
     as `test(publish): close Slice 1 with cleanup and triangulate tests`.

2. **Slice 2 dependencies** (next PR, stacked-to-main):
   - T12.7: CLI `experience drift`
   - T12.8: PII redaction test + `filepath.Base()` in handler
   - T12.9: MCP tool `experience_drift_status` (admin profile)
   - T12.10: `claudecode/resolve_trace.go` parity fix (lines 100-119)
   - T12.11: parity tests in `opencode` and `codex` packages
   - T12.12: `docs/22-ADAPTER-CONTRACT.md` Scenario tightening

3. **Slice 3 dependencies** (third PR, stacked-to-main):
   - T12.13: SBOM in `.goreleaser.yml`
   - T12.14: `RELEASE.md` runbook
   - T12.15: link from `docs/15-OPERATIONS.md`
   - T12.16: `CHANGELOG.md` backfill Hitos 8/9/10/11 + demote v1.0.0 ⏳
   - T12.17–T12.20: final verification, coverage, cross-build, docs

## Risks Surfaced During Apply

- **R-A1 (low)**: Sub-agent stalled twice during apply. Slice 1 was completed
  despite stalls. Slice 2/3 may need smaller delegation units (one task per
  sub-agent invocation) to avoid context overflow.
- **R-A2 (low)**: Lifecycle commit detection blocked cleanup commit. Operator
  can commit it directly. Not a code issue, just an orchestration constraint.

## Next Step

`verify` for Slice 1, then proceed to Slice 2.
