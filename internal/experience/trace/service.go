package trace

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// Service implements the progressive trace operation (Hito 4).
// It resolves a Learning back to its source ExperienceEvents via
// the learning_events join table, with optional excerpt resolution.
type Service struct {
	repo *Repository
	db   *sql.DB
}

// NewService wires the trace service with its repository and a DB
// handle for event lookups.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
		db:   db,
	}
}

// Trace returns the provenance chain for a Learning. When bounds
// IncludeExcerpt is false (the default), only event metadata is
// returned — no transcript content is disclosed.
func (s *Service) Trace(ctx context.Context, learningID domain.LearningID, bounds TraceBounds) (*TraceResult, error) {
	if s == nil || s.repo == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "trace: service not initialised")
	}
	if learningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "trace: learning_id is required")
	}

	links, err := s.repo.FindEventsByLearning(ctx, learningID)
	if err != nil {
		return nil, fmt.Errorf("trace: find events: %w", err)
	}

	result := &TraceResult{
		LearningID: learningID,
		Events:     make([]TracedEvent, 0, len(links)),
		Summary: TraceSummary{
			TotalEvents: len(links),
		},
	}

	for _, link := range links {
		event, err := s.getEvent(ctx, link.EventID)
		if err != nil {
			// Degraded: event not found is non-fatal; surface the code
			// per docs/24 §4: "una fuente ausente produce unavailable, no error global".
			result.Events = append(result.Events, TracedEvent{
				TraceCode: "trace_event_unavailable",
			})
			continue
		}

		te := TracedEvent{
			Event: *event,
		}

		if bounds.IncludeExcerpt {
			te.Excerpt = truncateSummary(event.Summary, bounds.MaxExcerptBytes)
		}

		result.Events = append(result.Events, te)
		if te.Excerpt != "" {
			result.Summary.WithExcerpts++
		}
	}

	return result, nil
}

// getEvent reads a single ExperienceEvent by ID.
func (s *Service) getEvent(ctx context.Context, eventID domain.ExperienceEventID) (*domain.ExperienceEvent, error) {
	if s.db == nil {
		return nil, fmt.Errorf("trace: database is nil")
	}

	var (
		id, projectID, turnID, kind, summary, observation, outcome, fingerprint, evidenceJSON string
		detectorJSON                                                                          sql.NullString
		confidence                                                                            string
		createdAt                                                                             string
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, turn_id, kind, summary, observation, outcome,
		        fingerprint, evidence_json, detector_json, confidence, created_at
		 FROM experience_events WHERE id = ?`, string(eventID))
	if err := row.Scan(
		&id, &projectID, &turnID, &kind, &summary, &observation, &outcome,
		&fingerprint, &evidenceJSON, &detectorJSON, &confidence, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("trace: event %s not found", string(eventID))
		}
		return nil, fmt.Errorf("trace: get event: %w", err)
	}

	ev := &domain.ExperienceEvent{
		ID:           domain.ExperienceEventID(id),
		ProjectID:    domain.ProjectID(projectID),
		TurnID:       domain.ExperienceTurnID(turnID),
		Kind:         domain.ExperienceEventKind(kind),
		Summary:      summary,
		Observation:  observation,
		Outcome:      outcome,
		Fingerprint:  fingerprint,
		EvidenceJSON: evidenceJSON,
	}
	if detectorJSON.Valid {
		// Parse detector identity if needed for future use.
		_ = detectorJSON.String
	}
	if conf, err := time.Parse(time.RFC3339, createdAt); err == nil {
		ev.CreatedAt = conf
	}
	return ev, nil
}

func truncateSummary(summary string, maxBytes int) string {
	if maxBytes <= 0 {
		return summary
	}
	if len(summary) <= maxBytes {
		return summary
	}
	return summary[:maxBytes] + "..."
}
