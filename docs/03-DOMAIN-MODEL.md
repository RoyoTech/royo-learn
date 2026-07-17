# Modelo de dominio

## Learning

Campos obligatorios:

```go
type Learning struct {
    ID                  string
    ProjectID           string
    Status              LearningStatus
    Type                LearningType
    Title               string
    Context             string
    Observation         string
    ReusableLesson      string
    RecommendedProcedure []string
    Limits              string
    ScopeGuess          Scope
    ApprovedScope       *Scope
    Confidence          Confidence
    EvidenceLevel       EvidenceLevel
    ProposedDestination DestinationType
    ApprovedDestination *Destination
    RetrievalTerms      []string
    Fingerprint         string
    NormalizedHash      string
    CreatedAt           time.Time
    UpdatedAt           time.Time
    CreatedBy           Actor
    Revision            int
}
```

## Tipos

```text
procedure
prevention
diagnostic
tooling
architecture
quality
security
hypothesis
preference
```

`preference` solo puede publicarse como conocimiento personal/proyecto, nunca como regla compartida sin una decisión explícita del usuario.

## Estados

```text
captured
needs_evidence
approved
rejected
merged
published
superseded
archived
```

### Reversión de una publicación

Un rollback exitoso devuelve el aprendizaje a `approved`: la curación sigue
siendo válida y solo se deshizo la materialización publicada. La publicación
pasa a `rolled_back` y el aprendizaje a `approved` en una misma transacción.
Si la restauración falla, la publicación pasa a `failed` y el aprendizaje
permanece `published`, porque no puede afirmarse que dejó de estar publicado.

La arista `published → approved` no pertenece a `domain.ValidTransitions`.
Esa tabla gobierna la curación; solo el ciclo de vida de publicación puede
restaurar archivos y revocar el estado publicado de manera coordinada (D18).

## Alcances

```text
project
shared
personal
unknown
```

## Evidencia

```go
type Evidence struct {
    ID          string
    LearningID  string
    Kind        EvidenceKind
    URI         string
    Summary     string
    SHA256      string
    CollectedAt time.Time
    Command     []string
    ExitCode    *int
    Redacted    bool
    SizeBytes   int64
}
```

Kinds:

```text
file
git_diff
git_commit
command
test
engram_observation
issue
pull_request
text
external_reference
```

### Entrada pública de evidencia

Un registro de evidencia solo existe si una interfaz pública lo crea. Las dos
entradas públicas son:

1. **Durante la captura**: `learning_capture` y `royo-learn capture` aceptan un
   array `evidence[]`.
2. **Después de la captura**: `learning_add_evidence` (MCP) y
   `royo-learn evidence add` (CLI). El estado `needs_evidence` exige esta
   segunda entrada: sin ella, un aprendizaje devuelto a `needs_evidence` nunca
   podría volver a `approved`.

Forma de cada elemento del array `evidence[]`:

```json
{
  "kind": "test",
  "summary": "El test reproduce el fallo de checksum",
  "source": "go test ./internal/storage",
  "content": "--- FAIL: TestMigrationChecksum ..."
}
```

| Campo | Obligatorio | Descripción |
|-------|-------------|-------------|
| `kind` | sí | Uno de los kinds anteriores. `type` se acepta como alias de entrada. |
| `summary` | sí | Descripción legible del registro. |
| `source` | no | Origen del registro (ruta, comando, URL). Se persiste en `URI`. |
| `content` | no | Contenido literal. Se almacena en el blob store content-addressed y su SHA-256 se persiste en el registro. |

Colectores disponibles, y solo estos: evidencia entregada directamente,
`git status`, `git diff`, y el resultado de un comando explícitamente permitido.
No existe una taxonomía extensa de colectores y no se añadirá sin una prueba de
fallo que la justifique.

### Redacción de secretos: condición de escritura

La redacción de secretos (`internal/evidence`) es una **condición de escritura**,
no un filtro de salida. Ocurre **antes** de cualquier persistencia:

```text
entrada → REDACCIÓN → SQLite, blob store, Markdown, audit log, respuesta MCP/CLI, logs
```

Ningún sink puede recibir contenido sin redactar. Alcanza tanto a los registros
de evidencia (`summary`, `source`, `content`, `command`) como a los campos de
texto libre del propio aprendizaje (`title`, `context`, `observation`,
`reusable_lesson`, `limits`, `recommended_procedure`). El hash normalizado se
calcula sobre el contenido **ya redactado**, de modo que la deduplicación es
determinista.

`Redacted` indica si el registro fue modificado por la redacción.

### Umbral de evidencia (D3)

Aprobar un aprendizaje exige **dos condiciones acumulativas**:

1. `evidence_level` ≥ `moderate` (nunca `weak` ni `insufficient`);
2. al menos **un registro de evidencia persistido**.

La clasificación declarada no sustituye al registro. Declarar
`evidence_level: "strong"` sin adjuntar ni un solo registro **no aprueba**.

### Idempotencia (D5)

```text
misma idempotency_key       → reintento técnico: no crea aprendizaje, no duplica evidencia
distinta key + mismo hash   → evento equivalente: reutiliza el aprendizaje y registra recurrencia
sin key + mismo hash        → deduplicación conservadora: no registra recurrencia automática
```

## Relación

```text
duplicate_of
extends
supersedes
contradicts
narrows
related
merged_into
```

Las relaciones semánticas no son inferidas por Go. El sistema puede proponer similitud lexical, pero el agente suministra el veredicto.

## Curación

```go
type Curation struct {
    ID               string
    LearningID       string
    Decision         CurationDecision
    Relation         *Relation
    Rationale        string
    Validation       []ValidationResult
    Destination      *Destination
    AcceptanceChecks []Check
    RollbackCondition string
    CuratedBy        Actor
    CuratedAt        time.Time
}
```

Decisiones:

```text
reject
needs_evidence
merge
approve_project_knowledge
approve_shared_knowledge
approve_new_skill
approve_skill_update
approve_agents_rule
approve_test
```

## Publicación

```go
type PublicationPlan struct {
    LearningID       string
    TargetRoot       string
    TargetPath       string
    Operation        PublicationOperation
    Content          string
    Patch            string
    ManagedBlockID   string
    Verification     []CommandSpec
    RequiresApproval bool
    Risk             RiskLevel
}
```

Operaciones:

```text
create
replace
replace_managed_block
apply_unified_patch
append_record_reference
```

No implementar edición heurística libre.

## Aprobación

```go
type Approval struct {
    ID          string
    LearningID  string
    PreviewHash string
    ApprovedBy  string
    Reason      string
    CreatedAt   time.Time
    ExpiresAt   *time.Time
    RevokedAt   *time.Time
}
```

## Ocurrencia

```go
type Occurrence struct {
    ID                   string
    LearningID           *string
    ProjectID            string
    Fingerprint          string
    Summary              string
    Evidence             []EvidenceRef
    LearningWasRetrieved *bool
    SkillWasActivated    *bool
    Outcome              OccurrenceOutcome
    OccurredAt           time.Time
}
```

Outcomes:

```text
prevented
recurred
detected_early
false_positive
unknown
```

## Actor

```go
type Actor struct {
    Kind      string // human, agent, system
    Name      string
    Model     string
    SessionID string
}
```

## Invariantes

- Un `published` siempre tiene al menos una publicación verificada.
- Un `approved` siempre tiene curación.
- Toda operación que cambia la verdad de un aprendizaje re-materializa su
  registro Markdown desde SQLite. Si esa tarea posterior al commit falla, la
  respuesta declara el estado ya confirmado y prohíbe el reintento ciego (D18).
- Un `Approval` solo vale para un `PreviewHash`.
- Una publicación no puede escapar de una raíz autorizada.
- Una evidencia nunca contiene un secreto conocido sin redacción.
- Una relación no puede referenciar el mismo ID en ambos extremos.
- El audit log no se actualiza ni elimina.
