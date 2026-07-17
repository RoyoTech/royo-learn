package publish

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// RollbackFromBackup restores files from complete entries stored in a publication.
func RollbackFromBackup(backupMgr *BackupManager, rollbackEntries []domain.RollbackEntry) []RestoreResult {
	entries := make([]BackupEntry, len(rollbackEntries))
	for i, re := range rollbackEntries {
		entries[i] = backupEntryFromDomain(re)
	}
	return backupMgr.RestoreAll(entries)
}

func backupEntryFromDomain(entry domain.RollbackEntry) BackupEntry {
	return BackupEntry{
		OriginalPath:          entry.Path,
		BackupPath:            entry.Backup,
		Checksum:              entry.BackupSHA256,
		OriginalHash:          entry.OriginalSHA256,
		OriginalMode:          entry.OriginalMode,
		OriginalExisted:       entry.OriginalExisted,
		ExpectedPublishedHash: entry.ExpectedPublishedHash,
		ExpectedPublishedMode: entry.ExpectedPublishedMode,
	}
}

type recoveryFailure struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Artifact string `json:"recovery_artifact,omitempty"`
}

func (s *Service) rollbackPublication(ctx context.Context, backupMgr *BackupManager, journal *Journal, pub *domain.Publication, actor domain.Actor) error {
	if len(pub.Rollback) == 0 {
		return s.legacyRecoveryError(pub.ID, "[]", "rollback metadata is empty")
	}
	for i, entry := range pub.Rollback {
		if err := validatePersistedRecoveryEntry(entry); err != nil {
			return s.legacyRecoveryError(pub.ID, "", fmt.Sprintf("rollback metadata entry %d is incomplete: %v", i, err))
		}
	}

	restored := 0
	failures := make([]recoveryFailure, 0)
	for i := range pub.Rollback {
		entry := &pub.Rollback[i]
		err := backupMgr.RestoreFile(backupEntryFromDomain(*entry))
		if err != nil {
			entry.Success = false
			entry.RecoveryState = domain.RecoveryConflict
			artifact, artifactErr := s.writeConflictArtifact(pub.ID, i, *entry, err)
			failureErr := err
			if artifactErr != nil {
				failureErr = errors.Join(err, fmt.Errorf("recovery artifact: %w", artifactErr))
			} else {
				entry.RecoveryArtifact = artifact
			}
			entry.FailureReason = failureErr.Error()
			failures = append(failures, recoveryFailure{Path: entry.Path, Reason: failureErr.Error(), Artifact: entry.RecoveryArtifact})
		} else {
			entry.Success = true
			entry.RecoveryState = domain.RecoveryRestored
			entry.FailureReason = ""
			restored++
		}

		if s.faults != nil && s.faults.BeforeRollbackProgress != nil {
			if hookErr := s.faults.BeforeRollbackProgress(i); hookErr != nil {
				return rollbackDatabasePendingError(pub.ID, entry.Path, hookErr)
			}
		}
		if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			return storage.UpdatePublication(ctx, tx, pub)
		}); err != nil {
			return rollbackDatabasePendingError(pub.ID, entry.Path, err)
		}
	}

	verification := domain.ValidationResult{
		Check: "rollback",
		Pass:  len(failures) == 0,
		Note:  fmt.Sprintf("restored %d of %d files", restored, len(pub.Rollback)),
	}
	pub.Verification = append(pub.Verification, verification)
	now := utcNowPublish()
	pub.CompletedAt = &now

	if len(failures) > 0 {
		code := string(domain.ErrRollbackFailed)
		message := fmt.Sprintf("rollback preserved %d conflicting target(s)", len(failures))
		pub.Status = domain.PubStatusFailed
		pub.ErrorCode = &code
		pub.ErrorMessage = &message
		if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			return storage.UpdatePublication(ctx, tx, pub)
		}); err != nil {
			return rollbackDatabasePendingError(pub.ID, failures[0].Path, errors.Join(errors.New(message), err))
		}
		journalErr := journal.Append(JournalEntry{
			PublicationID: string(pub.ID), LearningID: string(pub.LearningID), Targets: pub.Targets,
			Recovery: pub.Rollback, Verification: []domain.ValidationResult{verification}, RollbackStatus: "rollback_failed",
		})
		return rollbackFailureError(pub.ID, failures, journalErr)
	}

	if s.faults != nil && s.faults.BeforeRollbackCommit != nil {
		if hookErr := s.faults.BeforeRollbackCommit(); hookErr != nil {
			return rollbackDatabasePendingError(pub.ID, "", hookErr)
		}
	}
	pub.Status = domain.PubStatusRolledback
	pub.ErrorCode = nil
	pub.ErrorMessage = nil
	var learning *domain.Learning
	previousState := `{"status":"completed"}`
	newState := `{"status":"rolled_back"}`
	auditEvent := &domain.AuditEvent{
		ID: domain.AuditEventID(uuid.Must(uuid.NewV7()).String()), OccurredAt: now,
		Actor: actor, Operation: "publication.rollback", EntityType: "publication", EntityID: string(pub.ID),
		PreviousState: &previousState, NewState: &newState, PayloadSHA256: HashContent([]byte(pub.ID)), Result: "success",
	}
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		learning, err = storage.GetLearning(ctx, tx, pub.LearningID)
		if err != nil {
			return err
		}
		if learning == nil {
			return domain.NewNotFoundError(domain.ErrLearningNotFound, "learning: "+string(pub.LearningID))
		}
		if learning.Status == domain.StatusPublished {
			learning.Status = domain.StatusApproved
			learning.UpdatedAt = now
			if err := storage.UpdateLearning(ctx, tx, learning); err != nil {
				return err
			}
		}
		if err := storage.UpdatePublication(ctx, tx, pub); err != nil {
			return err
		}
		return storage.RecordEventTx(ctx, tx, auditEvent)
	}); err != nil {
		return rollbackDatabasePendingError(pub.ID, "", err)
	}
	materializeErr := s.materialize(learning)
	terminalStatus := "rolled_back"
	if materializeErr != nil {
		terminalStatus = "rolled_back_record_failed"
	}
	var journalErr error
	if s.faults != nil && s.faults.BeforeTerminalJournal != nil {
		journalErr = s.faults.BeforeTerminalJournal()
	} else {
		journalErr = journal.Append(JournalEntry{
			PublicationID: string(pub.ID), LearningID: string(pub.LearningID), Targets: pub.Targets,
			Recovery: pub.Rollback, Verification: []domain.ValidationResult{verification}, RollbackStatus: terminalStatus,
		})
	}
	if materializeErr != nil || journalErr != nil {
		return committedStateError(pub.ID, domain.PubStatusRolledback, materializeErr, journalErr)
	}
	return nil
}

func validatePersistedRecoveryEntry(entry domain.RollbackEntry) error {
	if entry.RecoveryState == "" {
		return fmt.Errorf("recovery state is required")
	}
	return validateRestoreEntry(backupEntryFromDomain(entry))
}

func rollbackFailureError(publicationID domain.PublicationID, failures []recoveryFailure, journalErr error) error {
	first := failures[0]
	var auditErr error
	if journalErr != nil {
		auditErr = fmt.Errorf("append rollback failure journal: %w", journalErr)
	}
	details := map[string]any{
		"publication_id":    string(publicationID),
		"recovery_id":       string(publicationID),
		"path":              first.Path,
		"reason":            first.Reason,
		"recovery_artifact": first.Artifact,
		"conflicts":         failures,
	}
	if auditErr != nil {
		details["audit_reason"] = auditErr.Error()
	}
	return &domain.DomainError{
		Code:        domain.ErrRollbackFailed,
		Message:     fmt.Sprintf("rollback could not safely restore %d target(s)", len(failures)),
		Recoverable: true,
		Details:     details,
		NextAction:  "review the reversal artifact, resolve the destination conflict, then retry rollback",
		Cause:       errors.Join(errors.New(first.Reason), auditErr),
	}
}

func rollbackDatabasePendingError(publicationID domain.PublicationID, path string, cause error) error {
	details := map[string]any{
		"publication_id": string(publicationID),
		"recovery_id":    string(publicationID),
		"database_state": "rollback_pending",
	}
	if path != "" {
		details["path"] = path
	}
	return &domain.DomainError{
		Code:        domain.ErrRollbackFailed,
		Message:     "filesystem recovery advanced but SQLite did not confirm the same state",
		Recoverable: true,
		Details:     details,
		NextAction:  fmt.Sprintf("retry 'royo-learn rollback --journal-id %s'; restored targets are detected idempotently", publicationID),
		Cause:       cause,
	}
}

func committedStateError(publicationID domain.PublicationID, status domain.PublicationStatus, materializeErr, journalErr error) error {
	message := "SQLite committed the terminal state"
	if materializeErr != nil {
		message += " but Markdown materialization failed"
	}
	if journalErr != nil {
		message += " and the append-only journal could not record the terminal result"
	}
	return &domain.DomainError{
		Code:        domain.ErrPublicationFailed,
		Message:     message,
		Recoverable: false,
		Details: map[string]any{
			"committed":              true,
			"publication_id":         string(publicationID),
			"status":                 string(status),
			"materialization_failed": materializeErr != nil,
			"audit_failed":           journalErr != nil,
		},
		NextAction: "do not retry blindly; inspect the publication by ID, repair record materialization or the journal path, and reconcile from SQLite truth",
		Cause:      errors.Join(materializeErr, journalErr),
	}
}

func (s *Service) legacyRecoveryError(publicationID domain.PublicationID, raw, reason string) error {
	body := "Automatic rollback was refused because persisted recovery metadata cannot be verified.\n" +
		"Publication: " + string(publicationID) + "\nReason: " + reason + "\n" +
		"The destination was not modified. Preserve the database row and backups for manual recovery.\n"
	if raw != "" {
		body += "Persisted rollback_json (verbatim):\n" + raw + "\n"
	}
	artifact, artifactErr := s.writeRecoveryArtifact(publicationID, "legacy-metadata", body)
	if artifactErr != nil {
		reason += "; recovery artifact creation failed: " + artifactErr.Error()
	}
	return &domain.DomainError{
		Code:        domain.ErrRollbackFailed,
		Message:     "rollback metadata is empty, malformed, or incomplete; destination preserved",
		Recoverable: false,
		Details: map[string]any{
			"publication_id":    string(publicationID),
			"recovery_id":       string(publicationID),
			"reason":            reason,
			"recovery_artifact": artifact,
		},
		NextAction: "inspect the recovery artifact and restore manually only after verifying the original and published identities",
	}
}

func (s *Service) writeConflictArtifact(publicationID domain.PublicationID, index int, entry domain.RollbackEntry, cause error) (string, error) {
	manager := NewBackupManager(s.projectRoot, s.backupDir)
	current, currentErr := manager.SnapshotFile(entry.Path)
	if currentErr != nil {
		return "", currentErr
	}
	var desired []byte
	if entry.OriginalExisted != nil && *entry.OriginalExisted {
		backupPath, err := secureAbsoluteWithin(s.backupDir, entry.Backup, "backup")
		if err != nil {
			return "", fmt.Errorf("validate original backup: %w", err)
		}
		desired, err = readVerifiedBackup(backupPath, entry.BackupSHA256)
		if err != nil {
			return "", fmt.Errorf("read original backup: %w", err)
		}
	}
	diff := GenerateDiff(current.Content, desired, entry.Path, current.Exists)
	body := fmt.Sprintf("# Automatic rollback blocked\n# Publication: %s\n# Target: %s\n# Reason: %v\n# Review and apply manually; royo-learn did not modify the destination.\n%s",
		publicationID, entry.Path, cause, diff)
	return s.writeRecoveryArtifact(publicationID, fmt.Sprintf("target-%d", index+1), body)
}

func (s *Service) writeRecoveryArtifact(publicationID domain.PublicationID, suffix, content string) (string, error) {
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, string(publicationID)+"-"+suffix) + ".patch"
	relative := filepath.Join(".royo-learn", "recovery", name)
	full := filepath.Join(s.projectRoot, relative)
	writer := NewWriter(s.projectRoot)
	if err := writer.WriteFile(relative, []byte(content), 0o600); err != nil {
		return "", err
	}
	return full, nil
}
