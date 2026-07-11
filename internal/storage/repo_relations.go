package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveRelation inserts a new learning relation.
func SaveRelation(ctx context.Context, tx *sql.Tx, r *domain.LearningRelation) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO learning_relations (id, source_learning_id, target_learning_id, relation, confidence, rationale, actor_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(r.ID),
		string(r.SourceLearningID),
		string(r.TargetLearningID),
		string(r.Relation),
		r.Confidence,
		r.Rationale,
		r.Actor.ActorJSON(),
		r.CreatedAt.Format(time.RFC3339),
		r.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrDuplicateLearning, "relation already exists")
		}
		if isCheckViolation(err) {
			return domain.NewValidationError(domain.ErrInvalidArgument, "self-reference not allowed")
		}
		return fmt.Errorf("SaveRelation: %w", err)
	}
	return nil
}

// ListRelationsBySource returns relations originating from a learning.
func ListRelationsBySource(ctx context.Context, tx *sql.Tx, sourceID domain.LearningID) ([]*domain.LearningRelation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, source_learning_id, target_learning_id, relation, confidence, rationale, actor_json, created_at, updated_at
		FROM learning_relations WHERE source_learning_id = ?
	`, string(sourceID))
	if err != nil {
		return nil, fmt.Errorf("ListRelationsBySource: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows)
}

// ListRelationsByTarget returns relations pointing to a learning.
func ListRelationsByTarget(ctx context.Context, tx *sql.Tx, targetID domain.LearningID) ([]*domain.LearningRelation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, source_learning_id, target_learning_id, relation, confidence, rationale, actor_json, created_at, updated_at
		FROM learning_relations WHERE target_learning_id = ?
	`, string(targetID))
	if err != nil {
		return nil, fmt.Errorf("ListRelationsByTarget: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows)
}

func scanRelations(rows *sql.Rows) ([]*domain.LearningRelation, error) {
	var out []*domain.LearningRelation
	for rows.Next() {
		r := &domain.LearningRelation{}
		var createdAt, updatedAt, actorJSON string
		if err := rows.Scan(
			(*string)(&r.ID),
			(*string)(&r.SourceLearningID),
			(*string)(&r.TargetLearningID),
			(*string)(&r.Relation),
			&r.Confidence,
			&r.Rationale,
			&actorJSON,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanRelations: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		r.Actor = parseActor(actorJSON)
		out = append(out, r)
	}
	return out, rows.Err()
}

// isCheckViolation checks if the error is a SQLite CHECK constraint violation.
func isCheckViolation(err error) bool {
	return err != nil && (contains(err.Error(), "CHECK constraint failed") || contains(err.Error(), "source_learning_id"))
}
