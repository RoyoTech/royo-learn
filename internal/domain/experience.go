package domain

import "time"

// This file adds the experience-discovery domain (Hito 1). Observed experience
// is preliminary evidence, never approved knowledge: these entities never mutate
// a Learning. The only bridge is a governed promotion through capture.Service.
// See docs/21-EXPERIENCE-DOMAIN.md.

// --- Typed IDs ------------------------------------------------

// ExperienceSessionID is a typed string ID for an ExperienceSession.
type ExperienceSessionID string

// ExperienceTurnID is a typed string ID for an ExperienceTurn.
type ExperienceTurnID string

// ExperienceEventID is a typed string ID for an ExperienceEvent.
type ExperienceEventID string

// --- Enums ----------------------------------------------------

// ExperienceSource identifies the platform that produced a session.
type ExperienceSource string

const (
	SourceOpenCode   ExperienceSource = "opencode"
	SourceClaudeCode ExperienceSource = "claude_code"
	SourceCodex      ExperienceSource = "codex"
	SourcePi         ExperienceSource = "pi"
	SourceManual     ExperienceSource = "manual"
)

// TurnStatus is the lifecycle state of an ingested turn.
type TurnStatus string

const (
	TurnObserved   TurnStatus = "observed"
	TurnIncomplete TurnStatus = "incomplete"
	TurnStable     TurnStatus = "stable"
	TurnIngested   TurnStatus = "ingested"
	TurnSuperseded TurnStatus = "superseded"
	TurnFailed     TurnStatus = "failed"
)

// ExperienceEventKind classifies a useful observation inside a turn.
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

// --- Value objects --------------------------------------------

// TranscriptLocator points to the external source of a session/turn. Path is
// local-only and must be validated against user-configured roots before use;
// the repository cannot widen those roots. Its content is never executed.
type TranscriptLocator struct {
	Kind       string `json:"kind"` // sqlite, jsonl, rollout, file, api
	Path       string `json:"path"`
	SessionID  string `json:"session_id"`
	TurnID     string `json:"turn_id"`
	Offset     int64  `json:"offset"`
	SourceHash string `json:"source_hash"`
}

// DetectorIdentity records which detector produced an event. Host-LLM output is
// untrusted: it cannot raise evidence to strong nor trigger automatic promotion.
type DetectorIdentity struct {
	Kind       string `json:"kind"` // deterministic, host_llm
	Name       string `json:"name"`
	Version    string `json:"version"`
	Model      string `json:"model"`       // host_llm only
	PromptHash string `json:"prompt_hash"` // host_llm only
}

// --- Entities -------------------------------------------------

// ExperienceSession is one discovered session from a platform source.
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

// ExperienceTurn is one turn inside a session. Full content is not stored by
// default; SafeSummary is optional and bounded. A revision produces a new
// SourceRevision, never a duplicate.
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

// ExperienceEvent is a structured observation extracted from a turn. It always
// keeps its TurnID for provenance. Observation and interpretation stay separate.
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
	Confidence   Confidence
	CreatedAt    time.Time
}

// IngestionCursor is the reconstructible checkpoint for one source instance. It
// is advanced only after the ingestion transaction commits.
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
	// SourceOrder is the adapter-supplied monotonic position of this checkpoint.
	// It is separate from the local revision so a late source read cannot win.
	SourceOrder int64
}
