package storage

import (
	"context"
	"database/sql"
	"fmt"

	"agent-royo-learn/internal/domain"
)

// The Get* helpers disagree on how they signal a missing row (some return a
// typed not-found error, some return sql.ErrNoRows). Import needs one
// unambiguous "does this id already exist?" answer per entity, so these helpers
// return a plain (bool, error) with a nil error when the row is simply absent.

// ProjectExists reports whether a project with the given ID is stored.
func ProjectExists(ctx context.Context, tx *sql.Tx, id domain.ProjectID) (bool, error) {
	return rowExists(ctx, tx, "SELECT 1 FROM projects WHERE id = ?", string(id))
}

// LearningHash returns the stored normalized hash of a learning and whether it
// exists. It is the conflict-detection primitive for import (plan 4.6).
func LearningHash(ctx context.Context, tx *sql.Tx, id domain.LearningID) (hash string, found bool, err error) {
	err = tx.QueryRowContext(ctx, "SELECT normalized_hash FROM learnings WHERE id = ?", string(id)).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("LearningHash: %w", err)
	}
	return hash, true, nil
}

// EvidenceExists reports whether an evidence row with the given ID is stored.
func EvidenceExists(ctx context.Context, tx *sql.Tx, id domain.EvidenceID) (bool, error) {
	return rowExists(ctx, tx, "SELECT 1 FROM evidence WHERE id = ?", string(id))
}

// RelationExists reports whether a relation with the given ID is stored.
func RelationExists(ctx context.Context, tx *sql.Tx, id domain.RelationID) (bool, error) {
	return rowExists(ctx, tx, "SELECT 1 FROM learning_relations WHERE id = ?", string(id))
}

func rowExists(ctx context.Context, tx *sql.Tx, query, arg string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, query, arg).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("rowExists: %w", err)
	}
	return true, nil
}
