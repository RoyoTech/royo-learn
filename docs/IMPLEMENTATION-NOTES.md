# Implementation Notes

## Bootstrap Lifecycle Decision

### Checkpoint-versus-receipt conflict

The original bootstrap instructions required an initial Git checkpoint before
implementation. The review lifecycle requires a valid, content-bound receipt
before a commit may be created. In an empty repository, a standalone baseline
checkpoint would therefore require a receipt for a tree that cannot yet be
committed, creating a circular ordering constraint.

### Approved ordering

The approved resolution preserves the lifecycle gate without creating a
standalone pre-code checkpoint:

1. Record this conflict and decision before T00 product writes.
2. Apply and verify the T00 bootstrap scope as one uncommitted work unit.
3. Stage every non-ignored specification-baseline and T00 path, then record the
   combined tree identity with `git write-tree`.
4. The parent lifecycle controller performs the post-apply review and obtains a
   content-bound native receipt for that exact staged tree.
5. The parent validates the receipt. A separately authorized action may create
   the first commit only after the native validator returns `allow`.

No review, receipt validation, or commit is performed by `sdd-apply`. Any
change to the staged tree invalidates a receipt and requires a new review.

## Dependency Provenance

T00 resolves and retains direct dependencies only for compile-validated future
ownership. Exact versions and validation evidence are appended after module
resolution. No MCP server or SQLite database is started by this bootstrap.

### Resolved dependencies

- `github.com/modelcontextprotocol/go-sdk v1.6.1` is the official Go MCP SDK.
  `internal/mcpserver/anchor.go` retains a compile-only reference to
  `mcp.NewServer`; T00 does not construct or run a server.
- `modernc.org/sqlite v1.53.0` is the CGO-free `database/sql` driver.
  `internal/storage/driver.go` blank-imports it only to retain driver ownership;
  T00 does not call `sql.Open` or inspect database state.

The resolved dependencies require the `go 1.25.0` module language version.
The T00 environment uses Go 1.26.5, which satisfies the project requirement
for Go 1.25 or newer.

### CI provenance

The base workflow pins `actions/checkout` v4.2.2 to
`11bd71901bbe5b1630ceea73d27597364c9af683` and `actions/setup-go` v5.0.2
to `0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32`. It runs formatting, tidy
diff, verification, tests, vet, and build checks on Linux, Windows, and macOS.

### T00 quality evidence

The following commands completed with exit status 0 on Windows/amd64 with Go
1.26.5:

```text
go fmt ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go mod verify
go test ./...
go vet ./...
go build -o <temporary-path>/royo-learn-windows-amd64.exe ./cmd/royo-learn
go test -race ./...
GOOS=linux GOARCH=amd64 go build -o <temporary-path>/royo-learn-linux-amd64 ./cmd/royo-learn
GOOS=darwin GOARCH=arm64 go build -o <temporary-path>/royo-learn-darwin-arm64 ./cmd/royo-learn
<temporary-path>/royo-learn-windows-amd64.exe version --json
```

The subprocess test covers the built-binary stdout/stderr contract. The direct
runtime command emitted one valid JSON object and no diagnostic output.

`make quality` could not run in this environment because `make` is not
installed (`/usr/bin/bash: line 1: make: command not found`). Its individual,
equivalent commands were executed successfully above; CI runs the same checks
on supported GitHub-hosted runners.

## T01 — Config loader dependency and design notes

### Resolved dependencies

- `gopkg.in/yaml.v3 v3.0.1` is the direct YAML parser for configuration files.
  It is used by `internal/config` to decode `.royo-learn/config.yaml` and the
  user config file with strict field matching (`KnownFields(true)`), rejection
  of YAML aliases, and a 1 MiB size limit. This dependency was previously
  available only as an indirect requirement of the MCP SDK; T01 promotes it to
  a direct dependency because config loading is a core runtime responsibility.

### Design choices

- Config precedence is implemented as compiled defaults < user config < project
  config. Explicit CLI flags and environment variables are intentionally left
  for callers to apply after `Load` returns, keeping the loader free of flag
  package dependencies in Task 1.
- The user config directory uses `os.UserConfigDir()` and resolves to
  `<UserConfigDir>/royo-learn/config.yaml` on all platforms.
- Validation rejects unknown YAML keys, YAML aliases, and config files larger
  than 1 MiB. Path validation checks `project_root` and `shared_root` against
  an explicit list of trusted roots and returns typed `*config.Error` values
  with stable codes (`invalid_config`, `path_outside_root`).

## Handoff — T01 Task 1 complete, PR #1 open

### Current state

- T01 Task 1 is committed on local `master` as `7af28fb`.
- The commit is pushed to `origin/master` on `RoyoTech/royo-learn`.
- Branch `main` exists on the remote at the T00 commit (`f172143`).
- PR #1 is open: https://github.com/RoyoTech/royo-learn/pull/1 (master → main).
- Native review receipt lineage `t01-config-project-v2` is approved and both
  `pre-commit` and `pre-PR` gates returned `allow`.

### How to resume

1. If PR #1 was merged: pull `main`, create a new branch from `main`, and start
   T01 Task 2.
2. If PR #1 is still open: continue from the current `master` branch for T01
   Task 2, then rebase or retarget before the next PR.
3. Next work: T01 Task 2 — project resolver (`internal/project`), integrated
   with `internal/config` and exposed through the `doctor` and CLI commands.

### Operational notes

- `.gitattributes` sets `* text=auto` to avoid CRLF/LF noise on Windows.
- `.gitignore` ignores build artifacts (`royo-learn`, `*.exe`).
- `openspec/changes/t01-config-project-v2/reviews/` contains non-authoritative
  receipt mirrors; the authoritative store is under
  `.git/gentle-ai/review-transactions/v1/t01-config-project-v2/`.

## T01 Task 2 — Project resolver, key derivation, and path security

### Branch

`feat/t01-task2-project-resolver`, started from `main`.

### Resolved dependencies

No new external dependencies. Uses standard library only:
- `crypto/sha256` for path hashing
- `os/exec` for Git command interaction
- `path/filepath` for cross-platform path handling
- `runtime` for OS detection (case-insensitive filesystem check)
- `log/slog` for structured logging (optional, via `WithLogger`)

### Design choices

- **Error type**: Package defines its own `Error` type (Code, Message, Err) matching
  the pattern from `internal/config`. Error codes are `project_not_found`,
  `ambiguous_project`, `path_outside_root`, `symlink_escape`, `protected_path`.
- **Path security**: `Canonicalize()` rejects UNC (`\\`), verbatim (`\\?\`), and
  device (`\\.\`) paths before any filesystem operation. Symlinks are resolved
  via `filepath.EvalSymlinks`. Non-existent paths fall back to `filepath.Clean`
  on the absolute path.
- **Case-insensitive comparison**: `IsInsideRoot` normalizes paths to lowercase
  on Windows and macOS (`runtime.GOOS` check). Linux comparisons are
  case-sensitive.
- **Key derivation**: Prefers Git remote URL parsing (detects both HTTPS and SSH
  formats) with relative path appended for monorepo sub-projects. Falls back to
  SHA-256 digest (first 12 hex chars) when no Git metadata exists.
- **Project resolution precedence**: ExplicitRoot > CWD marker walk-up > CWD Git
  root > MCPRoot. The walk-up algorithm checks for `.royo-learn/config.yaml` at
  each ancestor directory. Ambiguity is detected by checking sibling directories
  under the common parent.
- **Ambiguity detection**: When a project marker is found at directory D, all
  sibling directories under `filepath.Dir(D)` are scanned for their own
  `.royo-learn/config.yaml`. Two or more markers in siblings returns
  `ambiguous_project`.

### Files

| File | Lines | Purpose |
|------|-------|---------|
| `internal/project/project.go` | 296 | Resolver, Project struct, ResolveRequest, options pattern, Error type |
| `internal/project/key.go` | 114 | Git-based key derivation with SHA-256 fallback |
| `internal/project/path.go` | 126 | Canonicalize, IsInsideRoot, IsProtectedPath, protected path constants |
| `internal/project/project_test.go` | 458 | Table-driven tests covering all acceptance criteria |

### Testing

- 15 test functions, all passing on Windows/amd64 with Go 1.26.5.
- Tests requiring Git (`gitAvailable()`) skip gracefully when git is not installed.
- Symlink tests skip when the platform doesn't support symlink creation.
- Cross-platform path handling tested with `filepath` and `t.TempDir()`.
- `go test ./internal/project/...` → PASS
- `go vet ./internal/project/...` → PASS
- `go test ./...` → PASS (all packages)

### TDD evidence

Strict TDD cycle followed: tests written first → build failed (RED) → production
code implemented → all tests pass (GREEN) → refactoring to remove dead code,
simplify error handling, remove unused functions → all tests still pass.
- **2026-07-11 rebuild scope**: Batch T02 rebuild repairs FTS transactionally from canonical SQLite tables; the broader `rebuild-index` CLI reconstruction from Markdown records remains deferred until a record parser exists.

## P2 — Explicit skill area in curate_learning

### Persistence decision (no migration)

`Destination` is persisted as a JSON blob in the `curations.destination_json`
column via `marshalAny(c.Destination)` (internal/storage/repo_curations.go) and
deserialized via `json.Unmarshal` in `unmarshalDestination`. Adding the
`Area string json:"area,omitempty"` field to `domain.Destination` therefore
requires NO SQL migration: the field serializes/deserializes automatically.
Existing curations without the field unmarshal to `Area: ""` (omitempty →
automatic-derivation fallback), preserving backward compatibility.

### Design

- `domain.ValidateExplicitArea` + `domain.SanitizeSkillArea` (internal/domain/area.go)
  centralize area sanitization (alphanumeric, dash, underscore; spaces→dash;
  lowercase) and validation (max 64 chars; non-empty after sanitize). Shared by
  curate and publish with no new cross-peer dependency.
- `curate.CurateInput.Area` flows through `deriveDestination` into
  `Destination.Area` for skill decisions only.
- `publish.ResolveSkillArea(learning, explicitArea)` returns the explicit area
  when present (re-validating defensively) or falls back to `SkillArea(learning)`
  (the deterministic sorted-terms derivation, unchanged).
- `TargetContext.Area` carries the resolved area into the content builders so
  preview and publish use the SAME area for both path and frontmatter.
- Multi-target (child skill + index + AGENTS.md) activates when the curator set
  an explicit area OR the stored path is generic/matches the derived name. The
  explicit area NEVER falls into the single-target legacy path.
- Preview path-doubling fix: preview previously set `dest.Path = autoName +
  "/SKILL.md"` while publish set `dest.Path = autoName`; since
  `ResolveSkillPublishTargets` appends "SKILL.md" itself, preview doubled the
  path to `autoName/SKILL.md/SKILL.md`. Unified to `autoName` in both so
  preview == publish.
