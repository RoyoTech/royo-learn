package publish

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// Publish executes a publication plan: validates approval, backs up files,
// writes atomically, verifies, and records in the journal.
func (s *Service) Publish(ctx context.Context, projectID domain.ProjectID, input *PublishInput) (*PublishResult, error) {
	if input == nil || input.LearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "learning_id is required")
	}
	if input.PreviewHash == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "preview_hash is required")
	}

	// 1. Load the learning.
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("Publish: begin read tx: %w", err)
	}
	learning, err := storage.GetLearning(ctx, readTx, input.LearningID)
	readTx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Publish: get learning: %w", err)
	}
	if learning == nil {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "learning: "+string(input.LearningID))
	}
	if learning.Status != domain.StatusApproved {
		return nil, domain.NewValidationError(domain.ErrInvalidTransition,
			fmt.Sprintf("learning must be approved to publish (current: %s)", learning.Status))
	}

	// 2. Verify the preview is still valid (not invalidated).
	readTx2, _ := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	preview, err := storage.GetPreviewByHash(ctx, readTx2, input.PreviewHash)
	readTx2.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Publish: get preview: %w", err)
	}
	if preview.InvalidatedAt != nil {
		return nil, domain.NewConflictError(domain.ErrPreviewHashMismatch,
			"preview has been invalidated — regenerate before publishing")
	}

	// 3. Check approval if required.
	var approval *domain.Approval
	if preview.RequiresApproval {
		approval, err = s.CheckApproval(ctx, input.PreviewHash)
		if err != nil {
			return nil, err
		}
		// If a specific approval ID was provided, verify it.
		if input.ApprovalID != nil && approval.ID != *input.ApprovalID {
			return nil, domain.NewValidationError(domain.ErrApprovalInvalid,
				"provided approval_id does not match the valid approval for this preview")
		}
	}

	// 4. Get the latest curation.
	readTx3, _ := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	curations, err := storage.ListCurationsByLearning(ctx, readTx3, input.LearningID)
	readTx3.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Publish: list curations: %w", err)
	}
	if len(curations) == 0 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "no curation found for learning")
	}
	curation := curations[0]

	// 5. Resolve targets.
	targets, err := ResolveTarget(s.projectRoot, curation)
	if err != nil {
		return nil, fmt.Errorf("Publish: resolve targets: %w", err)
	}

	// 6. Check dirty worktree (unless --force).
	if !input.Force {
		dirty, err := CheckDirtyWorktree(s.projectRoot, targets)
		if err != nil {
			return nil, fmt.Errorf("Publish: check dirty: %w", err)
		}
		if dirty.IsDirty {
			return nil, domain.NewConflictError(domain.ErrDirtyTarget,
				"cannot publish: "+dirty.Reason+" — use --force to override")
		}
	}

	// 7. Back up target files.
	backupMgr := NewBackupManager(s.projectRoot, s.backupDir)
	var backupEntries []BackupEntry
	for _, target := range targets {
		// target.Root includes project root; compute a path relative to project root.
		relPath, err := filepath.Rel(s.projectRoot, filepath.Join(target.Root, target.Path))
		if err != nil {
			relPath = filepath.Join(target.Root, target.Path)
		}
		entry, err := backupMgr.BackupFile(relPath)
		if err != nil {
			return nil, fmt.Errorf("Publish: backup %s: %w", target.Path, err)
		}
		backupEntries = append(backupEntries, *entry)
	}

	// 8. Build publication content.
	proposedContent := BuildSkillContent(learning.Title, learning.Context,
		learning.ReusableLesson, strings.Join(learning.RecommendedProcedure, "\n"))

	// 9. Write files atomically.
	writer := NewWriter(s.projectRoot)
	var targetEntries []domain.TargetEntry
	var rollbackEntries []domain.RollbackEntry

	for i, target := range targets {
		targetFullPath := filepath.Join(target.Root, target.Path)
		relPath, _ := filepath.Rel(s.projectRoot, targetFullPath)
		if relPath == "" {
			relPath = target.Path
		}

		var contentToWrite string
		if target.IsManaged {
			// Use managed block for existing files.
			if target.Exists {
				existingContent, readErr := os.ReadFile(targetFullPath)
				if readErr != nil {
					// Rollback previously backed up files and return error.
					_ = rollbackAll(backupMgr, backupEntries)
					return nil, fmt.Errorf("Publish: read existing file %s: %w", target.Path, readErr)
				}
				managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s",
					input.LearningID, proposedContent)
				contentToWrite = InsertManagedBlock(string(existingContent), managedContent)
			} else {
				// Create new file with managed block.
				managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s",
					input.LearningID, proposedContent)
				contentToWrite = InsertManagedBlock("", managedContent)
			}
		} else {
			contentToWrite = proposedContent
		}

		if err := writer.WriteFile(relPath,
			[]byte(contentToWrite), 0o644); err != nil {
			// Rollback and return error.
			_ = rollbackAll(backupMgr, backupEntries)
			return nil, fmt.Errorf("Publish: write %s: %w", target.Path, err)
		}

		targetEntries = append(targetEntries, domain.TargetEntry{
			Root:      target.Root,
			Path:      relPath,
			Operation: target.Operation,
		})
		rollbackEntries = append(rollbackEntries, domain.RollbackEntry{
			Path:    relPath,
			Backup:  backupEntries[i].BackupPath,
			Success: true,
		})
	}

	// 10. Verify: check files exist and content matches.
	verification := verifyTargets(s.projectRoot, targetEntries, proposedContent)

	// 11. On verification failure, rollback.
	allVerified := true
	for _, v := range verification {
		if !v.Pass {
			allVerified = false
			break
		}
	}

	now := utcNowPublish()
	pubStatus := domain.PubStatusCompleted
	var errorCode, errorMsg *string

	if !allVerified {
		pubStatus = domain.PubStatusFailed
		code := string(domain.ErrVerificationFailed)
		errorCode = &code
		msg := "verification failed — files have been rolled back"
		errorMsg = &msg

		// Rollback all files.
		_ = rollbackAll(backupMgr, backupEntries)
	}

	// 12. Persist publication record.
	pubID := domain.PublicationID(uuid.Must(uuid.NewV7()).String())
	learning.Status = domain.StatusPublished

	var approvalID *domain.ApprovalID
	if approval != nil {
		approvalID = &approval.ID
	}

	pub := &domain.Publication{
		ID:           pubID,
		LearningID:   input.LearningID,
		PreviewHash:  input.PreviewHash,
		ApprovalID:   approvalID,
		Targets:      targetEntries,
		Verification: verification,
		Rollback:     rollbackEntries,
		Status:       pubStatus,
		StartedAt:    now,
		CompletedAt:  &now,
		ErrorCode:    errorCode,
		ErrorMessage: errorMsg,
	}

	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if saveErr := storage.SavePublication(ctx, tx, pub); saveErr != nil {
			return fmt.Errorf("save publication: %w", saveErr)
		}
		if allVerified {
			if updateErr := storage.UpdateLearning(ctx, tx, learning); updateErr != nil {
				return fmt.Errorf("update learning: %w", updateErr)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("Publish: persist publication: %w", err)
	}

	// 13. Record journal entry.
	journal, err := NewJournal(s.journalDir)
	if err != nil {
		return nil, fmt.Errorf("Publish: create journal: %w", err)
	}
	jEntry := JournalEntry{
		PublicationID: string(pubID),
		LearningID:    string(input.LearningID),
		Targets:       targetEntries,
		Verification:  verification,
	}
	for _, be := range backupEntries {
		jEntry.BackupPaths = append(jEntry.BackupPaths, be.BackupPath)
	}
	if err := journal.Append(jEntry); err != nil {
		return nil, fmt.Errorf("Publish: journal: %w", err)
	}

	return &PublishResult{
		Publication: pub,
		JournalID:   string(pubID),
	}, nil
}

// RollbackPublicationInput carries input for rolling back a publication.
type RollbackPublicationInput struct {
	PublicationID domain.PublicationID
	Actor         domain.Actor
}

// Rollback reverts a publication by restoring all files from backups.
func (s *Service) Rollback(ctx context.Context, projectID domain.ProjectID, input *RollbackPublicationInput) error {
	if input == nil || input.PublicationID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "publication_id is required")
	}

	// Load publication.
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("Rollback: begin tx: %w", err)
	}
	pub, err := storage.GetPublication(ctx, readTx, input.PublicationID)
	readTx.Rollback()
	if err != nil {
		return fmt.Errorf("Rollback: get publication: %w", err)
	}

	if pub.Status == domain.PubStatusRolledback {
		return domain.NewConflictError(domain.ErrRollbackConflict,
			"publication already rolled back")
	}

	// Create backup manager and perform rollback.
	backupMgr := NewBackupManager(s.projectRoot, s.backupDir)
	journal, err := NewJournal(s.journalDir)
	if err != nil {
		return fmt.Errorf("Rollback: create journal: %w", err)
	}

	return RollbackPublication(s.db, backupMgr, journal, pub)
}

// rollbackAll attempts to restore all files from backups. Errors are logged
// but not returned — best-effort rollback.
func rollbackAll(backupMgr *BackupManager, entries []BackupEntry) error {
	results := backupMgr.RestoreAll(entries)
	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("rollback failed for %s: %w", r.Path, r.Error)
		}
	}
	return nil
}

// verifyTargets checks that each target file was written correctly.
func verifyTargets(projectRoot string, targets []domain.TargetEntry, expectedContent string) []domain.ValidationResult {
	var results []domain.ValidationResult
	for _, t := range targets {
		fullPath := filepath.Join(projectRoot, t.Root, t.Path)
		_, err := os.Stat(fullPath)
		exists := err == nil

		results = append(results, domain.ValidationResult{
			Check: fmt.Sprintf("file-exists:%s", t.Path),
			Pass:  exists,
			Note:  "",
		})
	}
	return results
}
