package trace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// Repository owns every read and write against the learning_events
// join table. It is the only surface that should touch that table
// directly; higher-level Service wraps it.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a Repository backed by the supplied *sql.DB.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// SaveLearningEvent inserts a Learning↔Event link. It is idempotent on
// (learning_id, event_id): a duplicate insert is silently ignored
// (INSERT OR IGNORE). The relationship_type is NOT updated on a
// duplicate; callers must ensure the correct type on first write.
func (r *Repository) SaveLearningEvent(ctx context.Context, le LearningEvent) error {
	if le.LearningID == "" || le.EventID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "trace: learning_id and event_id are required")
	}
	if !IsValidRelationshipType(le.RelationshipType) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "trace: unknown relationship_type")
	}
	if le.CreatedAt.IsZero() {
		le.CreatedAt = time.Now().UTC()
	}

	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO learning_events (learning_id, event_id, relationship_type, created_at)
		 VALUES (?, ?, ?, ?)`,
		string(le.LearningID), string(le.EventID), string(le.RelationshipType), formatTime(le.CreatedAt))
	if err != nil {
		return fmt.Errorf("trace: save learning event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("trace: commit save learning event: %w", err)
	}
	return nil
}

// FindEventsByLearning returns all events linked to the given Learning,
// ordered by created_at ASC so the earliest evidence comes first.
func (r *Repository) FindEventsByLearning(ctx context.Context, learningID domain.LearningID) ([]LearningEvent, error) {
	if learningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "trace: learning_id is required")
	}

	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT learning_id, event_id, relationship_type, created_at
		 FROM learning_events WHERE learning_id = ?
		 ORDER BY created_at ASC`, string(learningID))
	if err != nil {
		return nil, fmt.Errorf("trace: find events by learning: %w", err)
	}
	defer rows.Close()

	var out []LearningEvent
	for rows.Next() {
		var le LearningEvent
		var createdAt string
		if err := rows.Scan(
			(*string)(&le.LearningID),
			(*string)(&le.EventID),
			(*string)(&le.RelationshipType),
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("trace: scan learning event: %w", err)
		}
		le.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, le)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trace: iterate learning events: %w", err)
	}
	if out == nil {
		out = []LearningEvent{}
	}
	return out, nil
}

// FindLearningsByEvent returns all learnings linked to the given event.
func (r *Repository) FindLearningsByEvent(ctx context.Context, eventID domain.ExperienceEventID) ([]LearningEvent, error) {
	if eventID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "trace: event_id is required")
	}

	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT learning_id, event_id, relationship_type, created_at
		 FROM learning_events WHERE event_id = ?
		 ORDER BY created_at ASC`, string(eventID))
	if err != nil {
		return nil, fmt.Errorf("trace: find learnings by event: %w", err)
	}
	defer rows.Close()

	var out []LearningEvent
	for rows.Next() {
		var le LearningEvent
		var createdAt string
		if err := rows.Scan(
			(*string)(&le.LearningID),
			(*string)(&le.EventID),
			(*string)(&le.RelationshipType),
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("trace: scan learning event: %w", err)
		}
		le.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, le)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trace: iterate learnings by event: %w", err)
	}
	if out == nil {
		out = []LearningEvent{}
	}
	return out, nil
}

func (r *Repository) resolveTx(ctx context.Context, writable bool) (*sql.Tx, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("trace: database is nil")
	}
	return r.db.BeginTx(ctx, nil)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
