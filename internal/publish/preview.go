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

// Preview generates a publication preview for an approved learning.
// It resolves targets, generates a diff, evaluates policies, and stores the preview.
func (s *Service) Preview(ctx context.Context, projectID domain.ProjectID, input *PreviewInput) (*PreviewResult, error) {
	if input == nil || input.LearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "learning_id is required for preview")
	}

	// Load the learning.
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("Preview: begin tx: %w", err)
	}
	learning, err := storage.GetLearning(ctx, readTx, input.LearningID)
	readTx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Preview: get learning: %w", err)
	}
	if learning == nil {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "learning: "+string(input.LearningID))
	}

	// Learning must be approved.
	if learning.Status != domain.StatusApproved {
		return nil, domain.NewValidationError(domain.ErrInvalidTransition,
			fmt.Sprintf("learning must be approved (current: %s)", learning.Status))
	}

	// Get the latest curation for this learning.
	readTx2, _ := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	curations, err := storage.ListCurationsByLearning(ctx, readTx2, input.LearningID)
	readTx2.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Preview: list curations: %w", err)
	}
	if len(curations) == 0 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "no curation found for learning")
	}
	curation := curations[0] // latest first

	// Resolve targets.
	targets, err := ResolveTarget(s.projectRoot, curation)
	if err != nil {
		return nil, fmt.Errorf("Preview: resolve target: %w", err)
	}

	// Build proposed content based on the learning and destination.
	proposedContent := BuildSkillContent(learning.Title, learning.Context, learning.ReusableLesson,
		strings.Join(learning.RecommendedProcedure, "\n"))

	// Generate diff for each target.
	var diffLines []string
	for _, target := range targets {
		var existingContent []byte
		targetFullPath := filepath.Join(target.Root, target.Path)
		if existing, err := os.ReadFile(targetFullPath); err == nil {
			existingContent = existing
		}

		diff := GenerateDiff(existingContent, []byte(proposedContent), target.Path, target.Exists)
		diffLines = append(diffLines, diff)
	}

	combinedDiff := strings.Join(diffLines, "\n")

	// Evaluate policies.
	policies := EvaluatePolicies(learning, curation)

	// Build preview record.
	previewID := domain.PreviewID(uuid.Must(uuid.NewV7()).String())
	previewHash := HashContent([]byte(combinedDiff))

	preview := &domain.PublicationPreview{
		ID:         previewID,
		LearningID: input.LearningID,
		Plan: domain.PublicationPlan{
			LearningID:       input.LearningID,
			TargetRoot:       s.projectRoot,
			TargetPath:       targets[0].Path,
			Operation:        targets[0].Operation,
			Content:          proposedContent,
			Patch:            combinedDiff,
			RequiresApproval: RequiresHumanApproval(policies),
			Risk:             evaluateRisk(learning, curation),
		},
		PreviewHash:      previewHash,
		Risk:             evaluateRisk(learning, curation),
		RequiresApproval: RequiresHumanApproval(policies),
		CreatedAt:        utcNowPublish(),
	}

	// Persist preview.
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		return storage.SavePreview(ctx, tx, preview)
	}); err != nil {
		return nil, fmt.Errorf("Preview: save preview: %w", err)
	}

	return &PreviewResult{
		Preview:  preview,
		Targets:  targets,
		Diff:     combinedDiff,
		Policies: policies,
	}, nil
}

// evaluateRisk determines the risk level of a publication based on
// destination type and scope.
func evaluateRisk(learning *domain.Learning, curation *domain.Curation) domain.RiskLevel {
	dest := curation.Destination
	if dest == nil {
		return domain.RiskLow
	}

	switch dest.Type {
	case domain.DestAgentsRule, domain.DestShared:
		return domain.RiskHigh
	case domain.DestSkill:
		if dest.Required {
			return domain.RiskMedium
		}
		return domain.RiskLow
	default:
		return domain.RiskLow
	}
}
