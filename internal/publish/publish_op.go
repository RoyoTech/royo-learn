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
	readTx2, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("Publish: begin read tx (preview): %w", err)
	}
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
	readTx3, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("Publish: begin read tx (curation): %w", err)
	}
	curations, err := storage.ListCurationsByLearning(ctx, readTx3, input.LearningID)
	readTx3.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Publish: list curations: %w", err)
	}
	if len(curations) == 0 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "no curation found for learning")
	}
	curation := curations[0]

	// Resolve target context for skill destinations.
	// Only auto-derive skill paths when the curation destination is generic
	// (no specific directory). If the curator already set an explicit path
	// (e.g., "my-skill/SKILL.md"), use the old single-target behavior.
	var targetCtx *TargetContext
	if curation.Destination != nil && curation.Destination.Type == domain.DestSkill {
		proj, projErr := s.loadProject(ctx, projectID)
		if projErr == nil {
			area := SkillArea(learning)
			autoName := SkillName(proj.ProjectKey, area)
			destDir := filepath.Dir(curation.Destination.Path)

			// Activate multi-target only when the destination path matches
			// the auto-derived pattern or is generic.
			if destDir == "." || destDir == "" || destDir == autoName {
				needHook, _ := s.needAgentsHook(proj.ProjectKey)
				targetCtx = &TargetContext{
					ProjectKey:     proj.ProjectKey,
					NeedAgentsHook: needHook,
				}
				// Set path to just the skill directory name; ResolveSkillPublishTargets
				// appends "SKILL.md" internally via SkillPath().
				curation.Destination.Path = autoName
			}
		}
	}

	// 5. Resolve targets.
	targets, err := ResolveTarget(s.projectRoot, curation, targetCtx)
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
		// target.Root is RELATIVE to projectRoot; join gives the full relative path.
		relPath := filepath.Join(target.Root, target.Path)
		entry, err := backupMgr.BackupFile(relPath)
		if err != nil {
			return nil, fmt.Errorf("Publish: backup %s: %w", target.Path, err)
		}
		backupEntries = append(backupEntries, *entry)
	}

	// 7b. Capture file hashes after backup for optimistic locking (M3).
	fileHashes := make(map[string]string)
	for _, target := range targets {
		if target.Exists {
			relPath := filepath.Join(target.Root, target.Path)
			targetFullPath := filepath.Join(s.projectRoot, relPath)
			h, err := HashFile(targetFullPath)
			if err != nil {
				return nil, fmt.Errorf("Publish: hash target %s: %w", target.Path, err)
			}
			fileHashes[relPath] = h
		}
	}

	// 8. Build per-target content.
	targetContents := s.buildPublishContents(ctx, targets, learning, curation, targetCtx)

	// 9. Write files atomically.
	writer := NewWriter(s.projectRoot)
	var targetEntries []domain.TargetEntry
	var rollbackEntries []domain.RollbackEntry
	writtenContents := make(map[string]string)

	for i, target := range targets {
		relPath := filepath.Join(target.Root, target.Path)
		targetFullPath := filepath.Join(s.projectRoot, relPath)

		var contentToWrite string

		if target.IsManaged {
			// Managed blocks: insert/merge into existing content.
			if target.Exists {
				existingContent, readErr := os.ReadFile(targetFullPath)
				if readErr != nil {
					if rbErr := rollbackAll(backupMgr, backupEntries); rbErr != nil {
						return nil, fmt.Errorf("Publish: read existing file %s: %w (rollback also failed: %v)",
							target.Path, readErr, rbErr)
					}
					return nil, fmt.Errorf("Publish: read existing file %s: %w", target.Path, readErr)
				}

				// For AGENTS.md, insert the reference line.
				if target.Path == "AGENTS.md" && targetCtx != nil {
					newContent, _ := InsertAgentsRef(string(existingContent), targetCtx.ProjectKey)
					contentToWrite = newContent
				} else {
					managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s",
						input.LearningID, targetContents[i].Content)
					contentToWrite = InsertManagedBlock(string(existingContent), managedContent)
				}
			} else {
				// Create new file with managed block.
				if target.Path == "AGENTS.md" && targetCtx != nil {
					contentToWrite = BuildAgentsRefManagedBlock(targetCtx.ProjectKey)
					if !strings.HasSuffix(contentToWrite, "\n") {
						contentToWrite += "\n"
					}
				} else {
					managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s",
						input.LearningID, targetContents[i].Content)
					contentToWrite = InsertManagedBlock("", managedContent)
				}
			}
		} else {
			contentToWrite = targetContents[i].Content
		}

		// 9b. Optimistic locking (M3).
		if !input.Force && target.Exists {
			changed, err := hashChanged(targetFullPath, relPath, fileHashes)
			if err != nil {
				if rbErr := rollbackAll(backupMgr, backupEntries); rbErr != nil {
					return nil, fmt.Errorf("Publish: %w (rollback also failed: %v)", err, rbErr)
				}
				return nil, fmt.Errorf("Publish: %w", err)
			}
			if changed {
				conflictErr := domain.NewConflictError(domain.ErrDirtyTarget,
					fmt.Sprintf("target file was modified after backup: %s — retry or use --force", target.Path))
				if rbErr := rollbackAll(backupMgr, backupEntries); rbErr != nil {
					return nil, fmt.Errorf("Publish: %w (rollback also failed: %v)", conflictErr, rbErr)
				}
				return nil, conflictErr
			}
		}

		if err := writer.WriteFile(relPath,
			[]byte(contentToWrite), 0o644); err != nil {
			if rbErr := rollbackAll(backupMgr, backupEntries); rbErr != nil {
				return nil, fmt.Errorf("Publish: write %s: %w (rollback also failed: %v)",
					target.Path, err, rbErr)
			}
			return nil, fmt.Errorf("Publish: write %s: %w", target.Path, err)
		}

		targetEntries = append(targetEntries, domain.TargetEntry{
			Root:      target.Root,
			Path:      target.Path,
			Operation: target.Operation,
		})
		rollbackEntries = append(rollbackEntries, domain.RollbackEntry{
			Path:    relPath,
			Backup:  backupEntries[i].BackupPath,
			Success: true,
		})

		writtenContents[relPath] = contentToWrite
	}

	// 10. Verify: check files exist and content hashes match (H1).
	verification := verifyTargets(s.projectRoot, targetEntries, writtenContents)

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

		if rbErr := rollbackAll(backupMgr, backupEntries); rbErr != nil {
			rbCode := "rollback_failed"
			errorCode = &rbCode
			rbMsg := fmt.Sprintf("verification failed AND rollback failed — files may be in corrupt state: %v", rbErr)
			errorMsg = &rbMsg
		}
	}

	// 12. Build publication record.
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

	// 13. Write journal entry BEFORE DB commit (M2).
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

	// 14. Persist publication record (DB commit).
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

	return &PublishResult{
		Publication: pub,
		JournalID:   string(pubID),
	}, nil
}

// buildPublishContents builds the content for each target during publish.
// goCtx is the context.Context for DB operations; ctx is the target context
// (project key, agents hook flag).
func (s *Service) buildPublishContents(goCtx context.Context, targets []TargetResolution, learning *domain.Learning, curation *domain.Curation, ctx *TargetContext) []TargetContent {
	var result []TargetContent

	if ctx == nil || ctx.ProjectKey == "" {
		proposedContent := BuildSkillContent(learning.Title, learning.Context,
			learning.ReusableLesson, strings.Join(learning.RecommendedProcedure, "\n"))
		for _, t := range targets {
			result = append(result, TargetContent{Target: t, Content: proposedContent})
		}
		return result
	}

	indexName := IndexSkillName(ctx.ProjectKey)

	for _, target := range targets {
		targetName := filepath.Base(filepath.Dir(target.Path))

		if targetName == indexName {
			// Index skill: regenerate catalog from discovered child skills.
			entries, err := DiscoverChildSkills(s.projectRoot, ctx.ProjectKey)
			if err != nil {
				entries = nil
			}
			result = append(result, TargetContent{
				Target:  target,
				Content: GenerateIndexContent(ctx.ProjectKey, entries),
			})
			continue
		}

		if target.Path == "AGENTS.md" {
			result = append(result, TargetContent{
				Target:  target,
				Content: BuildAgentsRefManagedBlock(ctx.ProjectKey),
			})
			continue
		}

		// Child skill: rebuild sections from DB (source of truth).
		// The skill file is a PROJECTION of the learnings in the DB, not a
		// round-trip of the markdown body. We read learning_ids from the
		// existing frontmatter, load those learnings from the DB, and rebuild
		// sections from domain.Learning objects. This preserves all fields
		// (including Procedure) without relying on markdown parsing.
		// Fallback: if DB load fails, parse the existing body (graceful degradation).
		var sections []SkillSection
		var existingLearnings []*domain.Learning

		targetFullPath := filepath.Join(s.projectRoot, target.Root, target.Path)
		var existingContent string
		if data, err := os.ReadFile(targetFullPath); err == nil {
			existingContent = string(data)
		}

		if existingContent != "" {
			fm, parseErr := ParseFrontmatter(existingContent)
			if parseErr == nil && len(fm.LearningIDs) > 0 {
				readTx, txErr := s.db.DB.BeginTx(goCtx, &sql.TxOptions{ReadOnly: true})
				if txErr == nil {
					existingLearnings, _ = storage.ListLearningsByIDs(goCtx, readTx, fm.LearningIDs)
					readTx.Rollback()
				}
			}
		}

		if len(existingLearnings) > 0 {
			// DB projection path: rebuild from domain.Learning objects.
			sections = BuildSectionsFromLearnings(existingLearnings, learning)
		} else if existingContent != "" {
			// Fallback: parse the existing body (handles DB failures gracefully).
			sections = MergeLearningIntoSections(parseSkillSections(existingContent), learning)
		} else {
			// New skill: just the new learning.
			sections = MergeLearningIntoSections(nil, learning)
		}

		ids := make([]domain.LearningID, 0, len(sections))
		for _, sec := range sections {
			ids = append(ids, sec.LearningID)
		}

		// Build description from ALL learnings in the skill (triggers from all).
		allLearnings := make([]*domain.Learning, 0, len(existingLearnings)+1)
		allLearnings = append(allLearnings, existingLearnings...)
		allLearnings = append(allLearnings, learning)

		area := SkillArea(learning)
		fm := SkillFrontmatter{
			Name:        SkillName(ctx.ProjectKey, area),
			Description: BuildDescription(ctx.ProjectKey, area, allLearnings),
			Source:      "royo-learn",
			Project:     ctx.ProjectKey,
			LearningIDs: ids,
			UpdatedAt:   utcNowPublish().Format("2006-01-02"),
		}

		result = append(result, TargetContent{
			Target:  target,
			Content: GenerateSkillContent(fm, sections),
		})
	}

	return result
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

// rollbackAll attempts to restore all files from backups. It aggregates ALL
// restore failures into a single error so the caller can surface every
// unrestored file observably (H2).
func rollbackAll(backupMgr *BackupManager, entries []BackupEntry) error {
	results := backupMgr.RestoreAll(entries)
	var failed []string
	for _, r := range results {
		if !r.Success {
			failed = append(failed, fmt.Sprintf("%s: %v", r.Path, r.Error))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("rollback failed for %d file(s): %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}

// verifyTargets checks that each target file was written correctly by
// comparing the on-disk content hash against the expected content hash (H1).
func verifyTargets(projectRoot string, targets []domain.TargetEntry, writtenContents map[string]string) []domain.ValidationResult {
	var results []domain.ValidationResult
	for _, t := range targets {
		fullPath := filepath.Join(projectRoot, t.Root, t.Path)
		key := filepath.Join(t.Root, t.Path)
		expected, ok := writtenContents[key]
		if !ok {
			results = append(results, domain.ValidationResult{
				Check: fmt.Sprintf("content-hash:%s", t.Path),
				Pass:  false,
				Note:  "no expected content provided for target",
			})
			continue
		}

		_, err := os.Stat(fullPath)
		if err != nil {
			results = append(results, domain.ValidationResult{
				Check: fmt.Sprintf("content-hash:%s", t.Path),
				Pass:  false,
				Note:  fmt.Sprintf("file does not exist: %v", err),
			})
			continue
		}

		fileHash, err := HashFile(fullPath)
		if err != nil {
			results = append(results, domain.ValidationResult{
				Check: fmt.Sprintf("content-hash:%s", t.Path),
				Pass:  false,
				Note:  fmt.Sprintf("hash failed: %v", err),
			})
			continue
		}
		expectedHash := HashContent([]byte(expected))
		pass := fileHash == expectedHash
		note := fileHash
		if !pass {
			note = fmt.Sprintf("expected %s, got %s", expectedHash, fileHash)
		}
		results = append(results, domain.ValidationResult{
			Check: fmt.Sprintf("content-hash:%s", t.Path),
			Pass:  pass,
			Note:  note,
		})
	}
	return results
}

// hashChanged returns true if the file at targetFullPath has a different hash
// than the one recorded in fileHashes[relPath] (M3 optimistic locking).
func hashChanged(targetFullPath, relPath string, fileHashes map[string]string) (bool, error) {
	baseline, ok := fileHashes[relPath]
	if !ok {
		return false, nil
	}
	currentHash, err := HashFile(targetFullPath)
	if err != nil {
		return false, fmt.Errorf("re-hash %s: %w", relPath, err)
	}
	return currentHash != baseline, nil
}
