package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveCuration inserts a new curation record.
func SaveCuration(ctx context.Context, tx *sql.Tx, c *domain.Curation) error {
	destJSON := "{}"
	if c.Destination != nil {
		destJSON = marshalAny(c.Destination)
	}
	validationJSON := marshalAny(c.Validation)
	checksJSON := marshalAny(c.AcceptanceChecks)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO curations (id, learning_id, decision, rationale, destination_json, validation_json, acceptance_checks_json, rollback_condition, actor_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(c.ID),
		string(c.LearningID),
		string(c.Decision),
		c.Rationale,
		destJSON,
		validationJSON,
		checksJSON,
		c.RollbackCondition,
		c.Actor.ActorJSON(),
		c.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("SaveCuration: %w", err)
	}
	return nil
}

// GetCuration retrieves a curation by ID.
func GetCuration(ctx context.Context, tx *sql.Tx, id domain.CurationID) (*domain.Curation, error) {
	c := &domain.Curation{}
	var createdAt, actorJSON, destJSON, valJSON, checksJSON string
	err := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, decision, rationale, destination_json, validation_json, acceptance_checks_json, rollback_condition, actor_json, created_at
		FROM curations WHERE id = ?
	`, string(id)).Scan(
		(*string)(&c.ID),
		(*string)(&c.LearningID),
		(*string)(&c.Decision),
		&c.Rationale,
		&destJSON,
		&valJSON,
		&checksJSON,
		&c.RollbackCondition,
		&actorJSON,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "curation")
	}
	if err != nil {
		return nil, fmt.Errorf("GetCuration: %w", err)
	}
	c.Destination = unmarshalDestination(destJSON)
	c.Validation = unmarshalValidationResults(valJSON)
	c.AcceptanceChecks = unmarshalChecks(checksJSON)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.Actor = parseActor(actorJSON)
	return c, nil
}

// ListCurationsByLearning returns all curations for a learning.
func ListCurationsByLearning(ctx context.Context, tx *sql.Tx, learningID domain.LearningID) ([]*domain.Curation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, learning_id, decision, rationale, destination_json, validation_json, acceptance_checks_json, rollback_condition, actor_json, created_at
		FROM curations WHERE learning_id = ?
		ORDER BY created_at DESC
	`, string(learningID))
	if err != nil {
		return nil, fmt.Errorf("ListCurationsByLearning: %w", err)
	}
	defer rows.Close()

	var out []*domain.Curation
	for rows.Next() {
		c := &domain.Curation{}
		var createdAt, actorJSON, destJSON, valJSON, checksJSON string
		if err := rows.Scan(
			(*string)(&c.ID),
			(*string)(&c.LearningID),
			(*string)(&c.Decision),
			&c.Rationale,
			&destJSON,
			&valJSON,
			&checksJSON,
			&c.RollbackCondition,
			&actorJSON,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("ListCurationsByLearning scan: %w", err)
		}
		c.Destination = unmarshalDestination(destJSON)
		c.Validation = unmarshalValidationResults(valJSON)
		c.AcceptanceChecks = unmarshalChecks(checksJSON)
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.Actor = parseActor(actorJSON)
		out = append(out, c)
	}
	return out, rows.Err()
}

// unmarshalDestination parses a Destination from JSON, returning nil for empty.
func unmarshalDestination(raw string) *domain.Destination {
	if raw == "" || raw == "{}" {
		return nil
	}
	var d domain.Destination
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil
	}
	return &d
}

// unmarshalValidationResults parses a JSON array of ValidationResult.
func unmarshalValidationResults(raw string) []domain.ValidationResult {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []domain.ValidationResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// unmarshalChecks parses a JSON array of Check.
func unmarshalChecks(raw string) []domain.Check {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []domain.Check
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
