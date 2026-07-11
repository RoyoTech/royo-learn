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
for Go 1.24 or newer.

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
