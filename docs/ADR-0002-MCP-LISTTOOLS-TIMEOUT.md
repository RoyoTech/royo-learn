# ADR-0002 — MCP ListTools timeout requires bounded investigation before Hito 2

- **Status:** Proposed
- **Date:** 2026-07-22
- **Decision context:** Repeated `internal/mcpserver` test failure observed during the
  Hito 1 closure gate on `feat/experience-hito1-1d`. Causality was not
  independently proven in the correction window; this ADR records the evidence
  and bounds the follow-up so the issue is not silently masked.
- **Scope:** test observability only. No production behavior, retry policy,
  timeout values, MCP profile, migration, or public CLI/MCP contract is
  changed by this ADR or by the correction that introduces it.

## 1. Context

`docs/IMPLEMENTATION-NOTES.md` (line 466) already records that the MCP suite
occasionally exceeds the client deadline and is **not** a Windows Defender /
SQLite cleanup flake. The Hito 1 delivered review lineage
`experience-hito1-1d-delivered-v1` escalated with a concrete reproduction that
re-confirms this orthogonal failure mode.

Observed evidence (Hito 1 closure gate, `feat/experience-hito1-1d`):

- Failing test: `internal/mcpserver.TestMCP_Rollback_NotServedInReadOrAgent`
  (file: `internal/mcpserver/occurrence_status_rollback_test.go:231`).
- Error: `ListTools: context deadline exceeded` raised from
  `ts.session.ListTools(ctx, nil)` inside `toolServed`
  (`internal/mcpserver/occurrence_status_rollback_test.go:264`).
- Invocation: `go test -race -count=2 ./...`.
- Failure is intermittent and orthogonal to the Windows cleanup flake
  mitigated by `internal/testutil.TempDir` and the per-test `time.Sleep`
  Windows branches. The failover path crosses an `mcp` SDK transport
  boundary, not a `t.TempDir()` cleanup path.

The correction window for Hito 1 explicitly excludes fixes that touch
`internal/mcpserver/`. Per the scope of the correction, this ADR proposes
the investigation boundary and acceptance criteria for the follow-up; it
does not authorize any production code change.

## 2. Decision

**Status: Proposed.** No production MCP behavior, retry masking, or timeout
relaxation is introduced in this scope. The follow-up before Hito 2 is
bounded as stated in Section 4.

### 2.1 Why this is not the Windows cleanup flake

The Windows cleanup flake mitigated by `internal/testutil.TempDir` and the
existing `setupApprovedLearning` sleep is a **filesystem lifecycle** issue:
Windows Defender holds SQLite WAL/SHM file handles for tens of milliseconds
after `db.Close()`, and `t.TempDir` (or any `os.RemoveAll` consumer) must
retry. The MCP timeout is a **transport deadline** issue: the test calls
`session.ListTools` with a 10 s `context.WithTimeout` (see
`internal/mcpserver/occurrence_status_rollback_test.go:233`), and the call
does not return before the deadline elapses. Different cause, different
mitigation, different file boundary. Conflating them would mask the real
defect.

The correction window therefore:

- Replaces `t.TempDir()` with `testutil.TempDir(t)` in
  `cmd/royo-learn/experience_test.go` so the new CLI test amortizes
  Windows cleanup the same way the rest of the suite does.
- Updates the misleading comment in `cmd/royo-learn/main_test.go` so the
  documented cleanup behavior matches the code (Windows-only sleep; Unix
  has no sleep).
- Records the MCP timeout here, with status **Proposed**, and does not
  modify any file under `internal/mcpserver/`.

### 2.2 What is explicitly NOT in this scope

- No change to `internal/mcpserver` source, fixtures, or test wiring.
- No retry, backoff, or `recover` wrapping around `session.ListTools`.
- No relaxation of the 10 s deadline in
  `TestMCP_Rollback_NotServedInReadOrAgent`.
- No change to MCP profiles (`read`, `agent`, `write`), tool registration,
  or transport configuration.
- No change to public CLI/MCP contracts, error envelopes, or migration
  sequencing.
- No dependency additions.

## 3. Evidence observed

Reproducible command (Windows, Go 1.26.5, `feat/experience-hito1-1d`):

```
go test -race -count=2 ./...
```

Observed failure example:

```
internal/mcpserver/occurrence_status_rollback_test.go:266:
  ListTools: context deadline exceeded
  FAIL: TestMCP_Rollback_NotServedInReadOrAgent
```

The failure occurs while verifying that `learning_rollback` is not served
in the `read` or `agent` profiles. The call that times out is the
`tools/list` enumeration preceding the assertion; the rest of the test logic
is not reached on the failing run.

This evidence is consistent with the `docs/IMPLEMENTATION-NOTES.md`
observation that the MCP suite "ocasionalmente excede el deadline del
cliente" and that the failure is orthogonal to the Windows cleanup flake.

## 4. Follow-up boundary (before Hito 2)

A dedicated investigation is required before Hito 2 begins. The follow-up
must produce the following artifacts; no other scope is implied.

### 4.1 Bounded startup/teardown investigation

- Profile `internal/mcpserver`'s per-test startup and teardown cost on
  both Windows and Linux, comparing baseline (no MCP suite) and candidate
  (full suite). Capture wall-clock, allocation, and goroutine count.
- Identify whether the timeout is caused by transport startup, server
  handler registration, or a goroutine that does not unblock before the
  deadline.

### 4.2 Base vs. candidate reproduction

- Establish a base commit where `TestMCP_Rollback_NotServedInReadOrAgent`
  passes consistently under `go test -race -count=10 ./internal/mcpserver/`.
- Run the same command against the current `feat/experience-hito1-1d`
  HEAD to confirm the failure reproduces.
- Bisect between base and candidate to localize the contributing changes.

### 4.3 Focused acceptance evidence

The follow-up must report, at minimum:

- Failing test name and file:line.
- Reproduction command (exact flags, count, package selector).
- First failing iteration count out of N (e.g. "2 of 5").
- Profile (WSL2 vs. native Windows vs. Linux) and Go version.
- Whether the timeout is observed outside the test (a real `mcp-serve`
  subprocess reproduces the same `ListTools: context deadline exceeded`
  under hand-driven `stdio` traffic).
- A concrete remediation proposal, evaluated against the constraints in
  Section 2.1 (no production relaxation, no contract change).

### 4.4 Resolution entry

When the follow-up lands, update `docs/IMPLEMENTATION-NOTES.md` to remove
the "Merece una investigación/ADR antes de Hito 2" sentence and replace it
with a one-line pointer to the resolution commit or PR. Until then, the
sentence is correct as written.

## 5. Consequences

- Positive: the Hito 1 closure gate is not falsely claimed green, and the
  MCP timeout is documented as a separate, scoped investigation rather
  than absorbed into the Windows cleanup mitigations.
- Positive: the proposed follow-up has explicit acceptance criteria and
  reproduction steps, so the next agent can execute it without re-deriving
  the boundary.
- Negative: the MCP timeout remains unfixed until the follow-up closes.
  This is acceptable because (a) the failure is in a test, not in
  production traffic, and (b) attempting to fix it now would expand the
  correction window into `internal/mcpserver/` and out of the authorized
  scope.

## 6. References

- `docs/IMPLEMENTATION-NOTES.md`, line 466 (MCP timeout observation).
- `internal/mcpserver/occurrence_status_rollback_test.go:231` (failing test).
- `internal/mcpserver/occurrence_status_rollback_test.go:233` (10 s deadline).
- `internal/mcpserver/occurrence_status_rollback_test.go:264` (timeout site).
- `internal/testutil/cleanup.go` (Windows cleanup mitigation, distinct cause).
- `cmd/royo-learn/main_test.go` (Windows-only sleep in `setupApprovedLearning`).
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` (precedent for file-based ADR).
