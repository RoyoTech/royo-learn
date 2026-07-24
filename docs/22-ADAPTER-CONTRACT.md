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
