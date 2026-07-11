package publish

import (
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// Service provides publish, preview, approval, and rollback operations.
type Service struct {
	db          *storage.DB
	projectRoot string
	backupDir   string
	journalDir  string
}

// NewService creates a new publish Service.
func NewService(db *storage.DB, projectRoot, backupDir, journalDir string) *Service {
	return &Service{
		db:          db,
		projectRoot: projectRoot,
		backupDir:   backupDir,
		journalDir:  journalDir,
	}
}

// TargetResolution describes where a learning would be published.
type TargetResolution struct {
	Root      string
	Path      string
	Operation domain.PublicationOperation
	Exists    bool
	IsManaged bool
}

// PreviewInput carries the input for generating a preview.
type PreviewInput struct {
	LearningID domain.LearningID
	Actor      domain.Actor
}

// PreviewResult is the output of a preview generation.
type PreviewResult struct {
	Preview  *domain.PublicationPreview
	Targets  []TargetResolution
	Diff     string
	Policies []PolicyEvaluation
}

// ApproveInput carries the input for recording an approval.
type ApproveInput struct {
	LearningID       domain.LearningID
	PreviewHash      string
	ApprovedBy       string
	Reason           string
	ApprovalEvidence string
	ExpiresIn        int // seconds; 0 = no expiry
	Actor            domain.Actor
}

// PublishInput carries the input for publishing.
type PublishInput struct {
	LearningID  domain.LearningID
	PreviewHash string
	ApprovalID  *domain.ApprovalID
	Force       bool
	Actor       domain.Actor
}

// PublishResult is the output of a publish operation.
type PublishResult struct {
	Publication *domain.Publication
	JournalID   string
}

// PolicyEvaluation records the result of a policy check.
type PolicyEvaluation struct {
	PolicyName string
	Passed     bool
	Reason     string
}
