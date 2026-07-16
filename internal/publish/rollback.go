package publish

import (
	"context"
	"database/sql"
	"fmt"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// RollbackFromBackup restores files from backup entries stored in a publication record.
func RollbackFromBackup(backupMgr *BackupManager, rollbackEntries []domain.RollbackEntry) []RestoreResult {
	entries := make([]BackupEntry, len(rollbackEntries))
	for i, re := range rollbackEntries {
		// Compute checksum from the backup file if it exists.
		var checksum string
		if re.Backup != "" {
			if h, err := HashFile(re.Backup); err == nil {
				checksum = h
			}
		}
		entries[i] = BackupEntry{
			OriginalPath: re.Path,
			BackupPath:   re.Backup,
			Checksum:     checksum,
		}
	}
	return backupMgr.RestoreAll(entries)
}

// revokePublished returns a published learning to `approved` after its content
// was restored (D18).
//
// It does not go through domain.MustTransition, and `published -> approved` is
// deliberately absent from domain.ValidTransitions. That table governs CURATION
// actions, and a curator must not be able to un-publish: it would leave
// `approved` on a learning whose file is still written on disk — the false
// state this revocation exists to remove. Only the publication lifecycle
// restores the files and revokes the status together, and it does so as the
// system, exactly as Publish assigns `published` without consulting the table.
func revokePublished(learning *domain.Learning) {
	learning.Status = domain.StatusApproved
	learning.Actor = domain.Actor{Kind: "system", Name: "rollback"}
	learning.Revision++
	learning.UpdatedAt = utcNowPublish()
}

// rollbackPublication reverts a publication by restoring from backups, updating
// the publication status to rolled_back, and revoking the published status of
// the learning it published.
//
// The learning status is part of the rollback, not an afterthought (D18): once
// the files are restored the published content no longer exists, so a learning
// still claiming `published` states something that is not true. It returns to
// `approved` — its curation and its approval are still valid; the only thing
// undone was the write to disk.
//
// A FAILED rollback leaves the learning alone. If the content could not be
// restored, it may well still be published, and inventing `approved` over it
// would trade one false state for another.
func (s *Service) rollbackPublication(backupMgr *BackupManager, journal *Journal, pub *domain.Publication) error {
	if pub.Status == domain.PubStatusRolledback {
		return domain.NewConflictError(domain.ErrRollbackConflict,
			"publication already rolled back: "+string(pub.ID))
	}

	// Restore files from backups.
	results := RollbackFromBackup(backupMgr, pub.Rollback)

	// Check for failures.
	allSuccess := true
	for _, r := range results {
		if !r.Success {
			allSuccess = false
		}
	}

	// Update publication in DB.
	ctx := context.Background()
	now := utcNowPublish()
	if allSuccess {
		pub.Status = domain.PubStatusRolledback
	} else {
		pub.Status = domain.PubStatusFailed
	}
	pub.CompletedAt = &now
	pub.Verification = append(pub.Verification, domain.ValidationResult{
		Check: "rollback",
		Pass:  allSuccess,
		Note:  fmt.Sprintf("restored %d files", len(results)),
	})

	// The publication record and the revoked learning status commit in ONE
	// transaction, mirroring how Publish commits the publication and the
	// `published` status together: either both land or neither does, so no
	// commit failure can leave a rolled-back publication next to a learning
	// that still calls itself published.
	var reverted *domain.Learning
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := storage.UpdatePublication(ctx, tx, pub); err != nil {
			return err
		}
		if !allSuccess {
			return nil
		}
		learning, err := storage.GetLearning(ctx, tx, pub.LearningID)
		if err != nil {
			return fmt.Errorf("get learning: %w", err)
		}
		if learning == nil || learning.Status != domain.StatusPublished {
			// Nothing to revoke: the learning was already moved on (superseded,
			// archived) or is gone. The publication rollback still stands.
			return nil
		}
		revokePublished(learning)
		if err := storage.UpdateLearning(ctx, tx, learning); err != nil {
			return fmt.Errorf("update learning: %w", err)
		}
		reverted = learning
		return nil
	}); err != nil {
		return fmt.Errorf("rollbackPublication: update: %w", err)
	}

	// The truth changed, so the derived record must follow it (D6, D18).
	if reverted != nil {
		if err := s.materializeRecord(reverted); err != nil {
			return fmt.Errorf("rollbackPublication: %w", err)
		}
	}

	// Record journal entry.
	if journal != nil {
		jEntry := JournalEntry{
			PublicationID:  string(pub.ID),
			LearningID:     string(pub.LearningID),
			Targets:        pub.Targets,
			RollbackStatus: string(pub.Status),
		}
		for _, re := range pub.Rollback {
			jEntry.BackupPaths = append(jEntry.BackupPaths, re.Backup)
		}
		if err := journal.Append(jEntry); err != nil {
			return fmt.Errorf("rollbackPublication: journal: %w", err)
		}
	}

	return nil
}
