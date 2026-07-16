package publish

import (
	"fmt"
	"os"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"
)

// FileWriter is the seam for atomic file writes. Production uses *Writer;
// fault-injection tests substitute a writer that fails or corrupts a specific
// write to prove the compensation path runs through the real service code
// (not by mutating storage behind the service's back).
type FileWriter interface {
	WriteFile(targetPath string, content []byte, perm os.FileMode) error
}

// FaultHooks injects deterministic failures at named points of the publish
// write flow. Every field is nil in production. Each hook, when set and
// returning a non-nil error, forces a failure at that exact point so a
// fault-injection test can prove the system never leaves a false `published`
// nor a partially modified tree.
type FaultHooks struct {
	// BeforeJournalAttempt fires just before the pre-write "attempt" journal
	// record, i.e. before any file is written.
	BeforeJournalAttempt func() error
	// BeforeDBCommit fires just before the final SQLite update that marks the
	// learning published, after files were written and verified.
	BeforeDBCommit func() error
	// FailRollback, when set, forces the compensating rollback to fail so a test
	// can prove a recovery instruction is emitted.
	FailRollback func() error
}

// Service provides publish, preview, approval, and rollback operations.
type Service struct {
	db          *storage.DB
	projectRoot string
	backupDir   string
	journalDir  string
	recordsDir  string

	// writer is the injectable file-write seam; nil means the real atomic writer.
	writer FileWriter
	// faults is nil in production; set only by fault-injection tests.
	faults *FaultHooks
}

// NewService creates a new publish Service. recordsDir is required, not
// optional: publish and rollback both change the status of a learning, and
// every operation that mutates the truth must re-materialize the derived
// Markdown record (D18 rules 5 and 6). An optional recordsDir would allow a
// Service that publishes without materializing — a divergence by construction.
func NewService(db *storage.DB, projectRoot, backupDir, journalDir, recordsDir string) *Service {
	return &Service{
		db:          db,
		projectRoot: projectRoot,
		backupDir:   backupDir,
		journalDir:  journalDir,
		recordsDir:  recordsDir,
	}
}

// materializeRecord rewrites the derived Markdown record of a learning after
// its truth changed in SQLite (D6, D18 rule 5).
//
// It runs AFTER the owning transaction commits, so a failure here never undoes
// a publication or a rollback that already succeeded — it reports a store that
// could not write, over a truth that is already correct. The window is
// recoverable and does not need a queue: `doctor` detects the divergence and
// `rebuild-index` rewrites the record from the same source of truth.
func (s *Service) materializeRecord(learning *domain.Learning) error {
	if err := record.WriteRecord(s.recordsDir, learning); err != nil {
		return fmt.Errorf("the operation completed and SQLite is correct, but the "+
			"Markdown record could not be written — run `royo-learn rebuild-index` "+
			"to repair it: %w", err)
	}
	return nil
}

// fileWriter returns the injected writer or the default atomic writer.
func (s *Service) fileWriter() FileWriter {
	if s.writer != nil {
		return s.writer
	}
	return NewWriter(s.projectRoot)
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
	// ExpiresAt is an absolute expiry instant. When set it takes precedence over
	// ExpiresIn. A value already in the past yields an immediately-expired
	// approval, which CheckApproval rejects.
	ExpiresAt *time.Time
	ExpiresIn int // seconds; 0 = no expiry
	Actor     domain.Actor
}

// PublishInput carries the input for publishing.
type PublishInput struct {
	LearningID  domain.LearningID
	PreviewHash string
	ApprovalID  *domain.ApprovalID
	// Apply gates the actual write. When false (the default) Publish validates
	// the plan and returns a dry-run result WITHOUT touching any file (D7): the
	// write path is the second, independent line of defence after approval.
	Apply bool
	Force bool
	Actor domain.Actor
}

// PublishResult is the output of a publish operation.
type PublishResult struct {
	// Publication is nil for a dry run.
	Publication *domain.Publication
	JournalID   string
	// DryRun is true when the request validated the plan but wrote nothing.
	DryRun bool
	// Targets lists the destinations the plan would (or did) write.
	Targets []domain.TargetEntry
}

// PolicyEvaluation records the result of a policy check.
type PolicyEvaluation struct {
	PolicyName string
	Passed     bool
	Reason     string
}
