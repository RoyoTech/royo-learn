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

	// Resolve target context for skill destinations.
	// Only auto-derive when the destination path is generic.
	var targetCtx *TargetContext
	if curation.Destination != nil && curation.Destination.Type == domain.DestSkill {
		proj, projErr := s.loadProject(ctx, projectID)
		if projErr == nil {
			area := SkillArea(learning)
			autoName := SkillName(proj.ProjectKey, area)
			destDir := filepath.Dir(curation.Destination.Path)

			if destDir == "." || destDir == "" || destDir == autoName {
				needHook, _ := s.needAgentsHook(proj.ProjectKey)
				targetCtx = &TargetContext{
					ProjectKey:     proj.ProjectKey,
					NeedAgentsHook: needHook,
				}
				curation.Destination.Path = autoName + "/SKILL.md"
			}
		}
	}

	// Resolve targets.
	targets, err := ResolveTarget(s.projectRoot, curation, targetCtx)
	if err != nil {
		return nil, fmt.Errorf("Preview: resolve target: %w", err)
	}

	// Generate per-target content.
	var diffLines []string
	var targetContents []TargetContent

	for _, target := range targets {
		var proposedContent string
		var existingContent []byte
		targetFullPath := filepath.Join(s.projectRoot, target.Root, target.Path)

		if existing, err := os.ReadFile(targetFullPath); err == nil {
			existingContent = existing
		}

		// Build content based on target type.
		proposedContent = s.buildTargetContent(target, learning, curation, targetCtx)

		targetContents = append(targetContents, TargetContent{
			Target:  target,
			Content: proposedContent,
		})

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
			Content:          targetContents[0].Content,
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

// TargetContent holds a target and its proposed content.
type TargetContent struct {
	Target  TargetResolution
	Content string
}

// buildTargetContent builds the proposed content for a specific target.
func (s *Service) buildTargetContent(target TargetResolution, learning *domain.Learning, curation *domain.Curation, ctx *TargetContext) string {
	if ctx == nil || ctx.ProjectKey == "" {
		return BuildSkillContent(learning.Title, learning.Context,
			learning.ReusableLesson, strings.Join(learning.RecommendedProcedure, "\n"))
	}

	// Determine which kind of target this is.
	indexName := IndexSkillName(ctx.ProjectKey)
	targetName := filepath.Base(filepath.Dir(target.Path)) // skill name from path

	if targetName == indexName {
		// Index skill: regenerate catalog.
		entries, err := DiscoverChildSkills(s.projectRoot, ctx.ProjectKey)
		if err != nil {
			entries = nil
		}
		return GenerateIndexContent(ctx.ProjectKey, entries)
	}

	if target.Path == "AGENTS.md" {
		return BuildAgentsRefManagedBlock(ctx.ProjectKey)
	}

	// Child skill: merge learning into existing skill content.
	var sections []SkillSection

	targetFullPath := filepath.Join(s.projectRoot, target.Root, target.Path)
	if existing, err := os.ReadFile(targetFullPath); err == nil {
		sections = parseSkillSections(string(existing))
	}

	sections = MergeLearningIntoSections(sections, learning)

	// Collect all learning IDs.
	ids := make([]domain.LearningID, 0, len(sections))
	for _, sec := range sections {
		ids = append(ids, sec.LearningID)
	}

	area := SkillArea(learning)
	fm := SkillFrontmatter{
		Name:        SkillName(ctx.ProjectKey, area),
		Description: BuildDescription(ctx.ProjectKey, area, []*domain.Learning{learning}),
		Source:      "royo-learn",
		Project:     ctx.ProjectKey,
		LearningIDs: ids,
		UpdatedAt:   utcNowPublish().Format("2006-01-02"),
	}

	return GenerateSkillContent(fm, sections)
}

// loadProject loads the project from the database.
func (s *Service) loadProject(ctx context.Context, projectID domain.ProjectID) (*domain.Project, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("loadProject: begin tx: %w", err)
	}
	defer tx.Rollback()

	proj, err := storage.GetProject(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("loadProject: get: %w", err)
	}
	if proj == nil {
		return nil, domain.NewNotFoundError(domain.ErrProjectNotFound, "project: "+string(projectID))
	}
	return proj, nil
}

// needAgentsHook determines whether the AGENTS.md hook needs to be inserted.
func (s *Service) needAgentsHook(projectKey string) (bool, error) {
	agentsPath := filepath.Join(s.projectRoot, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No AGENTS.md exists yet — hook will be created.
			return true, nil
		}
		return false, err
	}
	return !HasAgentsRef(string(data), projectKey), nil
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
