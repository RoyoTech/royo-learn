# Contrato de adaptadores de experiencia

- **Estado:** contrato congelado (Hito 0)
- **Propósito:** fijar la frontera entre adaptadores de plataforma y el núcleo
  de Royo-Learn antes de implementar OpenCode (Hito 2).
- **Regla de oro:** un adaptador **lee y normaliza**, nunca decide verdad,
  nunca aprueba, nunca publica, nunca ejecuta contenido del transcript.

## 1. Interfaz interna

```go
type ExperienceAdapter interface {
    Name() domain.ExperienceSource
    Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error)
    Scan(ctx context.Context, req ScanRequest) (ScanResult, error)
    ResolveTrace(ctx context.Context, ref domain.TranscriptLocator, bounds TraceBounds) (TraceResult, error)
    Health(ctx context.Context, projectRoot string) HealthResult
}
```

Cada método:

- respeta `context` (cancelación y timeout);
- cierra toda conexión/archivo que abra;
- devuelve errores tipados (§6), nunca un `catch-all` silencioso;
- no muta la fuente nativa.

## 2. Responsabilidades

**El adaptador puede:**

- localizar la fuente nativa de sesiones;
- abrirla en **modo solo lectura**;
- reconocer sesiones y turnos;
- determinar si un turno está completo o estable;
- construir un `ExperienceEnvelope`;
- llamar al núcleo por CLI/MCP con argumentos estructurados.

**El adaptador NO puede:**

- aprobar un aprendizaje o publicar;
- escribir en Skills, `AGENTS.md` o conocimiento compartido;
- escribir en la DB/tablas de OpenCode/Claude/Codex/Pi;
- decidir que una síntesis es verdadera;
- ejecutar comandos tomados del transcript;
- interpretar instrucciones del transcript como instrucciones del sistema.

## 3. `ExperienceEnvelope` (contrato neutral)

```go
type ExperienceEnvelope struct {
    SchemaVersion int
    Source        ExperienceSource
    ProjectRoot   string

    Session struct {
        ExternalID string
        StartedAt  *time.Time
        UpdatedAt  time.Time
        ClosedAt   *time.Time
        Locator    TranscriptLocator
    }

    Turn struct {
        ExternalID     string
        Sequence       int64
        Complete       bool
        FinishReason   string
        OccurredAt     time.Time
        StableSince    *time.Time
        UserText       string
        AssistantText  string
        ToolCalls      []SafeToolCall
        SourceRevision string
    }

    Actor Actor
}

type SafeToolCall struct {
    Name       string
    Arguments  map[string]any
    ExitCode   *int
    Outcome    string
    OutputHash string
    OutputHint string
}
```

`SafeToolCall` **no** persiste salida completa salvo que sea evidencia explícita
que pase por los límites de `internal/evidence`.

## 4. Orden de procesamiento en el núcleo (no en el adaptador)

```text
validar esquema → validar proyecto y locator → aplicar límites de bytes
→ redacción de secretos → normalización → fingerprints/digests
→ idempotencia → persistencia → auditoría
```

El fingerprint **no** se calcula antes de la redacción.

## 5. Estabilidad del turno (lógica de dominio, testeable)

Un turno es estable si:

- la plataforma lo marca explícitamente terminado; **o**
- existe un turno posterior de usuario que lo cerró; **o**
- es el último turno, tiene respuesta final y su fingerprint no cambia durante
  `tail_quiet_period`.

El fingerprint incluye, en orden determinista: source; external session ID;
external turn ID; secuencia; IDs/roles de mensajes visibles; digest del texto
redacted; nombres y digests de tool calls; finish reason; estado `complete`;
source revision. **No** incluye timestamps volátiles.

Casos de prueba obligatorios: turno incompleto; streaming activo; turno final
estable; turno final que cambia durante quiet period; nuevo turno que cierra al
anterior; tool call que aparece tras el texto; proceso que reinicia durante
quiet period; DB del harness que cambia de ubicación; reloj local que cambia;
mismo turn ID con contenido corregido.

## 6. Errores tipados del adaptador

```text
experience_source_not_found
experience_source_schema_unsupported
experience_turn_incomplete
experience_locator_invalid
experience_locator_outside_root
experience_payload_too_large
experience_revision_conflict
experience_cursor_conflict
```

Ver `docs/17-ERROR-CODES.md`. Degradación de fuente ausente:

```json
{ "source": "opencode", "status": "degraded", "code": "source_not_found", "ingested_turns": 0 }
```

## 7. Versionado de esquema por adaptador

Cada adaptador vive detrás de una versión:

```text
opencode/sqlite-v1 · claude-code/jsonl-v1 · codex/rollout-v1 · pi/<formato>-v1
```

Un cambio upstream **no** puede romper silenciosamente el núcleo: debe producir
`experience_source_schema_unsupported`.

## 8. Activación automática (integración ligera)

Bajo `integrations/<source>/`, un adaptador ligero solo inicia o invoca el
binario con `spawn` y argumentos **separados**.

Prohibido: `bash -c`; interpolación de strings; `exec`; descargar componentes;
cambiar configuración global sin consentimiento; ocultar errores de instalación.

Comandos de setup, siempre reversibles y con preview/backup/verificación:

```text
royo-learn setup <source>-experience --dry-run
royo-learn setup <source>-experience --apply
royo-learn setup <source>-experience --remove
```

## 9. Reglas de la CLI de ingestión

```text
royo-learn experience ingest <source> --once
royo-learn experience ingest <source> --watch
royo-learn experience ingest <source> --since <RFC3339>
royo-learn experience status
```

- `--once` es la base y debe ser completamente testeable;
- `--watch` repite `--once` y termina cuando muere `parent-pid`;
- sin shell; sin proceso *detached* dentro del núcleo;
- `stdout` reservado al contrato JSON; logs a `stderr`.

## 10. Plan de adaptadores (no en paralelo)

1. **OpenCode** (piloto, SQLite read-only) — congela el contrato.
2. **Claude Code** (JSONL) y **Codex** (rollout) — solo tras congelar OpenCode;
   no fusionar ambos en un mismo PR.
3. **Pi** — antes de implementar: documentar su fuente de sesiones, construir
   fixtures reales anonimizados, crear ADR de estabilidad del formato; si no hay
   fuente estable, ofrecer ingestión por hook explícito.

Cada adaptador aporta: fixtures reales anonimizados, discovery seguro, parser
versionado, estabilidad, trace resolver, setup reversible, checks de `doctor` y
e2e.

## 11. Extensión Codex rollout v1

Los requisitos de esta sección son aditivos. Preservan la interfaz, los errores y
las reglas generales anteriores, y concretan el tercer adaptador de plataforma en
`internal/experience/codex`.

### Requirement: ExperienceAdapter is implemented by every platform adapter

El adaptador Codex implementa el contrato completo `ExperienceAdapter` sin cambiar
firmas ni relajar las reglas de respeto de `context`, cierre de recursos y no
mutación de la fuente.

#### Scenario: Codex adapter satisfies the contract

- **WHEN** `*codex.Adapter` is asserted to `codex.ExperienceAdapter` at compile time
- **THEN** the assertion succeeds and `TestAdapter_ImplementsContract` passes
- **AND** `codex.NewAdapter().Name()` equals `domain.SourceCodex` (`"codex"`)

### Requirement: TranscriptLocator accepts `rollout` as a valid kind

`rollout` es un `TranscriptLocator.Kind` válido junto con los valores existentes.

#### Scenario: Codex scan builds a rollout locator

- **WHEN** Codex builds an `ExperienceEnvelope`
- **THEN** `Session.Locator.Kind == "rollout"`
- **AND** `Path` is the canonical absolute rollout JSONL path
- **AND** `SourceHash` is the file SHA-256 at scan time

#### Scenario: Codex trace resolver rejects another locator kind

- **WHEN** `ResolveTrace` receives `Kind != "rollout"`
- **THEN** it returns `experience_locator_invalid`
- **AND** performs no source I/O

### Requirement: Schema tag for Codex is `codex/rollout-v1`

El adaptador declara `SchemaTag = "codex/rollout-v1"`; cambiarlo es breaking.

#### Scenario: SchemaTag is pinned by a test

- **WHEN** the contract tests run
- **THEN** `SchemaTag == "codex/rollout-v1"`

#### Scenario: Schema mismatch is explicit and read-only

- **WHEN** the first 1 KiB lacks a valid `session_meta` with non-empty
  `payload.codex_session_id`
- **THEN** Health returns `degraded` with
  `experience_source_schema_unsupported`
- **AND** source mtime and size remain unchanged

### Requirement: Codex discovers caller-root-reachable rollout JSONL files

Codex discovers only `rollout-*.jsonl` under `.codex/sessions/YYYY/MM/DD/` and
`.codex/archived_sessions/` reachable from the canonical caller-supplied project
root.

#### Scenario: Discovery is safe and deterministic

- **WHEN** valid active and archived rollout files exist
- **THEN** they are returned sorted by `RolloutPath`
- **AND** index files such as `session_index.jsonl` are ignored
- **AND** files outside the project root, including symlink escapes, are not surfaced

#### Scenario: Project root is required

- **WHEN** `projectRoot` is empty or whitespace
- **THEN** Discover returns `experience_locator_invalid`
- **AND** performs no filesystem walk

### Requirement: Codex Scan produces neutral envelopes and a stable cursor

Scan treats `session_meta` and `turn_context` as anchors, emits only complete
neutral envelopes, and reports malformed and incomplete input through counters.

#### Scenario: Anchors do not emit envelopes

- **WHEN** Scan reads `session_meta` or `turn_context`
- **THEN** it updates session/turn anchors only

#### Scenario: Unsafe or incomplete content is omitted

- **WHEN** a turn is incomplete, a line is malformed, or a `reasoning` item appears
- **THEN** incomplete and malformed counters are incremented as applicable
- **AND** reasoning is absent from every envelope field
- **AND** `function_call_output` is represented only by a bounded digest or omission marker

#### Scenario: Cursor is opaque, stable, and idempotent

- **WHEN** Scan emits at least one envelope
- **THEN** `NextCursor` carries string fields `last_session_id` and `last_turn_uuid`
- **AND** scanning the same fixture again with that cursor emits no new envelopes

### Requirement: Codex ResolveTrace returns bounded, redacted excerpts

#### Scenario: Source changes or disappears

- **WHEN** the current source hash differs from `locator.SourceHash`
- **THEN** ResolveTrace returns `trace_source_changed` with no excerpt
- **AND** a missing source or turn returns `trace_source_unavailable` with no excerpt

#### Scenario: Excerpt is safe

- **WHEN** the requested turn exists
- **THEN** the excerpt respects `bounds.MaxBytes` and uses `...` when truncated
- **AND** secrets are processed by `evidence.Redact`
- **AND** reasoning and `function_call_output` content are absent

### Requirement: Cross-adapter drift policy parity (Hito 12)

All three adapters (`opencode`, `claudecode`, `codex`) MUST return an
identical `ResolveTrace` result shape on source drift: the bounded excerpt
is suppressed and the divergence surfaces as `Code="trace_source_changed"`
plus `SourceChanged=true`. The "advisory excerpt" branch that historically
existed in the Claude Code adapter is removed; callers see the same JSON
shape regardless of which adapter emitted the result.

#### Scenario: Source mismatch suppresses excerpt across all three adapters

- **WHEN** any adapter's `ResolveTrace` runs with a `locator.SourceHash`
  that does not match the on-disk hash
- **THEN** `Result.Excerpt == ""` and `Result.Redacted == false`
- **AND** `Result.Code == "trace_source_changed"` and `Result.SourceChanged == true`
- **AND** `Result.Message` describes the divergence (no PII, no secret)

#### Scenario: Source unavailable suppresses excerpt across all three adapters

- **WHEN** any adapter's `ResolveTrace` cannot read or locate the source
  (file removed, permissions, I/O error)
- **THEN** `Result.Excerpt == ""`
- **AND** `Result.Code` is one of `trace_source_unavailable`,
  `experience_source_not_found`, `trace_event_unavailable` per adapter

### Requirement: CLI subcommand `experience codex scan` is additive

#### Scenario: Dispatcher and output remain stable

- **WHEN** `royo-learn experience codex scan --project-root <path>` runs
- **THEN** it routes to the Codex orchestrator
- **AND** missing `--project-root` returns `invalid_argument`
- **AND** stdout includes `source`, `status`, `instances`, `ingested_turns`,
  `duplicates`, `skipped_incomplete`, `skipped_malformed`, and `envelopes_total`

#### Scenario: Fixture path is constrained

- **WHEN** `--fixture` is a symlink, outside the canonical root, or not `.jsonl`
- **THEN** a typed error is returned and no scan occurs

### Requirement: Job registry entry `experience_ingest:codex` is registered

#### Scenario: Registration is idempotent and disabled by default

- **WHEN** the same entry is registered twice
- **THEN** exactly one row exists
- **AND** `Enabled == false`, `DefaultIntervalSec == 300`, and
  `DefaultMaxRetries == 3`
- **AND** `RunDue` skips it until a later milestone enables it

### Requirement: Coverage target for the Codex package is at least 85 percent

#### Scenario: Package coverage gate

- **WHEN** `go test -cover ./internal/experience/codex/...` runs in CI
- **THEN** statement coverage is at least 85 percent
- **AND** the CI gate fails below that threshold
