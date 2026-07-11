# Review Policy: t01-config-project

## Scope

T01 Task 1 of Agent Royo Learn: add the `internal/config` package with compiled defaults, user/project config loading, precedence, validation, path security, symlink resolution, and YAML hardening.

## Risk tier

High: config loading touches path security, symlink escape, UNC/device paths, and data-bearing directory configuration.

## Initial lenses

- `review-risk`
- `review-resilience`
- `review-readability`
- `review-reliability`

## Correction budget

One correction transaction composed of atomic work units, followed by exactly one scoped fix-delta validation.

## Approval criteria

- All BLOCKER and CRITICAL findings resolved.
- Scoped fix-delta validator returns `approve`.
- `go test ./...`, `go vet ./...`, and cross-builds exit 0.

## Out of scope

MCP server, SQLite operations, Engram integration, Gentle-AI integration, full `doctor` checks beyond project resolution, and `internal/project` resolver are expected to be missing and not findings for this slice.
