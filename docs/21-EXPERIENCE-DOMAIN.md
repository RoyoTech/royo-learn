# Modelo de dominio — Experiencia y patrones

- **Estado:** contrato congelado (Hito 0)
- **Relación con `docs/03-DOMAIN-MODEL.md`:** este documento **añade** entidades
  nuevas. No modifica `Learning`, `Evidence`, `Curation`, `Publication`,
  `Approval`, `Occurrence` ni `Actor`. La experiencia observada es evidencia
  preliminar, no conocimiento aprobado.
- **Anclaje al código real:** `Actor` ya existe con `SessionID`
  (`internal/domain/types.go`); estas entidades lo reutilizan sin duplicarlo.

## 0. Regla de separación

No mezclar estas entidades con `Learning`. El único puente permitido es la
**promoción** (`docs/23-PATTERN-MINING.md` §promoción), que crea un `Learning`
mediante `capture.Service`.

## 1. `ExperienceSource`

```go
type ExperienceSource string

const (
    SourceOpenCode   ExperienceSource = "opencode"
    SourceClaudeCode ExperienceSource = "claude_code"
    SourceCodex      ExperienceSource = "codex"
    SourcePi         ExperienceSource = "pi"
    SourceManual     ExperienceSource = "manual"
)
```

## 2. `ExperienceSession`

```go
type ExperienceSession struct {
    ID                ExperienceSessionID
    ProjectID         ProjectID
    Source            ExperienceSource
    ExternalSessionID string
    Locator           TranscriptLocator
    StartedAt         *time.Time
    UpdatedAt         time.Time
    ClosedAt          *time.Time
    MetadataSHA256    string
    CreatedAt         time.Time
}
```

Reglas:

- `(project_id, source, external_session_id)` único;
- `Locator` no se materializa en records Markdown por defecto;
- datos sensibles del path permanecen en SQLite local;
- toda actualización es idempotente.

## 3. `TranscriptLocator`

```go
type TranscriptLocator struct {
    Kind       string // sqlite, jsonl, rollout, file, api
    Path       string // solo local; validar raíz permitida
    SessionID  string
    TurnID     string
    Offset     int64
    SourceHash string
}
```

Reglas de seguridad (ver `docs/24-EXPERIENCE-THREAT-MODEL.md`):

- validar contra roots configuradas **por el usuario**; el repositorio no puede
  ampliarlas;
- no seguir symlinks fuera de raíz;
- no aceptar esquemas remotos en v1;
- nunca ejecutar contenido del locator;
- `SourceHash` detecta sustitución del archivo o DB.

## 4. `ExperienceTurn`

```go
type ExperienceTurn struct {
    ID              ExperienceTurnID
    SessionID       ExperienceSessionID
    ExternalTurnID  string
    Sequence        int64
    Status          TurnStatus
    Fingerprint     string
    UserDigest      string
    AssistantDigest string
    ToolCallsDigest string
    SafeSummary     string
    OccurredAt      time.Time
    StableAt        *time.Time
    IngestedAt      time.Time
    SourceRevision  string
    Redacted        bool
}
```

Estados (`TurnStatus`):

```text
observed → incomplete → stable → ingested
                     ↘ superseded
                     ↘ failed
```

Reglas:

- `(session_id, external_turn_id)` único;
- una modificación del turno produce nueva `SourceRevision`, no un duplicado;
- un turno incompleto **nunca** origina un patrón cualificado;
- el contenido completo no se almacena por defecto; `SafeSummary` es opcional y
  acotado;
- la referencia al original permanece disponible.

## 5. `ExperienceEvent`

```go
type ExperienceEventKind string

const (
    EventUserCorrection       ExperienceEventKind = "user_correction"
    EventCommandFailure       ExperienceEventKind = "command_failure"
    EventTestFailure          ExperienceEventKind = "test_failure"
    EventTestSuccess          ExperienceEventKind = "test_success"
    EventSuccessfulProcedure  ExperienceEventKind = "successful_procedure"
    EventRetryCorrected       ExperienceEventKind = "retry_corrected"
    EventToolLimitation       ExperienceEventKind = "tool_limitation"
    EventArchitectureDecision ExperienceEventKind = "architecture_decision"
    EventPreference           ExperienceEventKind = "preference"
    EventUnknown              ExperienceEventKind = "unknown"
)

type ExperienceEvent struct {
    ID           ExperienceEventID
    ProjectID    ProjectID
    TurnID       ExperienceTurnID
    Kind         ExperienceEventKind
    Summary      string
    Observation  string
    Outcome      string
    Fingerprint  string
    EvidenceJSON string
    Detector     DetectorIdentity
    Confidence   Confidence   // reutiliza el tipo existente del dominio
    CreatedAt    time.Time
}
```

Reglas:

- separar **observación** de **interpretación**;
- redacción **antes** del hash;
- no aceptar comandos para ejecución;
- `preference` nunca se convierte en Skill;
- todo evento conserva `TurnID` (procedencia).

`DetectorIdentity` distingue detector determinista de detector asistido por host:

```go
type DetectorIdentity struct {
    Kind       string // deterministic, host_llm
    Name       string
    Version    string
    Model      string // solo host_llm
    PromptHash string // solo host_llm
}
```

La salida `host_llm` se trata como **no confiable**: se validan enums, se limita
texto, se redacta, se exigen fuentes y no puede elevar evidencia a `strong` ni
provocar promoción automática.

## 6. `ExperiencePattern`

```go
type PatternStatus string

const (
    PatternObserved  PatternStatus = "observed"
    PatternQualified PatternStatus = "qualified"
    PatternDismissed PatternStatus = "dismissed"
    PatternPromoted  PatternStatus = "promoted"
    PatternStale     PatternStatus = "stale"
)

type ExperiencePattern struct {
    ID                 ExperiencePatternID
    ProjectID          ProjectID
    Status             PatternStatus
    Kind               ExperienceEventKind
    Fingerprint        string
    Title              string
    Summary            string
    DistinctSessions   int
    DistinctDays       int
    OccurrenceCount    int
    FirstSeenAt        time.Time
    LastSeenAt         time.Time
    ProposedLearningID *LearningID
    DetectorVersion    string
    InputDigest        string
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

Tabla relacional `experience_pattern_members`:

```text
pattern_id · event_id · similarity_kind · similarity_score · added_at
UNIQUE(pattern_id, event_id)
```

## 7. `IngestionCursor`

```go
type IngestionCursor struct {
    ProjectID        ProjectID
    Source           ExperienceSource
    SourceInstance   string
    CursorJSON       string
    LastSuccessfulAt *time.Time
    LastAttemptAt    *time.Time
    LastErrorCode    string
    LastErrorMessage string
    InputDigest      string
    Revision         int
}
```

Reconstrucción: si se pierde, releer la fuente y deduplicar por IDs y
fingerprints; no re-resumir ni recrear eventos existentes. **El cursor se
actualiza solo tras el commit de la transacción de ingestión.**

## 8. `JobState`

```go
type JobState struct {
    ProjectID      ProjectID
    JobName        string
    Status         string // idle, running, ok, degraded, error
    InputDigest    string
    LeaseOwner     string
    LeaseExpiresAt *time.Time
    LastStartedAt  *time.Time
    LastSuccessAt  *time.Time
    LastFailedAt   *time.Time
    LastErrorCode  string
    LastError      string
    MetricsJSON    string
}
```

SQLite es la autoridad del lease. Un `.lock` de filesystem solo puede añadirse
como defensa **secundaria** para procesos locales, nunca como coordinación
única.

## 9. Asociación `Learning ↔ experiencia`

Tabla `learning_experience_sources`:

```text
learning_id · event_id · relation · confidence · created_at
UNIQUE(learning_id, event_id, relation)
```

Relaciones: `origin`, `supporting`, `contradicting`, `recurrence`, `validation`.
Al promoverse un patrón, sus eventos miembros quedan asociados como `origin`.

## 10. Invariantes del dominio de experiencia

- Un turno `incomplete` no puede pertenecer a un patrón `qualified`.
- Un `ExperienceEvent` siempre tiene `TurnID`; sin procedencia no hay evento.
- Ningún fingerprint se calcula antes de la redacción.
- Ningún estado de `Learning` cambia salvo por promoción vía `capture.Service`.
- El cursor nunca se adelanta a un commit.
- El lease vive en SQLite; el archivo `.lock` es opcional y secundario.
- La experiencia observada nunca se publica ni instala automáticamente.
