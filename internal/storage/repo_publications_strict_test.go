package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestGetPublicationRejectsMalformedRollbackJSONWithoutErasingIt(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	learning := newTestLearning(project.ID)
	publication := &domain.Publication{
		ID:          "strict-publication",
		LearningID:  learning.ID,
		PreviewHash: "strict-preview",
		Status:      domain.PubStatusInProgress,
		StartedAt:   utcNow(),
	}
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveLearning(ctx, tx, learning); err != nil {
			return err
		}
		return SavePublication(ctx, tx, publication)
	}); err != nil {
		t.Fatalf("seed publication: %v", err)
	}
	const malformed = `{"unterminated":`
	if _, err := db.DB.Exec(`UPDATE publications SET rollback_json = ? WHERE id = ?`, malformed, string(publication.ID)); err != nil {
		t.Fatalf("inject malformed rollback metadata: %v", err)
	}

	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin read transaction: %v", err)
	}
	_, getErr := GetPublication(ctx, tx, publication.ID)
	tx.Rollback()
	var metadataErr *PublicationMetadataError
	if !errors.As(getErr, &metadataErr) || metadataErr.Field != "rollback_json" || metadataErr.Raw != malformed {
		t.Fatalf("GetPublication error = %#v, want typed rollback metadata error", getErr)
	}

	var raw string
	if err := db.DB.QueryRow(`SELECT rollback_json FROM publications WHERE id = ?`, string(publication.ID)).Scan(&raw); err != nil {
		t.Fatalf("read raw rollback metadata: %v", err)
	}
	if raw != malformed {
		t.Fatalf("stored metadata changed: got %q want %q", raw, malformed)
	}
}
