package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"

	"github.com/google/uuid"
)

func TestSaveAndGetApproval(t *testing.T) {
	db, proj := setupTestDB(t)
	ctx := context.Background()

	l := newTestLearning(proj.ID)
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	approval := &domain.Approval{
		ID:               domain.ApprovalID(uuid.Must(uuid.NewV7()).String()),
		LearningID:       l.ID,
		PreviewHash:      "preview-hash-123",
		ApprovedBy:       "test-user",
		Reason:           "Looks good",
		ApprovalEvidence: "human approved via CLI",
		CreatedAt:        utcNow(),
	}

	t.Run("save and get", func(t *testing.T) {
		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return SaveApproval(ctx, tx, approval)
		})
		if err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetApproval(ctx, readTx, approval.ID)
		if err != nil {
			t.Fatalf("GetApproval: %v", err)
		}
		if got.ApprovedBy != "test-user" {
			t.Fatalf("approved_by = %q, want %q", got.ApprovedBy, "test-user")
		}
		if got.PreviewHash != "preview-hash-123" {
			t.Fatalf("preview_hash = %q", got.PreviewHash)
		}
	})

	t.Run("get by hash", func(t *testing.T) {
		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetApprovalByHash(ctx, readTx, "preview-hash-123")
		if err != nil {
			t.Fatalf("GetApprovalByHash: %v", err)
		}
		if got.ID != approval.ID {
			t.Fatalf("id mismatch: got %q, want %q", got.ID, approval.ID)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return RevokeApproval(ctx, tx, approval.ID)
		})
		if err != nil {
			t.Fatalf("RevokeApproval: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetApproval(ctx, readTx, approval.ID)
		if err != nil {
			t.Fatalf("GetApproval after revoke: %v", err)
		}
		if got.RevokedAt == nil {
			t.Fatal("expected RevokedAt to be set")
		}

		// GetApprovalByHash should NOT return revoked approvals.
		_, err = GetApprovalByHash(ctx, readTx, "preview-hash-123")
		if err == nil {
			t.Fatal("GetApprovalByHash should not return revoked approval")
		}
	})

	t.Run("expiry", func(t *testing.T) {
		expiry := time.Now().UTC().Add(1 * time.Hour)
		expApproval := &domain.Approval{
			ID:               domain.ApprovalID(uuid.Must(uuid.NewV7()).String()),
			LearningID:       l.ID,
			PreviewHash:      "preview-hash-expiry",
			ApprovedBy:       "test-user",
			Reason:           "Expiring approval",
			ApprovalEvidence: "",
			CreatedAt:        utcNow(),
			ExpiresAt:        &expiry,
		}
		if err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return SaveApproval(ctx, tx, expApproval)
		}); err != nil {
			t.Fatalf("SaveApproval with expiry: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetApproval(ctx, readTx, expApproval.ID)
		if err != nil {
			t.Fatalf("GetApproval: %v", err)
		}
		if got.ExpiresAt == nil {
			t.Fatal("expected ExpiresAt to be saved")
		}
	})
}

func TestSaveAndGetPublication(t *testing.T) {
	db, proj := setupTestDB(t)
	ctx := context.Background()

	l := newTestLearning(proj.ID)
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Create an approval first so FK is satisfied.
	approvalID := domain.ApprovalID(uuid.Must(uuid.NewV7()).String())
	approval := &domain.Approval{
		ID:               approvalID,
		LearningID:       l.ID,
		PreviewHash:      "preview-hash-pub",
		ApprovedBy:       "test-user",
		Reason:           "approved",
		ApprovalEvidence: "",
		CreatedAt:        utcNow(),
	}
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return SaveApproval(ctx, tx, approval)
	}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	pub := &domain.Publication{
		ID:          domain.PublicationID(uuid.Must(uuid.NewV7()).String()),
		LearningID:  l.ID,
		PreviewHash: "preview-hash-pub",
		ApprovalID:  &approvalID,
		Targets: []domain.TargetEntry{
			{Root: "/test", Path: "/skill.md", Operation: domain.OpCreate},
		},
		Verification: []domain.ValidationResult{
			{Check: "file-exists", Pass: true},
		},
		Rollback: []domain.RollbackEntry{
			{Path: "/skill.md", Backup: "/backup/skill.md", Success: true},
		},
		Status:       domain.PubStatusCompleted,
		StartedAt:    utcNow(),
		CompletedAt:  nil,
		ErrorCode:    nil,
		ErrorMessage: nil,
	}

	t.Run("save and get", func(t *testing.T) {
		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return SavePublication(ctx, tx, pub)
		})
		if err != nil {
			t.Fatalf("SavePublication: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetPublication(ctx, readTx, pub.ID)
		if err != nil {
			t.Fatalf("GetPublication: %v", err)
		}
		if got.Status != domain.PubStatusCompleted {
			t.Fatalf("status = %q, want %q", got.Status, domain.PubStatusCompleted)
		}
		if len(got.Targets) != 1 {
			t.Fatalf("targets len = %d, want 1", len(got.Targets))
		}
		if got.Targets[0].Path != "/skill.md" {
			t.Fatalf("target path = %q", got.Targets[0].Path)
		}
		if len(got.Verification) != 1 {
			t.Fatalf("verification len = %d, want 1", len(got.Verification))
		}
		if len(got.Rollback) != 1 {
			t.Fatalf("rollback len = %d, want 1", len(got.Rollback))
		}
		if got.ApprovalID == nil || *got.ApprovalID != approvalID {
			t.Fatalf("approval_id mismatch")
		}
	})

	t.Run("update", func(t *testing.T) {
		now := utcNow()
		pub.Status = domain.PubStatusFailed
		pub.CompletedAt = &now
		errCode := "verification_failed"
		pub.ErrorCode = &errCode
		errMsg := "file not found"
		pub.ErrorMessage = &errMsg

		err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return UpdatePublication(ctx, tx, pub)
		})
		if err != nil {
			t.Fatalf("UpdatePublication: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		got, err := GetPublication(ctx, readTx, pub.ID)
		if err != nil {
			t.Fatalf("GetPublication: %v", err)
		}
		if got.Status != domain.PubStatusFailed {
			t.Fatalf("status after update = %q", got.Status)
		}
		if got.ErrorCode == nil || *got.ErrorCode != "verification_failed" {
			t.Fatalf("error_code after update = %v", got.ErrorCode)
		}
	})

	t.Run("list by learning", func(t *testing.T) {
		// Save another publication without approval for the same learning.
		pub2 := &domain.Publication{
			ID:          domain.PublicationID(uuid.Must(uuid.NewV7()).String()),
			LearningID:  l.ID,
			PreviewHash: "preview-hash-pub2",
			ApprovalID:  nil,
			Targets:     []domain.TargetEntry{},
			Status:      domain.PubStatusPending,
			StartedAt:   utcNow(),
		}
		if err := WithTx(ctx, db, func(tx *sql.Tx) error {
			return SavePublication(ctx, tx, pub2)
		}); err != nil {
			t.Fatalf("SavePublication 2: %v", err)
		}

		readTx, _ := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		defer readTx.Rollback()
		list, err := ListPublicationsByLearning(ctx, readTx, l.ID)
		if err != nil {
			t.Fatalf("ListPublicationsByLearning: %v", err)
		}
		if len(list) < 2 {
			t.Fatalf("ListPublicationsByLearning returned %d, want at least 2", len(list))
		}
	})
}
