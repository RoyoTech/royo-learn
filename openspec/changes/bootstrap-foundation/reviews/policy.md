# Review Policy: bootstrap-foundation

## Scope

T00 Bootstrap Foundation: first executable vertical slice for `royo-learn`, including the Go module, `version --json` CLI command, stderr diagnostics, compile-only dependency anchors for MCP SDK and CGO-free SQLite, CI workflow, Makefile, and project documentation baseline.

## Risk tier

High: more than 400 authored changed lines in the initial combined baseline-plus-T00 snapshot.

## Initial lenses

- `review-risk`
- `review-resilience`
- `review-readability`
- `review-reliability`

## Correction budget

One correction transaction composed of atomic work units, followed by exactly one scoped fix-delta validation.

## Approval criteria

- All BLOCKER and CRITICAL findings are resolved or justified as out of scope.
- Scoped fix-delta validator returns `approve`.
- Final verification commands exit 0.

## Out of scope

Missing later-slice functionality (MCP server, SQLite operations, migrations, config, capture, approval, publish, recurrence, Engram integration) is expected and not a finding.
