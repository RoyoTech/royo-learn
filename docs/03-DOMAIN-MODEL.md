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
- Un `Approval` solo vale para un `PreviewHash`.
- Una publicación no puede escapar de una raíz autorizada.
- Una evidencia nunca contiene un secreto conocido sin redacción.
- Una relación no puede referenciar el mismo ID en ambos extremos.
- El audit log no se actualiza ni elimina.
