# Delta for experience-adapters (Hito 12 — drift/release hardening)

> This delta MODIFIES the existing `experience-adapters` capability as
> expressed in `docs/22-ADAPTER-CONTRACT.md`. The canonical OpenSpec file
> `openspec/specs/experience-adapters/spec.md` does not yet exist for this
> project; the source of truth for the adapter contract lives in
> `docs/22-ADAPTER-CONTRACT.md` and the prior delta shape lives in
> `openspec/changes/hito11-semantic/specs/experience-adapters/spec.md`.
> This delta tightens the "Source changes or disappears" scenario so that
> all three adapters (`opencode`, `claudecode`, `codex`) share one
> identical policy on source drift: **no excerpt, typed error code**.
>
> Source contracts being tightened:
>
> - `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or disappears"
>   (currently defined inside the Codex section §11 of the contract).
> - `docs/22-ADAPTER-CONTRACT.md` §6 typed errors
>   (`trace_source_changed`, `trace_source_unavailable`,
>   `trace_event_unavailable`, `experience_source_not_found`).
> - `openspec/changes/hito11-semantic/specs/experience-adapters/spec.md`
>   (the prior delta this delta stacks on top of; the per-adapter
>   `Job()` accessor and taxonomy remain unchanged).

## MODIFIED Requirements

### Requirement: ResolveTrace omits excerpt on source drift across all three adapters

The system SHALL make `internal/experience/{opencode,claudecode,codex}/resolve_trace.go`
share one identical policy on source drift. When `ResolveTrace` detects
that the current source hash differs from `locator.SourceHash`, the
adapter MUST return `trace_source_changed` with `Excerpt == ""`,
`Advisory == false`, and no bounded excerpt of any kind (including
redacted content, suffix markers, or preview lines). When the source or
the requested turn is missing, the adapter MUST return
`trace_source_unavailable` with the same empty-excerpt guarantee. The
claudecode code path that emitted a bounded advisory excerpt on source
mismatch (`internal/experience/claudecode/resolve_trace.go:101–117`) is
removed; the resolver returns the same nil-out result shape as
`opencode/resolve_trace.go` and `codex/resolve_trace.go`. The typed
error codes continue to map to `docs/22-ADAPTER-CONTRACT.md` §6.
(Previously: only `opencode` and `codex` omitted the excerpt on drift;
`claudecode` returned a bounded advisory excerpt with `Advisory = true`,
breaking semantic parity across the three adapters on the same contract
scenario.)

#### Scenario: opencode returns no excerpt on `trace_source_changed`

- GIVEN a fixture source whose content has changed since
  `locator.SourceHash` was computed
- WHEN `opencode.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_changed"`
- AND `TraceResult.Excerpt == ""`
- AND `TraceResult.Advisory == false`
- AND the test `TestResolveTrace_SourceChanged_OmitsExcerpt` passes.

#### Scenario: opencode returns no excerpt on `trace_source_unavailable`

- GIVEN a fixture locator pointing at a source that has been deleted
- WHEN `opencode.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_unavailable"`
- AND `TraceResult.Excerpt == ""`
- AND no `os.Open` retry path produces a non-empty excerpt.

#### Scenario: claudecode returns no excerpt on `trace_source_changed`

- GIVEN a fixture source whose content has changed since
  `locator.SourceHash` was computed
- WHEN `claudecode.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_changed"`
- AND `TraceResult.Excerpt == ""`
- AND `TraceResult.Advisory == false`
- AND the advisory-excerpt branch (former lines 101–117) is unreachable.
- AND the test `TestResolveTrace_SourceChanged_OmitsExcerpt` passes.

#### Scenario: claudecode returns no excerpt on `trace_source_unavailable`

- GIVEN a fixture locator pointing at a source that has been deleted
- WHEN `claudecode.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_unavailable"`
- AND `TraceResult.Excerpt == ""`
- AND `TraceResult.Advisory == false`.

#### Scenario: codex returns no excerpt on `trace_source_changed`

- GIVEN a fixture rollout whose content has changed since
  `locator.SourceHash` was computed
- WHEN `codex.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_changed"`
- AND `TraceResult.Excerpt == ""`
- AND the test `TestResolveTrace_SourceChanged_OmitsExcerpt` passes.

#### Scenario: codex returns no excerpt on `trace_source_unavailable`

- GIVEN a fixture locator pointing at a rollout file that has been
  deleted
- WHEN `codex.Adapter.ResolveTrace` runs
- THEN the returned `TraceResult.Err == "trace_source_unavailable"`
- AND `TraceResult.Excerpt == ""`.

#### Scenario: All three adapters emit the same typed error codes on drift

- GIVEN one drift fixture per adapter (`opencode`, `claudecode`,
  `codex`) where the source has been mutated out-of-band
- WHEN the three `ResolveTrace` calls run in succession
- THEN all three return an error string drawn from the
  `{"trace_source_changed", "trace_source_unavailable"}` set
- AND no adapter returns a sentinel like `"source_changed_advisory"`
  or any custom non-typed variant.

### Requirement: `docs/22-ADAPTER-CONTRACT.md` Scenario "Source changes or disappears" is tightened to forbid excerpt on drift

The system SHALL modify `docs/22-ADAPTER-CONTRACT.md` so that the
Scenario "Source changes or disappears" is lifted out of the Codex-only
section §11 and rewritten as a cross-adapter requirement. The tightened
scenario MUST add the sentence: "All three adapters MUST return no
excerpt on `trace_source_changed` or `trace_source_unavailable`; the
typed error code is the only signal the caller receives." The
`resolve_trace_test.go` parity tests
(`TestResolveTrace_SourceChanged_OmitsExcerpt`) in each adapter package
are the executable assertion of the tightening; a future regression
that re-introduces the advisory excerpt branch must fail at least one
of the three parity tests.
(Previously: the scenario lived only inside `docs/22-ADAPTER-CONTRACT.md`
§11 (Codex) and the sentence "with no excerpt" was true for Codex but
implicit for OpenCode and contradicted by the Claude Code implementation
that returned a bounded advisory excerpt.)

#### Scenario: Contract document carries the cross-adapter tightening sentence

- GIVEN `docs/22-ADAPTER-CONTRACT.md`
- WHEN a static review searches for the literal
  "All three adapters MUST return no excerpt"
- THEN the sentence is present
- AND it sits adjacent to the "Source changes or disappears" scenario
  header.

#### Scenario: Parity test exists in every adapter package

- GIVEN `go test ./internal/experience/opencode/...`,
  `./internal/experience/claudecode/...`, and
  `./internal/experience/codex/...`
- WHEN each test binary runs
- THEN the test
  `TestResolveTrace_SourceChanged_OmitsExcerpt` exists in
  `internal/experience/opencode/resolve_trace_test.go`,
  `internal/experience/claudecode/resolve_trace_test.go`, and
  `internal/experience/codex/resolve_trace_test.go`
- AND all three pass on linux/amd64.

## ADDED Requirements

### Requirement: Advisory excerpt branch is removed from `claudecode/resolve_trace.go`

The system SHALL remove the code block in
`internal/experience/claudecode/resolve_trace.go` lines 101–117 that
returned a bounded excerpt together with `Advisory: true` on source
mismatch. The replacement MUST follow the same nil-out result shape used
by `opencode/resolve_trace.go` and `codex/resolve_trace.go`. The
removal is mandatory; the field `TraceResult.Advisory` remains reserved
for future use but MUST be `false` for every `claudecode` outcome in
this change. No call site outside `claudecode/resolve_trace_test.go`
asserts the presence of the field; the Hito 10 SEVERE trace-leak
invariant (`hito10-codex-review-fixes.md`,
`docs/24-EXPERIENCE-THREAT-MODEL.md` §6) is preserved.

#### Scenario: Static review confirms the advisory branch is gone

- GIVEN `internal/experience/claudecode/resolve_trace.go`
- WHEN `grep -n "Advisory" internal/experience/claudecode/resolve_trace.go`
  runs
- THEN the only matches are field assignments equal to `false`
- AND no match sets `Advisory: true`.

#### Scenario: Hito 10 SEVERE invariant still holds

- GIVEN the Hito 10 security invariant
  ("`result.Advisory` is never-public and `excerpt` is bounded")
- WHEN `internal/experience/claudecode/resolve_trace_test.go` runs the
  leak-canary assertions from Hito 10 fixtures
- THEN all existing assertions pass unchanged.

## References

- `docs/22-ADAPTER-CONTRACT.md` §6 (typed errors), §11 Scenario
  "Source changes or disappears" (tightened)
- `docs/24-EXPERIENCE-THREAT-MODEL.md` §6 (audit + leak invariant)
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` §2 (Hito 12 acceptance row)
- `docs/26-IMPLEMENTATION-ROADMAP.md` §5 (Hito 12 row)
- `openspec/changes/hito11-semantic/specs/experience-adapters/spec.md`
  (prior delta this change stacks on)
- `openspec/changes/hito10-codex/specs/experience-adapters/spec.md`
  (Codex parity reference)
- `openspec/changes/hito10-codex-review-fixes.md` (SEVERE trace-leak
  invariant)
