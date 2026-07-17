package publish

import (
	"context"
	"database/sql"
	"errors"
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
	if learning.Status == domain.StatusPublished && input.Apply {
		return s.reconcilePublished(ctx, input, learning)
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
	if err := validatePreviewIntegrity(preview, input.LearningID); err != nil {
		return nil, err
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
			area, areaErr := ResolveSkillArea(learning, curation.Destination.Area)
			if areaErr != nil {
				return nil, areaErr
			}
			autoName := SkillName(proj.ProjectKey, area)
			destDir := filepath.Dir(curation.Destination.Path)
			explicitArea := curation.Destination.Area != ""

			// Multi-target activates when the curator set an explicit area
			// (the chosen area drives the skill name regardless of the stored
			// path), or when the stored path is generic / already matches the
			// derived name.
			if explicitArea || destDir == "." || destDir == "" || destDir == autoName {
				needHook, _ := s.needAgentsHook(proj.ProjectKey)
				targetCtx = &TargetContext{
					ProjectKey:     proj.ProjectKey,
					NeedAgentsHook: needHook,
					Area:           area,
				}
				// Path holds just the skill directory name;
				// ResolveSkillPublishTargets appends "SKILL.md" internally
				// via SkillPath().
				curation.Destination.Path = autoName
			}
		}
	}

	// 5. Resolve targets.
	targets, err := ResolveTarget(s.projectRoot, curation, targetCtx)
	if err != nil {
		return nil, fmt.Errorf("Publish: resolve targets: %w", err)
	}
	policies := EvaluatePolicies(learning, curation)
	if preview.Plan.PolicySignature != PolicySignature(policies) {
		return nil, previewMismatch("curation or policy changed after preview; regenerate the preview")
	}
	plannedContents, err := validatePlannedTargets(preview, targets)
	if err != nil {
		return nil, err
	}

	// Approval is derived from current resolved impact, never trusted from the
	// persisted boolean alone.
	var approval *domain.Approval
	if RequiresHumanApproval(policies) || requiresSensitiveApproval(targets) {
		if input.ApprovalID == nil || *input.ApprovalID == "" {
			return nil, domain.NewValidationError(domain.ErrApprovalRequired,
				"approval_id is required to publish this preview — obtain one with 'royo-learn approve' / learning_approve")
		}
		approval, err = s.CheckApproval(ctx, input.PreviewHash)
		if err != nil {
			return nil, err
		}
		if approval.ID != *input.ApprovalID || approval.LearningID != input.LearningID {
			return nil, domain.NewValidationError(domain.ErrApprovalInvalid,
				"provided approval_id does not match this learning and preview")
		}
	}

	// Dry-run gate (D7): validation has passed, but a write requires an explicit
	// Apply. Without it, return the plan and touch NO file. This runs AFTER the
	// approval and preview checks so a dry run still reports "this would be
	// blocked" instead of silently succeeding.
	planEntries := make([]domain.TargetEntry, 0, len(targets))
	for _, t := range targets {
		planEntries = append(planEntries, domain.TargetEntry{
			Root: t.Root, Path: t.Path, Operation: t.Operation,
		})
	}
	if !input.Apply {
		return &PublishResult{DryRun: true, Targets: planEntries}, nil
	}
	lock, err := acquirePublicationLock(s.projectRoot, "publish", input.Actor)
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	// From here the plan is fixed. Every failure after the FIRST write must
	// restore the tree byte-for-byte and must NEVER mark the learning published.

	// 6. Validate current hashes: refuse if any destination changed on disk
	// after the preview was taken (prior-hash mismatch). No file has been
	// touched yet, so this is a plain refusal with nothing to compensate.
	if err := s.validateCurrentHashes(preview, targets); err != nil {
		return nil, err
	}

	// 7. Acquire lock: refuse a dirty worktree (unless --force).
	if !input.Force {
		dirty, derr := CheckDirtyWorktree(s.projectRoot, targets)
		if derr != nil {
			return nil, fmt.Errorf("Publish: check dirty: %w", derr)
		}
		if dirty.IsDirty {
			return nil, domain.NewConflictError(domain.ErrDirtyTarget,
				"cannot publish: "+dirty.Reason+" — use --force to override")
		}
	}

	// 8. Capture one snapshot per target and create each backup from those exact
	// bytes. The snapshot also supplies the CAS identity used by the writer.
	backupMgr := NewBackupManager(s.projectRoot, s.backupDir)
	backupEntries := make([]BackupEntry, 0, len(targets))
	snapshots := make([]FileSnapshot, 0, len(targets))
	for _, target := range targets {
		relPath := filepath.Join(target.Root, target.Path)
		snapshot, snapshotErr := backupMgr.SnapshotFile(relPath)
		if snapshotErr != nil {
			return nil, fmt.Errorf("Publish: snapshot %s: %w", target.Path, snapshotErr)
		}
		entry, berr := backupMgr.BackupSnapshot(snapshot)
		if berr != nil {
			return nil, fmt.Errorf("Publish: backup %s: %w", target.Path, berr)
		}
		snapshots = append(snapshots, *snapshot)
		backupEntries = append(backupEntries, *entry)
	}

	// 8b. Apply the exact posterior bytes persisted by the preview. Mutable
	// learning and curation state is never used to recompose authorized output.
	composedContents := plannedContents
	rollbackEntries := make([]domain.RollbackEntry, len(targets))
	for i := range targets {
		contentToWrite := composedContents[i]
		expectedHash := HashContent([]byte(contentToWrite))
		backupEntries[i].ExpectedPublishedHash = expectedHash
		publishedMode := fileModeIdentity(0o644)
		if snapshots[i].Exists {
			publishedMode = fileModeIdentity(snapshots[i].Mode)
		}
		backupEntries[i].ExpectedPublishedMode = &publishedMode
		rollbackEntries[i] = domain.RollbackEntry{
			Path:                  backupEntries[i].OriginalPath,
			Backup:                backupEntries[i].BackupPath,
			OriginalExisted:       backupEntries[i].OriginalExisted,
			OriginalSHA256:        backupEntries[i].OriginalHash,
			BackupSHA256:          backupEntries[i].Checksum,
			OriginalMode:          backupEntries[i].OriginalMode,
			ExpectedPublishedHash: expectedHash,
			ExpectedPublishedMode: &publishedMode,
			RecoveryState:         domain.RecoveryPending,
		}
	}

	// Assemble the target/backup summary used for both the journal and the
	// publication record.
	pubID := domain.PublicationID(uuid.Must(uuid.NewV7()).String())
	targetEntries := make([]domain.TargetEntry, 0, len(targets))
	backupPaths := make([]string, 0, len(targets))
	for i, target := range targets {
		targetEntries = append(targetEntries, domain.TargetEntry{
			Root: target.Root, Path: target.Path, Operation: target.Operation,
		})
		backupPaths = append(backupPaths, backupEntries[i].BackupPath)
	}
	startedAt := utcNowPublish()
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
		Rollback:     rollbackEntries,
		Verification: []domain.ValidationResult{filesystemDurabilityVerification()},
		Status:       domain.PubStatusInProgress,
		StartedAt:    startedAt,
	}
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		return storage.SavePublication(ctx, tx, pub)
	}); err != nil {
		return nil, fmt.Errorf("Publish: persist recoverable attempt: %w", err)
	}

	// 9. Mirror the recoverable SQLite attempt in the append-only journal before
	// writing. The public recovery ID is the publication ID in both stores.
	journal, jErr := NewJournal(s.projectRoot, s.journalDir)
	if jErr != nil {
		return nil, recoveryInterruptionError(pubID, "create attempting journal", jErr)
	}
	if s.faults != nil && s.faults.BeforeJournalAttempt != nil {
		if fErr := s.faults.BeforeJournalAttempt(); fErr != nil {
			return nil, recoveryInterruptionError(pubID, "write attempting journal", fErr)
		}
	}
	if aErr := journal.Append(JournalEntry{
		PublicationID:  string(pubID),
		LearningID:     string(input.LearningID),
		Targets:        targetEntries,
		BackupPaths:    backupPaths,
		Recovery:       rollbackEntries,
		Verification:   pub.Verification,
		RollbackStatus: "attempting",
	}); aErr != nil {
		return nil, recoveryInterruptionError(pubID, "write attempting journal", aErr)
	}
	if s.faults != nil && s.faults.AfterAttemptPersisted != nil {
		if fErr := s.faults.AfterAttemptPersisted(pubID); fErr != nil {
			return nil, recoveryInterruptionError(pubID, "interrupted before first target write", fErr)
		}
	}

	// 10. Write files with compare-and-swap against the captured snapshots. Any
	// final-boundary replacement is preserved and reported as a conflict.
	// compensates (rolls back) and returns a structured error; it never marks
	// the learning published.
	writer := s.fileWriter()
	writtenContents := make(map[string]string)

	for i, target := range targets {
		relPath := filepath.Join(target.Root, target.Path)
		contentToWrite := composedContents[i]
		expected := TargetIdentity{Exists: snapshots[i].Exists, Hash: snapshots[i].Hash}
		perm := os.FileMode(0o644)
		if snapshots[i].Exists {
			mode := fileModeIdentity(snapshots[i].Mode)
			expected.Mode = &mode
			perm = snapshots[i].Mode
		}
		if wErr := writer.WriteFileCAS(relPath, []byte(contentToWrite), perm, expected); wErr != nil {
			return nil, s.compensate(journal, backupMgr, backupEntries, pubID, input.LearningID, targetEntries, backupPaths,
				fmt.Sprintf("write %s", target.Path), wErr)
		}
		if s.faults != nil && s.faults.AfterTargetWrite != nil {
			if fErr := s.faults.AfterTargetWrite(i, relPath); fErr != nil {
				return nil, recoveryInterruptionError(pubID, "interrupted after target write", fErr)
			}
		}

		rollbackEntries[i].RecoveryState = domain.RecoveryPublished
		pub.Rollback = rollbackEntries
		if progressErr := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			return storage.UpdatePublication(ctx, tx, pub)
		}); progressErr != nil {
			return nil, s.compensate(journal, backupMgr, backupEntries, pubID, input.LearningID, targetEntries, backupPaths,
				fmt.Sprintf("persist recovery progress for %s", target.Path), progressErr)
		}
		writtenContents[relPath] = contentToWrite
	}

	// 11. Verify: every file must exist and match its expected content hash. A
	// verification failure is a late failure and must compensate.
	verification := verifyTargets(s.projectRoot, targetEntries, writtenContents)
	for _, v := range verification {
		if !v.Pass {
			return nil, s.compensate(journal, backupMgr, backupEntries, pubID, input.LearningID, targetEntries, backupPaths,
				"verification failed",
				domain.NewValidationError(domain.ErrVerificationFailed, "verification failed: "+v.Check+" ("+v.Note+")"))
		}
	}

	// 12. Persist result and mark published atomically. The publication record
	// and the published status commit in ONE transaction: either both land or
	// neither does, so a commit failure never leaves a false `published`.
	now := utcNowPublish()
	learning.Status = domain.StatusPublished
	pub.Verification = append(pub.Verification, verification...)
	pub.Rollback = rollbackEntries
	pub.Status = domain.PubStatusCompleted
	pub.CompletedAt = &now

	if s.faults != nil && s.faults.BeforeDBCommit != nil {
		if fErr := s.faults.BeforeDBCommit(); fErr != nil {
			return nil, s.compensate(journal, backupMgr, backupEntries, pubID, input.LearningID, targetEntries, backupPaths,
				"persist publication", fErr)
		}
	}
	if pErr := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if updateErr := storage.UpdatePublication(ctx, tx, pub); updateErr != nil {
			return fmt.Errorf("update publication: %w", updateErr)
		}
		if updateErr := storage.UpdateLearning(ctx, tx, learning); updateErr != nil {
			return fmt.Errorf("update learning: %w", updateErr)
		}
		return nil
	}); pErr != nil {
		return nil, s.compensate(journal, backupMgr, backupEntries, pubID, input.LearningID, targetEntries, backupPaths,
			"persist publication", pErr)
	}

	// 13. SQLite is committed. Materialize its truth, then close the journal. A
	// failure here must report committed state and forbid blind publication retry.
	materializeErr := s.materialize(learning)
	terminalStatus := "completed"
	if materializeErr != nil {
		terminalStatus = "completed_record_failed"
	}
	var terminalJournalErr error
	if s.faults != nil && s.faults.BeforeTerminalJournal != nil {
		terminalJournalErr = s.faults.BeforeTerminalJournal()
	} else {
		terminalJournalErr = journal.Append(JournalEntry{
			PublicationID:  string(pubID),
			LearningID:     string(input.LearningID),
			Targets:        targetEntries,
			Recovery:       rollbackEntries,
			Verification:   pub.Verification,
			RollbackStatus: terminalStatus,
		})
	}
	if materializeErr != nil || terminalJournalErr != nil {
		return nil, committedStateError(pubID, domain.PubStatusCompleted, materializeErr, terminalJournalErr)
	}

	return &PublishResult{
		Publication: pub,
		JournalID:   string(pubID),
		Targets:     targetEntries,
	}, nil
}

func filesystemDurabilityVerification() domain.ValidationResult {
	if directorySyncAvailable() {
		return domain.ValidationResult{
			Check: "filesystem-durability", Pass: true,
			Note: "file sync, atomic placement, and parent-directory sync are enabled",
		}
	}
	return domain.ValidationResult{
		Check: "filesystem-durability", Pass: true,
		Note: "parent-directory sync is unsupported on this platform; file sync and atomic placement remain enabled",
	}
}

func recoveryInterruptionError(publicationID domain.PublicationID, stage string, cause error) error {
	return &domain.DomainError{
		Code:        domain.ErrPublicationFailed,
		Message:     stage + "; a recoverable in-progress publication was persisted before the target mutation",
		Recoverable: true,
		Details: map[string]any{
			"recovery_id":    string(publicationID),
			"publication_id": string(publicationID),
		},
		NextAction: fmt.Sprintf("run 'royo-learn rollback --journal-id %s' before retrying publication", publicationID),
		Cause:      cause,
	}
}

// composeTargetContent produces the exact bytes to write for a target, applying
// managed-block/AGENTS.md merge rules for managed destinations.
func (s *Service) composeTargetContent(target TargetResolution, proposed string, learningID domain.LearningID, targetCtx *TargetContext) (string, error) {
	if !target.IsManaged {
		return proposed, nil
	}

	targetFullPath := filepath.Join(s.projectRoot, target.Root, target.Path)

	if target.Exists {
		existingContent, readErr := os.ReadFile(targetFullPath)
		if readErr != nil {
			return "", readErr
		}
		if target.Path == "AGENTS.md" && targetCtx != nil {
			newContent, _ := InsertAgentsRef(string(existingContent), targetCtx.ProjectKey)
			return newContent, nil
		}
		managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s", learningID, proposed)
		return InsertManagedBlock(string(existingContent), managedContent), nil
	}

	// Create a new managed file.
	if target.Path == "AGENTS.md" && targetCtx != nil {
		content := BuildAgentsRefManagedBlock(targetCtx.ProjectKey)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content, nil
	}
	managedContent := fmt.Sprintf("<!-- royo-learn:managed id:%s -->\n%s", learningID, proposed)
	return InsertManagedBlock("", managedContent), nil
}

// validateCurrentHashes refuses the publication if any destination changed on
// disk after the preview was taken. It compares each destination's current
// content hash against the prior hash the preview recorded. A bare preview with
// no recorded targets skips the check (backward compatibility).
func (s *Service) validateCurrentHashes(preview *domain.PublicationPreview, targets []TargetResolution) error {
	priorByKey := make(map[string]string, len(preview.Plan.Targets))
	for _, pt := range preview.Plan.Targets {
		priorByKey[filepath.Join(pt.Root, pt.Path)] = pt.PriorHash
	}
	for _, target := range targets {
		key := filepath.Join(target.Root, target.Path)
		recorded, ok := priorByKey[key]
		if !ok {
			continue
		}
		snapshot, err := NewBackupManager(s.projectRoot, s.backupDir).SnapshotFile(key)
		if err != nil {
			return fmt.Errorf("inspect destination %s: %w", target.Path, err)
		}
		current := snapshot.Hash
		if current != recorded {
			return domain.NewConflictError(domain.ErrTargetChanged,
				fmt.Sprintf("destination %s changed on disk after the preview was taken — regenerate the preview and re-approve", target.Path))
		}
	}
	return nil
}

func validatePreviewIntegrity(preview *domain.PublicationPreview, learningID domain.LearningID) error {
	if preview == nil || preview.LearningID != learningID || preview.Plan.LearningID != learningID {
		return previewMismatch("preview does not belong to the requested learning")
	}
	if len(preview.Plan.Targets) == 0 || preview.Plan.PolicySignature == "" {
		return previewMismatch("persisted preview plan is incomplete")
	}
	seen := make(map[string]struct{}, len(preview.Plan.Targets))
	for _, target := range preview.Plan.Targets {
		key := filepath.Join(target.Root, target.Path)
		if target.Root == "" || target.Path == "" || target.Operation == "" || target.PosteriorHash == "" {
			return previewMismatch("persisted preview target is incomplete")
		}
		if _, duplicate := seen[key]; duplicate {
			return previewMismatch("persisted preview contains a duplicate target")
		}
		seen[key] = struct{}{}
		if HashContent([]byte(target.Content)) != target.PosteriorHash {
			return previewMismatch("persisted preview posterior bytes do not match their hash")
		}
	}
	if previewHash(learningID, preview.Plan.Targets, preview.Plan.PolicySignature) != preview.PreviewHash {
		return previewMismatch("persisted preview plan does not match its hash")
	}
	return nil
}

func validatePlannedTargets(preview *domain.PublicationPreview, targets []TargetResolution) ([]string, error) {
	if len(preview.Plan.Targets) != len(targets) {
		return nil, previewMismatch("resolved target set differs from the persisted preview plan")
	}
	planned := make(map[string]domain.PublicationPlanTarget, len(preview.Plan.Targets))
	for _, target := range preview.Plan.Targets {
		planned[filepath.Join(target.Root, target.Path)] = target
	}
	contents := make([]string, len(targets))
	for i, target := range targets {
		entry, ok := planned[filepath.Join(target.Root, target.Path)]
		if !ok || entry.Operation != target.Operation {
			return nil, previewMismatch("resolved target or operation differs from the persisted preview plan")
		}
		contents[i] = entry.Content
	}
	return contents, nil
}

func previewMismatch(message string) error {
	return domain.NewConflictError(domain.ErrPreviewHashMismatch, message)
}

// compensate rolls back every backed-up file after a post-write failure,
// records the outcome in the journal, and returns a structured error. It NEVER
// marks the learning published. When the rollback itself fails, the returned
// error carries a recovery instruction (the backup paths and the journal ID) so
// a human or `doctor` can restore the tree manually (H2).
func (s *Service) compensate(journal *Journal, backupMgr *BackupManager, backups []BackupEntry,
	pubID domain.PublicationID, learningID domain.LearningID, targets []domain.TargetEntry, backupPaths []string,
	stage string, cause error) error {
	ctx := context.Background()
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return rollbackDatabasePendingError(pubID, "", errors.Join(cause, err))
	}
	pub, err := storage.GetPublication(ctx, readTx, pubID)
	readTx.Rollback()
	if err != nil {
		return rollbackDatabasePendingError(pubID, "", errors.Join(cause, err))
	}

	results := make([]RestoreResult, len(backups))
	if s.faults != nil && s.faults.FailRollback != nil {
		forcedErr := s.faults.FailRollback()
		for i, entry := range backups {
			results[i] = RestoreResult{Path: entry.OriginalPath, Backup: entry.BackupPath, Error: forcedErr}
		}
	} else {
		results = backupMgr.RestoreAll(backups)
	}

	failures := make([]recoveryFailure, 0)
	restored := 0
	for i, result := range results {
		if result.Success {
			pub.Rollback[i].Success = true
			pub.Rollback[i].RecoveryState = domain.RecoveryRestored
			pub.Rollback[i].FailureReason = ""
			restored++
			continue
		}
		pub.Rollback[i].Success = false
		pub.Rollback[i].RecoveryState = domain.RecoveryConflict
		artifact, artifactErr := s.writeConflictArtifact(pubID, i, pub.Rollback[i], result.Error)
		failureErr := result.Error
		if artifactErr != nil {
			failureErr = errors.Join(result.Error, fmt.Errorf("recovery artifact: %w", artifactErr))
		} else {
			pub.Rollback[i].RecoveryArtifact = artifact
		}
		pub.Rollback[i].FailureReason = failureErr.Error()
		failures = append(failures, recoveryFailure{Path: result.Path, Reason: failureErr.Error(), Artifact: pub.Rollback[i].RecoveryArtifact})
	}
	code := string(domain.ErrPublicationFailed)
	if len(failures) > 0 {
		code = string(domain.ErrRollbackFailed)
	}
	message := fmt.Sprintf("%s; compensating rollback restored %d of %d targets", stage, restored, len(results))
	pub.Status = domain.PubStatusFailed
	pub.ErrorCode = &code
	pub.ErrorMessage = &message
	pub.Verification = append(pub.Verification, domain.ValidationResult{
		Check: "compensating-rollback", Pass: len(failures) == 0,
		Note: fmt.Sprintf("restored %d of %d files", restored, len(results)),
	})
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		return storage.UpdatePublication(ctx, tx, pub)
	}); err != nil {
		return rollbackDatabasePendingError(pubID, "", errors.Join(cause, err))
	}

	status := "rolled_back"
	if len(failures) > 0 {
		status = "rollback_failed"
	}
	var journalErr error
	if journal != nil {
		journalErr = journal.Append(JournalEntry{
			PublicationID: string(pubID), LearningID: string(learningID), Targets: targets,
			BackupPaths: backupPaths, Recovery: pub.Rollback, Verification: pub.Verification,
			RollbackStatus: status,
		})
	}
	if len(failures) > 0 {
		first := failures[0]
		details := map[string]any{
			"publication_id": string(pubID), "recovery_id": string(pubID), "journal_id": string(pubID),
			"path": first.Path, "reason": first.Reason, "recovery_artifact": first.Artifact,
			"conflicts": failures,
		}
		if journalErr != nil {
			details["audit_reason"] = journalErr.Error()
		}
		return &domain.DomainError{
			Code:        domain.ErrRollbackFailed,
			Message:     fmt.Sprintf("%s failed and compensating rollback preserved %d conflicting target(s)", stage, len(failures)),
			Recoverable: true,
			Details:     details,
			NextAction:  "review the reversal artifact to restore each target manually, resolve every conflict, then retry rollback",
			Cause:       errors.Join(cause, journalErr),
		}
	}
	return &domain.DomainError{
		Code:        domain.ErrPublicationFailed,
		Message:     fmt.Sprintf("%s; all target mutations were restored and the failed attempt was recorded", stage),
		Recoverable: true,
		Details:     map[string]any{"publication_id": string(pubID), "recovery_id": string(pubID)},
		NextAction:  "inspect the original cause, then create a fresh preview before retrying publication",
		Cause:       errors.Join(cause, journalErr),
	}
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

		area := ctx.Area
		if area == "" {
			area = SkillArea(learning)
		}
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
	lock, err := acquirePublicationLock(s.projectRoot, "rollback", input.Actor)
	if err != nil {
		return err
	}
	defer lock.Release()

	// Load publication.
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("Rollback: begin tx: %w", err)
	}
	pub, err := storage.GetPublication(ctx, readTx, input.PublicationID)
	readTx.Rollback()
	if err != nil {
		var metadataErr *storage.PublicationMetadataError
		if errors.As(err, &metadataErr) {
			return s.legacyRecoveryError(input.PublicationID, metadataErr.Raw, metadataErr.Error())
		}
		return fmt.Errorf("Rollback: get publication: %w", err)
	}

	// Create backup manager and perform rollback.
	backupMgr := NewBackupManager(s.projectRoot, s.backupDir)
	journal, err := NewJournal(s.projectRoot, s.journalDir)
	if err != nil {
		return fmt.Errorf("Rollback: create journal: %w", err)
	}
	if pub.Status == domain.PubStatusRolledback {
		reconciled, reconcileErr := s.reconcileRolledBack(ctx, journal, pub)
		if reconcileErr != nil {
			return reconcileErr
		}
		if reconciled {
			return nil
		}
		return domain.NewConflictError(domain.ErrPublicationConflict, "publication was already rolled back")
	}
	ownershipTx, txErr := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if txErr != nil {
		return fmt.Errorf("Rollback: ownership tx: %w", txErr)
	}
	overlaps, newerID, ownershipErr := storage.HasNewerActivePublicationOverlap(ctx, ownershipTx, pub)
	ownershipTx.Rollback()
	if ownershipErr != nil {
		return fmt.Errorf("Rollback: publication ownership: %w", ownershipErr)
	}
	if overlaps {
		return domain.NewConflictError(domain.ErrPublicationConflict,
			fmt.Sprintf("newer publication %s owns an overlapping target", newerID))
	}

	return s.rollbackPublication(ctx, backupMgr, journal, pub, input.Actor)
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
