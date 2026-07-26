// Package trace implements the progressive trace bridge (Hito 4) that
// resolves a Learning back to its source ExperienceEvents. It is the
// last piece of Ola 1: every promoted Learning carries a provenance
// chain the agent can inspect via learning_trace without ever seeing
// raw transcript content by default.
//
// See docs/20-EXPERIENCE-INGESTION-PRD.md §CU-E03 and
// docs/24-EXPERIENCE-THREAT-MODEL.md §4.
package trace

import (
	"time"

	"agent-royo-learn/internal/domain"
)

// RelationshipType classifies the provenance link between a Learning
// and an ExperienceEvent. The enum is closed at the CHECK constraint
// in migration 006; new values require a migration amendment.
type RelationshipType string

const (
	// RelationSource means the event was the direct evidence used during
	// pattern qualification and promotion.
	RelationSource RelationshipType = "source"

	// RelationDerived means the event was indirectly linked (e.g. a
	// previous occurrence of the same fingerprint in a different session).
	RelationDerived RelationshipType = "derived"

	// RelationReferenced means the event was explicitly referenced but
	// was not part of the primary evidence chain.
	RelationReferenced RelationshipType = "referenced"
)

// IsValidRelationshipType reports whether the supplied value is known.
func IsValidRelationshipType(r RelationshipType) bool {
	switch r {
	case RelationSource, RelationDerived, RelationReferenced:
		return true
	}
	return false
}

// LearningEvent is the join row linking a Learning to a source event.
type LearningEvent struct {
	LearningID       domain.LearningID
	EventID          domain.ExperienceEventID
	RelationshipType RelationshipType
	CreatedAt        time.Time
}

// TraceBounds parameterises a trace request. MaxExcerptBytes is a hard
// cap on the excerpt returned per event; 0 means use a sensible default.
type TraceBounds struct {
	MaxExcerptBytes int
	IncludeExcerpt  bool
}

// TraceResult is the output of a trace call. Events lists the source
// events in insertion order; for each event an optional redacted
// excerpt is included when IncludeExcerpt was set. Summary carries a
// human-readable count and the trace outcome code.
type TraceResult struct {
	LearningID domain.LearningID `json:"learning_id"`
	Events     []TracedEvent     `json:"events"`
	Summary    TraceSummary      `json:"summary"`
}

// TracedEvent pairs a domain event with an optional redacted excerpt.
type TracedEvent struct {
	Event           domain.ExperienceEvent `json:"event"`
	Excerpt         string                 `json:"excerpt,omitempty"`
	ExcerptRedacted bool                   `json:"excerpt_redacted"`
	TraceCode       string                 `json:"trace_code,omitempty"`
}

// TraceSummary gives the caller a quick overview of the trace result.
type TraceSummary struct {
	TotalEvents  int    `json:"total_events"`
	WithExcerpts int    `json:"with_excerpts"`
	Code         string `json:"code"` // "" = ok, or an error code
	Message      string `json:"message,omitempty"`
}
