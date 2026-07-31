# Spec: experience-cli-collapse

## Purpose

Collapse the three per-source `experience <source> scan` subcommands
(`opencode`, `claudecode`, `codex`) into one unified
`experience scan --source=<opencode|claudecode|codex>` subcommand. The
unified subcommand MUST validate the `--source` value against
`domain.ExperienceSource` constants, MUST preserve the JSON envelope
shape of the previous per-source subcommand for parity, and MUST print a
deprecation note for the legacy form when run with the experimental
feature flag off. This change is documented in `docs/04-CLI-SPEC.md`
(additive only) and aligned with `docs/22` §9.

## Requirements

### Requirement: Unified `experience scan --source=<value>` subcommand

The system SHALL expose a single subcommand
`royo-learn experience scan --source=<opencode|claudecode|codex>`
implemented in `cmd/royo-learn/experience.go` as
`runExperienceUnified(args)`. The subcommand MUST dispatch to the
selected adapter's `Job()` accessor and MUST run exactly one end-to-end
scan with the same JSON envelope shape as the previous per-source
subcommand. `--source` is a required flag.

#### Scenario: Operator runs the unified subcommand for opencode

- GIVEN the operator types
  `royo-learn experience scan --source=opencode --project-root <path>`
- WHEN the dispatcher parses the args
- THEN it calls `opencode.Job().JobFunc(ctx, deps)`
- AND stdout carries one JSON object with the documented shape
  (`source`, `status`, `instances[]`, `ingested_turns`, `duplicates`,
  `skipped_incomplete`, `skipped_malformed`, `envelopes_total`)
- AND exit code is `0` on success.

#### Scenario: Operator runs the unified subcommand for codex

- GIVEN the operator types
  `royo-learn experience scan --source=codex --project-root <path>`
- WHEN the dispatcher parses the args
- THEN it calls `codex.Job().JobFunc(ctx, deps)`
- AND the JSON envelope shape is identical to the
  `--source=opencode` run.

#### Scenario: Missing `--source` flag fails fast

- GIVEN the operator types
  `royo-learn experience scan --project-root <path>` without
  `--source`
- WHEN the dispatcher parses the args
- THEN it returns a typed validation error (`invalid_argument`)
- AND the usage line prints `--source=<opencode|claudecode|codex>`.

### Requirement: `--source` value validated against `domain.ExperienceSource`

The system SHALL validate `--source` against the constants exported by
`internal/domain` (`domain.SourceOpenCode`,
`domain.SourceClaudeCode`, `domain.SourceCodex`). An invalid value MUST
return exit code `2` and an actionable error message naming the
allowed values. Validation MUST happen before any adapter call.

#### Scenario: Invalid `--source` returns exit code 2

- GIVEN the operator types
  `royo-learn experience scan --source=does_not_exist`
- WHEN the dispatcher parses the args
- THEN it returns exit code `2`
- AND the error message lists the allowed values
  (`opencode`, `claudecode`, `codex`)
- AND no adapter code runs.

#### Scenario: Each valid constant is accepted

- GIVEN the dispatcher loop iterates over `domain.SourceOpenCode`,
  `domain.SourceClaudeCode`, `domain.SourceCodex`
- WHEN each is passed as `--source`
- THEN the dispatcher reaches the matching adapter's `Job()` accessor
  without raising a validation error.

### Requirement: Per-source subcommand removal behind a feature flag

The system SHALL ship behind the boolean flag
`--experimental-cli-collapse`. When the flag is **on**, the legacy
`experience <source> scan` subcommands are removed and only the unified
form exists. When the flag is **off**, the legacy subcommands MUST
remain available and the unified form MAY be available as a parallel
subcommand. The flag default is `on` per the proposal Rollback §3.

#### Scenario: Flag on removes the per-source form

- GIVEN the binary is built with `--experimental-cli-collapse` set to
  `true` (the default)
- WHEN the operator types `royo-learn experience opencode scan`
- THEN the dispatcher returns `unknown command` (exit code `2`)
- AND no usage hint references the per-source form.

#### Scenario: Flag off keeps the per-source form

- GIVEN the binary is built with `--experimental-cli-collapse` set to
  `false`
- WHEN the operator types `royo-learn experience opencode scan`
- THEN the dispatcher routes to the opencode orchestrator
- AND stdout carries the same JSON envelope as the unified form.

### Requirement: Help output prints a deprecation note for the legacy form when the flag is off

The system SHALL print a one-line deprecation note on `stderr` whenever
the legacy per-source form is invoked while the flag is `off`. The
note MUST name the replacement form
(`experience scan --source=<opencode|claudecode|codex>`) and MUST
remain silent when the flag is `on` (the legacy form is gone). The
note is informational; it does not change the exit code.

#### Scenario: Legacy call prints the deprecation note

- GIVEN the binary is built with `--experimental-cli-collapse=false`
- WHEN the operator types `royo-learn experience codex scan`
- THEN stderr carries
  `DEPRECATED: use 'experience scan --source=codex' (legacy form removed in v1)`
- AND stdout still carries the JSON envelope
- AND exit code is `0` on success.

#### Scenario: Unified call never prints the deprecation note

- GIVEN any flag value
- WHEN the operator types the unified form
  `royo-learn experience scan --source=codex`
- THEN stderr does NOT carry the deprecation note
- AND exit code is `0` on success.

### Requirement: JSON envelope shape parity with the previous per-source subcommand

The system SHALL preserve the JSON envelope shape documented for the
previous per-source subcommands. The shape is
`{ source, status, instances[], ingested_turns, duplicates,
skipped_incomplete, skipped_malformed, envelopes_total }`. No field is
added or removed in v1. The unified subcommand's output MUST be
byte-identical to the legacy per-source subcommand's output for the
same input (deterministic ordering preserved).

#### Scenario: Output JSON keys are identical across forms

- GIVEN the same `--project-root`, `--source`, and fixture
- WHEN the operator runs the unified form
  `experience scan --source=opencode`
- AND the legacy form `experience opencode scan` (flag off)
- THEN the two stdout JSON objects have the same set of top-level
  keys
- AND the per-key value types match.

#### Scenario: Field removal breaks parity

- GIVEN a future change drops the `skipped_malformed` key from the
  envelope
- WHEN a regression test runs
- THEN it fails with a typed `envelope_schema_drift` error
- AND the change is blocked by the CI gate.

## References

- `docs/04-CLI-SPEC.md` (additive)
- `docs/21-EXPERIENCE-DOMAIN.md` §1
- `docs/22-ADAPTER-CONTRACT.md` §9
