// Package domain defines the core types and errors for royo-learn.
package domain

import (
	"encoding/json"
	"time"
)

// --- Typed IDs ------------------------------------------------

// LearningID is a typed string ID for a Learning.
type LearningID string

// ProjectID is a typed string ID for a Project.
type ProjectID string

// EvidenceID is a typed string ID for Evidence.
type EvidenceID string

// RelationID is a typed string ID for a LearningRelation.
type RelationID string

// CurationID is a typed string ID for a Curation.
type CurationID string

// ApprovalID is a typed string ID for an Approval.
type ApprovalID string

// PublicationID is a typed string ID for a Publication.
type PublicationID string

// OccurrenceID is a typed string ID for an Occurrence.
type OccurrenceID string

// AuditEventID is a typed string ID for an AuditEvent.
type AuditEventID string

// --- Enums ----------------------------------------------------

// LearningStatus represents the lifecycle state of a learning.
type LearningStatus string

const (
	StatusCaptured      LearningStatus = "captured"
	StatusNeedsEvidence LearningStatus = "needs_evidence"
	StatusApproved      LearningStatus = "approved"
	StatusRejected      LearningStatus = "rejected"
	StatusMerged        LearningStatus = "merged"
	StatusPublished     LearningStatus = "published"
	StatusSuperseded    LearningStatus = "superseded"
	StatusArchived      LearningStatus = "archived"
)

// LearningType classifies the kind of learning.
type LearningType string

const (
	TypeProcedure    LearningType = "procedure"
	TypePrevention   LearningType = "prevention"
	TypeDiagnostic   LearningType = "diagnostic"
	TypeTooling      LearningType = "tooling"
	TypeArchitecture LearningType = "architecture"
	TypeQuality      LearningType = "quality"
	TypeSecurity     LearningType = "security"
	TypeHypothesis   LearningType = "hypothesis"
	TypePreference   LearningType = "preference"
)

// Scope represents the applicability scope of a learning.
type Scope string

const (
	ScopeProject  Scope = "project"
	ScopeShared   Scope = "shared"
	ScopePersonal Scope = "personal"
	ScopeUnknown  Scope = "unknown"
)

// Confidence represents the confidence level of a learning.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// EvidenceLevel describes the strength of evidence.
type EvidenceLevel string

const (
	EvidenceStrong       EvidenceLevel = "strong"
	EvidenceModerate     EvidenceLevel = "moderate"
	EvidenceWeak         EvidenceLevel = "weak"
	EvidenceInsufficient EvidenceLevel = "insufficient"
)

// EvidenceKind describes the type of evidence.
type EvidenceKind string

const (
	KindFile              EvidenceKind = "file"
	KindGitDiff           EvidenceKind = "git_diff"
	KindGitCommit         EvidenceKind = "git_commit"
	KindCommand           EvidenceKind = "command"
	KindTest              EvidenceKind = "test"
	KindEngramObservation EvidenceKind = "engram_observation"
	KindIssue             EvidenceKind = "issue"
	KindPullRequest       EvidenceKind = "pull_request"
	KindText              EvidenceKind = "text"
	KindExternalReference EvidenceKind = "external_reference"
)

// DestinationType represents where a learning is proposed to be published.
type DestinationType string

const (
	DestNone       DestinationType = "none"
	DestProject    DestinationType = "project"
	DestShared     DestinationType = "shared"
	DestSkill      DestinationType = "skill"
	DestAgentsRule DestinationType = "agents_rule"
)

// CurationDecision represents a curator's decision on a learning.
type CurationDecision string

const (
	CurationReject                  CurationDecision = "reject"
	CurationNeedsEvidence           CurationDecision = "needs_evidence"
	CurationMerge                   CurationDecision = "merge"
	CurationApproveProjectKnowledge CurationDecision = "approve_project_knowledge"
	CurationApproveSharedKnowledge  CurationDecision = "approve_shared_knowledge"
	CurationApproveNewSkill         CurationDecision = "approve_new_skill"
	CurationApproveSkillUpdate      CurationDecision = "approve_skill_update"
	CurationApproveAgentsRule       CurationDecision = "approve_agents_rule"
	CurationApproveTest             CurationDecision = "approve_test"
)

// RelationType describes the semantic relationship between two learnings.
type RelationType string

const (
	RelationDuplicateOf RelationType = "duplicate_of"
	RelationExtends     RelationType = "extends"
	RelationSupersedes  RelationType = "supersedes"
	RelationContradicts RelationType = "contradicts"
	RelationNarrows     RelationType = "narrows"
	RelationRelated     RelationType = "related"
	RelationMergedInto  RelationType = "merged_into"
)

// RiskLevel represents the risk of a publication operation.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// PublicationStatus represents the status of a publication job.
type PublicationStatus string

const (
	PubStatusPending    PublicationStatus = "pending"
	PubStatusInProgress PublicationStatus = "in_progress"
	PubStatusCompleted  PublicationStatus = "completed"
	PubStatusFailed     PublicationStatus = "failed"
	PubStatusRolledback PublicationStatus = "rolled_back"
)

// OccurrenceOutcome represents the outcome of an occurrence check.
type OccurrenceOutcome string

const (
	OutcomePrevented     OccurrenceOutcome = "prevented"
	OutcomeRecurred      OccurrenceOutcome = "recurred"
	OutcomeDetectedEarly OccurrenceOutcome = "detected_early"
	OutcomeFalsePositive OccurrenceOutcome = "false_positive"
	OutcomeUnknown       OccurrenceOutcome = "unknown"
)

// PublicationOperation represents the type of file operation for a publication.
type PublicationOperation string

const (
	OpCreate              PublicationOperation = "create"
	OpReplace             PublicationOperation = "replace"
	OpReplaceManagedBlock PublicationOperation = "replace_managed_block"
	OpApplyUnifiedPatch   PublicationOperation = "apply_unified_patch"
	OpAppendRecordRef     PublicationOperation = "append_record_reference"
)

// --- Structs --------------------------------------------------

// Actor identifies who performed an action.
type Actor struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
}

// ActorJSON returns the JSON-encoded representation of an Actor,
// or "{}" if encoding fails.
func (a Actor) ActorJSON() string {
	b, err := json.Marshal(a)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Project represents a project in the royo-learn database.
type Project struct {
	ID            ProjectID
	ProjectKey    string
	DisplayName   string
	CanonicalPath string
	GitRemote     string
	Fingerprint   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Learning represents a captured learning entry.
type Learning struct {
	ID                   LearningID
	ProjectID            ProjectID
	Status               LearningStatus
	Type                 LearningType
	Title                string
	Context              string
	Observation          string
	ReusableLesson       string
	RecommendedProcedure []string
	Limits               string
	ScopeGuess           Scope
	ApprovedScope        *Scope
	Confidence           Confidence
	EvidenceLevel        EvidenceLevel
	ProposedDestination  DestinationType
	ApprovedDestination  *Destination
	RetrievalTerms       []string
	Fingerprint          string
	NormalizedHash       string
	IdempotencyKey       *string
	Actor                Actor
	Revision             int
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Destination describes the target of a publication.
type Destination struct {
	Type     DestinationType `json:"type"`
	Root     string          `json:"root"`
	Path     string          `json:"path"`
	Required bool            `json:"required"`
}

// Evidence is a piece of supporting evidence attached to a learning.
type Evidence struct {
	ID          EvidenceID
	LearningID  LearningID
	Kind        EvidenceKind
	URI         string
	Summary     string
	SHA256      string
	Command     []string
	ExitCode    *int
	Redacted    bool
	SizeBytes   int64
	CollectedAt time.Time
}

// EvidenceRef is a lightweight reference to evidence (used in Occurrence).
type EvidenceRef struct {
	EvidenceID EvidenceID `json:"evidence_id"`
	Kind       string     `json:"kind"`
	Summary    string     `json:"summary"`
}

// LearningRelation describes a semantic relationship between two learnings.
type LearningRelation struct {
	ID               RelationID
	SourceLearningID LearningID
	TargetLearningID LearningID
	Relation         RelationType
	Confidence       *float64
	Rationale        string
	Actor            Actor
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Curation represents a curator's decision on a learning.
type Curation struct {
	ID                CurationID
	LearningID        LearningID
	Decision          CurationDecision
	Rationale         string
	Destination       *Destination
	Validation        []ValidationResult
	AcceptanceChecks  []Check
	RollbackCondition string
	Actor             Actor
	CreatedAt         time.Time
}

// ValidationResult holds the outcome of a validation check.
type ValidationResult struct {
	Check string `json:"check"`
	Pass  bool   `json:"pass"`
	Note  string `json:"note"`
}

// Check is an acceptance check condition.
type Check struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Expected string `json:"expected"`
}

// PublicationPreview is a preview of what a publication would do.
type PublicationPreview struct {
	ID               PreviewID
	LearningID       LearningID
	Plan             PublicationPlan
	PreviewHash      string
	Risk             RiskLevel
	RequiresApproval bool
	CreatedAt        time.Time
	InvalidatedAt    *time.Time
}

// PreviewID is a typed string ID for a PublicationPreview.
type PreviewID string

// PublicationPlan describes the planned publication operation.
type PublicationPlan struct {
	LearningID       LearningID           `json:"learning_id"`
	TargetRoot       string               `json:"target_root"`
	TargetPath       string               `json:"target_path"`
	Operation        PublicationOperation `json:"operation"`
	Content          string               `json:"content"`
	Patch            string               `json:"patch"`
	ManagedBlockID   string               `json:"managed_block_id"`
	Verification     []CommandSpec        `json:"verification"`
	RequiresApproval bool                 `json:"requires_approval"`
	Risk             RiskLevel            `json:"risk"`
}

// CommandSpec describes a verification command.
type CommandSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

// Approval records a human approval for publication.
type Approval struct {
	ID               ApprovalID
	LearningID       LearningID
	PreviewHash      string
	ApprovedBy       string
	Reason           string
	ApprovalEvidence string
	CreatedAt        time.Time
	ExpiresAt        *time.Time
	RevokedAt        *time.Time
}

// Publication records a publication job.
type Publication struct {
	ID           PublicationID
	LearningID   LearningID
	PreviewHash  string
	ApprovalID   *ApprovalID
	Targets      []TargetEntry
	Verification []ValidationResult
	Rollback     []RollbackEntry
	Status       PublicationStatus
	StartedAt    time.Time
	CompletedAt  *time.Time
	ErrorCode    *string
	ErrorMessage *string
}

// TargetEntry describes a single publication target file.
type TargetEntry struct {
	Root      string               `json:"root"`
	Path      string               `json:"path"`
	Operation PublicationOperation `json:"operation"`
}

// RollbackEntry describes a rollback step.
type RollbackEntry struct {
	Path    string `json:"path"`
	Backup  string `json:"backup"`
	Success bool   `json:"success"`
}

// Occurrence records an occurrence (or non-occurrence) of a learning's pattern.
type Occurrence struct {
	ID                   OccurrenceID
	LearningID           *LearningID
	ProjectID            ProjectID
	Fingerprint          string
	Summary              string
	Evidence             []EvidenceRef
	LearningWasRetrieved *bool
	SkillWasActivated    *bool
	Outcome              OccurrenceOutcome
	OccurredAt           time.Time
	Actor                Actor
}

// AuditEvent is an append-only record of a system action.
type AuditEvent struct {
	Sequence      int
	ID            AuditEventID
	OccurredAt    time.Time
	Actor         Actor
	Operation     string
	EntityType    string
	EntityID      string
	PreviousState *string
	NewState      *string
	PayloadSHA256 string
	Result        string
	ErrorCode     *string
	Details       map[string]any
}

// Revision represents a point-in-time snapshot of a learning's payload.
type Revision struct {
	ID            string
	LearningID    LearningID
	Revision      int
	Payload       Learning
	PayloadSHA256 string
	CreatedAt     time.Time
	CreatedBy     Actor
}

// --- Filters --------------------------------------------------

// LearningFilter provides filtering options for listing learnings.
type LearningFilter struct {
	Status []LearningStatus
	Type   []LearningType
	Scope  []Scope
	Limit  int
	Offset int
}
