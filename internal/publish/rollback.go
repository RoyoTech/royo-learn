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
		entries[i] = BackupEntry{
			OriginalPath:          re.Path,
			BackupPath:            re.Backup,
			Checksum:              re.BackupSHA256,
			OriginalHash:          re.OriginalSHA256,
			OriginalMode:          re.OriginalMode,
			OriginalExisted:       re.OriginalExisted,
			ExpectedPublishedHash: re.ExpectedPublishedHash,
		}
	}
	return backupMgr.RestoreAll(entries)
}

// RollbackPublication reverts a publication by restoring from backups
// and updating the publication status to rolled_back.
func RollbackPublication(db *storage.DB, backupMgr *BackupManager, journal *Journal, pub *domain.Publication) error {
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

	if err := storage.WithTx(context.Background(), db, func(tx *sql.Tx) error {
		return storage.UpdatePublication(context.Background(), tx, pub)
	}); err != nil {
		return fmt.Errorf("RollbackPublication: update: %w", err)
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
			return fmt.Errorf("RollbackPublication: journal: %w", err)
		}
	}

	return nil
}
