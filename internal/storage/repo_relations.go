package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// relationColumns is the full column list every relation read path selects, so
// the propose/confirm lifecycle (plan 4.5) round-trips.
const relationColumns = `id, source_learning_id, target_learning_id, relation, confidence, rationale,
	actor_json, status, proposed_by_json, confirmed_by_json, confirmed_at, created_at, updated_at`

// SaveRelation inserts a new learning relation. A relation is inserted in its
// current lifecycle state; ProposeRelation creates it as `proposed`.
func SaveRelation(ctx context.Context, tx *sql.Tx, r *domain.LearningRelation) error {
	status := r.Status
	if status == "" {
		status = domain.RelationProposed
	}
	proposedBy := r.ProposedBy
	if proposedBy.Kind == "" && proposedBy.Name == "" {
		proposedBy = r.Actor
	}
	var confirmedByJSON *string
	if r.ConfirmedBy != nil {
		j := r.ConfirmedBy.ActorJSON()
		confirmedByJSON = &j
	}
	var confirmedAt *string
	if r.ConfirmedAt != nil {
		s := r.ConfirmedAt.Format(time.RFC3339)
		confirmedAt = &s
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO learning_relations (id, source_learning_id, target_learning_id, relation, confidence, rationale, actor_json, status, proposed_by_json, confirmed_by_json, confirmed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(r.ID),
		string(r.SourceLearningID),
		string(r.TargetLearningID),
		string(r.Relation),
		r.Confidence,
		r.Rationale,
		r.Actor.ActorJSON(),
		string(status),
		proposedBy.ActorJSON(),
		confirmedByJSON,
		confirmedAt,
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

// GetRelation returns a single relation by ID, or nil if none exists.
func GetRelation(ctx context.Context, tx *sql.Tx, id domain.RelationID) (*domain.LearningRelation, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+relationColumns+` FROM learning_relations WHERE id = ?`, string(id))
	if err != nil {
		return nil, fmt.Errorf("GetRelation: %w", err)
	}
	defer rows.Close()
	list, err := scanRelations(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// ConfirmRelation transitions a proposed relation to confirmed, recording who
// confirmed it and when. It returns ErrLearningNotFound when no proposed
// relation with that ID exists, so a confirmation is never silently faked.
func ConfirmRelation(ctx context.Context, tx *sql.Tx, id domain.RelationID, confirmedBy domain.Actor, at time.Time) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE learning_relations
		SET status = ?, confirmed_by_json = ?, confirmed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`,
		string(domain.RelationConfirmed),
		confirmedBy.ActorJSON(),
		at.Format(time.RFC3339),
		at.Format(time.RFC3339),
		string(id),
		string(domain.RelationProposed),
	)
	if err != nil {
		return fmt.Errorf("ConfirmRelation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ConfirmRelation: rows affected: %w", err)
	}
	if n == 0 {
		return domain.NewNotFoundError(domain.ErrLearningNotFound, "no proposed relation with id "+string(id))
	}
	return nil
}

// ListRelationsBySource returns relations originating from a learning.
func ListRelationsBySource(ctx context.Context, tx *sql.Tx, sourceID domain.LearningID) ([]*domain.LearningRelation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT `+relationColumns+`
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
		SELECT `+relationColumns+`
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
		var createdAt, updatedAt, actorJSON, status, proposedByJSON string
		var confirmedByJSON, confirmedAt *string
		if err := rows.Scan(
			(*string)(&r.ID),
			(*string)(&r.SourceLearningID),
			(*string)(&r.TargetLearningID),
			(*string)(&r.Relation),
			&r.Confidence,
			&r.Rationale,
			&actorJSON,
			&status,
			&proposedByJSON,
			&confirmedByJSON,
			&confirmedAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanRelations: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		r.Actor = parseActor(actorJSON)
		r.Status = domain.RelationStatus(status)
		if proposedByJSON != "" {
			r.ProposedBy = parseActor(proposedByJSON)
		} else {
			r.ProposedBy = r.Actor
		}
		if confirmedByJSON != nil && *confirmedByJSON != "" {
			a := parseActor(*confirmedByJSON)
			r.ConfirmedBy = &a
		}
		if confirmedAt != nil && *confirmedAt != "" {
			t, _ := time.Parse(time.RFC3339, *confirmedAt)
			r.ConfirmedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// isCheckViolation checks if the error is a SQLite CHECK constraint violation.
func isCheckViolation(err error) bool {
	return err != nil && (contains(err.Error(), "CHECK constraint failed") || contains(err.Error(), "source_learning_id"))
}
