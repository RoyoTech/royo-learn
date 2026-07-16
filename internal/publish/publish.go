package publish

import (
	"os"
	"path/filepath"
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
	WriteFileCAS(targetPath string, content []byte, perm os.FileMode, expected TargetIdentity) error
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
	// AfterAttemptPersisted simulates an interruption after both SQLite and the
	// journal contain complete recovery metadata but before the first target write.
	AfterAttemptPersisted func(domain.PublicationID) error
	// AfterTargetWrite simulates abrupt process loss after a target mutation. The
	// service deliberately does not compensate this test-only crash seam.
	AfterTargetWrite func(index int, path string) error
	// BeforeRollbackProgress fails after a target has been restored but before
	// that per-target progress is persisted.
	BeforeRollbackProgress func(index int) error
	// BeforeRollbackCommit fails the final publication-state transaction after
	// all target restoration progress has been persisted.
	BeforeRollbackCommit func() error
	// BeforeMaterialize injects a post-commit record materialization failure.
	BeforeMaterialize func() error
	// BeforeTerminalJournal injects a failure at the final audit boundary.
	BeforeTerminalJournal func() error
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

// NewService creates a new publish Service.
func NewService(db *storage.DB, projectRoot, backupDir, journalDir string, recordsDir ...string) *Service {
	records := filepath.Join(projectRoot, ".royo-learn", "records")
	if len(recordsDir) > 0 && recordsDir[0] != "" {
		records = recordsDir[0]
	}
	return &Service{
		db:          db,
		projectRoot: projectRoot,
		backupDir:   backupDir,
		journalDir:  journalDir,
		recordsDir:  records,
	}
}

func (s *Service) materialize(learning *domain.Learning) error {
	if s.faults != nil && s.faults.BeforeMaterialize != nil {
		if err := s.faults.BeforeMaterialize(); err != nil {
			return err
		}
	}
	return record.WriteRecord(s.recordsDir, learning)
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
	PolicyName string `json:"PolicyName"`
	Passed     bool   `json:"Passed"`
	Reason     string `json:"Reason"`
}
