package publish

import (
	"context"
	"database/sql"
	"fmt"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// RecoveryCandidate is a product-visible interrupted publication.
type RecoveryCandidate struct {
	PublicationID domain.PublicationID     `json:"publication_id"`
	LearningID    domain.LearningID        `json:"learning_id"`
	Status        domain.PublicationStatus `json:"status"`
	JournalStatus string                   `json:"journal_status"`
	Targets       []domain.TargetEntry     `json:"targets"`
}

func (s *Service) RecoverablePublications(ctx context.Context) ([]RecoveryCandidate, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("recoverable publications: begin: %w", err)
	}
	publications, err := storage.ListRecoverablePublications(ctx, tx)
	tx.Rollback()
	if err != nil {
		return nil, err
	}
	journal, err := NewJournal(s.projectRoot, s.journalDir)
	if err != nil {
		return nil, err
	}
	candidates := make([]RecoveryCandidate, 0, len(publications))
	for _, publication := range publications {
		journalStatus, journalErr := journal.LatestStatus(publication.ID)
		if journalErr != nil {
			return nil, journalErr
		}
		candidates = append(candidates, RecoveryCandidate{
			PublicationID: publication.ID, LearningID: publication.LearningID,
			Status: publication.Status, JournalStatus: journalStatus, Targets: publication.Targets,
		})
	}
	return candidates, nil
}

func (s *Service) reconcilePublished(ctx context.Context, input *PublishInput, learning *domain.Learning) (*PublishResult, error) {
	lock, err := acquirePublicationLock(s.projectRoot, "reconcile-publish", input.Actor)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	publications, err := storage.ListPublicationsByLearning(ctx, tx, learning.ID)
	tx.Rollback()
	if err != nil {
		return nil, err
	}
	for _, publication := range publications {
		if publication.PreviewHash != input.PreviewHash || publication.Status != domain.PubStatusCompleted {
			continue
		}
		journal, journalErr := NewJournal(s.projectRoot, s.journalDir)
		if journalErr != nil {
			return nil, committedStateError(publication.ID, publication.Status, nil, journalErr)
		}
		materializeErr := s.materialize(learning)
		journalErr = journal.Append(JournalEntry{
			PublicationID: string(publication.ID), LearningID: string(publication.LearningID),
			Targets: publication.Targets, Recovery: publication.Rollback,
			Verification: publication.Verification, RollbackStatus: "completed_reconciled",
		})
		if materializeErr != nil || journalErr != nil {
			return nil, committedStateError(publication.ID, publication.Status, materializeErr, journalErr)
		}
		return &PublishResult{Publication: publication, JournalID: string(publication.ID), Targets: publication.Targets}, nil
	}
	return nil, domain.NewValidationError(domain.ErrInvalidTransition, "learning is published but no matching completed publication can be reconciled")
}

func (s *Service) reconcileRolledBack(ctx context.Context, journal *Journal, publication *domain.Publication) (bool, error) {
	status, err := journal.LatestStatus(publication.ID)
	if err != nil {
		return false, err
	}
	if status == "rolled_back" || status == "rolled_back_reconciled" {
		return false, nil
	}
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	learning, err := storage.GetLearning(ctx, tx, publication.LearningID)
	tx.Rollback()
	if err != nil {
		return false, err
	}
	materializeErr := s.materialize(learning)
	journalErr := journal.Append(JournalEntry{
		PublicationID: string(publication.ID), LearningID: string(publication.LearningID),
		Targets: publication.Targets, Recovery: publication.Rollback,
		Verification: publication.Verification, RollbackStatus: "rolled_back_reconciled",
	})
	if materializeErr != nil || journalErr != nil {
		return false, committedStateError(publication.ID, publication.Status, materializeErr, journalErr)
	}
	return true, nil
}
