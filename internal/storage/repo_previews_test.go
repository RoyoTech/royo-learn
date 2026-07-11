package storage

import (
	"context"
	"database/sql"
	"testing"

	"agent-royo-learn/internal/domain"

	"github.com/google/uuid"
)

func TestSaveAndGetPreview(t *testing.T) {
	db, proj := setupTestDB(t)
	ctx := context.Background()

	preview := &domain.PublicationPreview{
		ID:         domain.PreviewID(uuid.Must(uuid.NewV7()).String()),
		LearningID: domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		Plan: domain.PublicationPlan{
			LearningID: domain.LearningID("test-learning"),
			TargetPath: "/test/path",
			Operation:  domain.OpCreate,
			Content:    "test content",
		},
		PreviewHash:      "sha256:abc123",
		Risk:             domain.RiskLow,
		RequiresApproval: true,
		CreatedAt:        utcNow(),
	}

	// Also save a learning so FK is satisfied.
	l := newTestLearning(proj.ID)
	preview.LearningID = l.ID
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit learning: %v", err)
	}

	t.Run("save and get", func(t *testing.T) {
		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return SavePreview(ctx, tx, preview)
		})
		if err != nil {
			t.Fatalf("SavePreview: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetPreview(ctx, readTx, preview.ID)
		if err != nil {
			t.Fatalf("GetPreview: %v", err)
		}
		if got == nil {
			t.Fatal("expected preview, got nil")
		}
		if got.PreviewHash != preview.PreviewHash {
			t.Fatalf("hash = %q, want %q", got.PreviewHash, preview.PreviewHash)
		}
		if got.Risk != preview.Risk {
			t.Fatalf("risk = %q, want %q", got.Risk, preview.Risk)
		}
		if got.RequiresApproval != preview.RequiresApproval {
			t.Fatalf("requires_approval = %v, want %v", got.RequiresApproval, preview.RequiresApproval)
		}
	})

	t.Run("get by hash", func(t *testing.T) {
		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetPreviewByHash(ctx, readTx, preview.PreviewHash)
		if err != nil {
			t.Fatalf("GetPreviewByHash: %v", err)
		}
		if got.ID != preview.ID {
			t.Fatalf("id = %q, want %q", got.ID, preview.ID)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		_, err := GetPreview(ctx, readTx, domain.PreviewID("nonexistent"))
		if err == nil {
			t.Fatal("expected error for nonexistent preview")
		}
	})

	t.Run("invalidate", func(t *testing.T) {
		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return InvalidatePreview(ctx, tx, preview.ID)
		})
		if err != nil {
			t.Fatalf("InvalidatePreview: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetPreview(ctx, readTx, preview.ID)
		if err != nil {
			t.Fatalf("GetPreview after invalidate: %v", err)
		}
		if got.InvalidatedAt == nil {
			t.Fatal("expected InvalidatedAt to be set")
		}
	})
}
