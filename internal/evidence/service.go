package evidence

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// evidenceDir is the store-relative directory holding the content-addressed
// blobs. It mirrors config.DefaultEvidenceDir without importing config, which
// would pull a configuration dependency into the domain-facing evidence layer.
const evidenceDir = ".royo-learn/evidence"

// Item is one evidence record as supplied through a public interface (CLI or
// MCP). It is the raw, UNREDACTED input; nothing in this struct may be
// persisted or returned to a caller without passing through Prepare first.
type Item struct {
	Kind     domain.EvidenceKind
	Summary  string
	Source   string
	Content  string
	Command  []string
	ExitCode *int
}

// Service turns evidence Items into persisted evidence records.
//
// Secret redaction runs inside Prepare, BEFORE anything is written. It is a
// write condition, not an output filter: SQLite, the blob store, the Markdown
// records, the audit log and every CLI/MCP response receive content that was
// already redacted upstream of them.
type Service struct {
	blobs   *BlobStore
	allowed []string
}

// NewService opens the content-addressed blob store beneath projectRoot.
//
// allowedCommands is the allowlist for command evidence collection. A nil or
// empty list permits only git, which is the CommandRunner default.
func NewService(projectRoot string, allowedCommands []string) (*Service, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("evidence: project root is required")
	}
	blobs, err := StoreWithinRoot(
		projectRoot,
		filepath.Join(projectRoot, filepath.FromSlash(evidenceDir)),
		DefaultEvidenceBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("evidence: open blob store: %w", err)
	}
	return &Service{blobs: blobs, allowed: allowedCommands}, nil
}

// Prepare redacts every item and stores its content in the blob store, returning
// domain records ready to persist.
//
// Redaction happens here, before the blob is written and before the caller can
// hand the records to SQLite. A record returned by Prepare is safe for every
// sink.
func (s *Service) Prepare(learningID domain.LearningID, items []Item, now time.Time) ([]*domain.Evidence, error) {
	if s == nil {
		return nil, fmt.Errorf("evidence: nil service")
	}
	if len(items) == 0 {
		return nil, nil
	}
	if learningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "evidence: learning id is required")
	}

	records := make([]*domain.Evidence, 0, len(items))
	for i, item := range items {
		kind := item.Kind
		if kind == "" {
			kind = domain.KindText
		}

		// --- Redaction, before any write. ---
		summary := RedactString(item.Summary)
		source := RedactString(item.Source)
		content := Redact([]byte(item.Content), nil)
		redacted := summary != item.Summary ||
			source != item.Source ||
			string(content) != item.Content

		var command []string
		if len(item.Command) > 0 {
			command = make([]string, 0, len(item.Command))
			for _, arg := range item.Command {
				clean := RedactString(arg)
				if clean != arg {
					redacted = true
				}
				command = append(command, clean)
			}
		}

		if summary == "" {
			return nil, domain.NewValidationError(domain.ErrInvalidArgument,
				fmt.Sprintf("evidence[%d]: summary is required", i))
		}

		// The blob store only ever receives redacted bytes.
		var sha string
		var size int64
		if len(content) > 0 {
			hash, err := s.blobs.Put(content)
			if err != nil {
				return nil, fmt.Errorf("evidence[%d]: store blob: %w", i, err)
			}
			sha = hash
			size = int64(len(content))
		}

		record := &domain.Evidence{
			ID:          domain.EvidenceID(uuid.Must(uuid.NewV7()).String()),
			LearningID:  learningID,
			Kind:        kind,
			URI:         source,
			Summary:     summary,
			SHA256:      sha,
			Command:     command,
			ExitCode:    item.ExitCode,
			Redacted:    redacted,
			SizeBytes:   size,
			CollectedAt: now,
		}
		if err := domain.ValidateEvidence(record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// PersistTx writes prepared records inside an existing transaction, so that a
// learning and its evidence land in one coherent operation.
func PersistTx(ctx context.Context, tx *sql.Tx, records []*domain.Evidence) error {
	for _, record := range records {
		if err := storage.SaveEvidence(ctx, tx, record); err != nil {
			return err
		}
	}
	return nil
}

// AnyRedacted reports whether redaction modified any of the records.
func AnyRedacted(records []*domain.Evidence) bool {
	for _, record := range records {
		if record.Redacted {
			return true
		}
	}
	return false
}

// IDs returns the identifiers of the records, in order.
func IDs(records []*domain.Evidence) []domain.EvidenceID {
	out := make([]domain.EvidenceID, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	return out
}

// RedactString is the string form of Redact.
func RedactString(s string) string {
	if s == "" {
		return ""
	}
	return string(Redact([]byte(s), nil))
}

// RedactLearning redacts every free-text field of a learning in place.
//
// A secret does not become harmless because it arrived in the observation rather
// than in an evidence record: SQLite and the Markdown record are sinks either
// way. Callers MUST invoke this before computing the normalized hash, so that
// deduplication is computed over the redacted content and stays deterministic.
func RedactLearning(l *domain.Learning) {
	if l == nil {
		return
	}
	l.Title = RedactString(l.Title)
	l.Context = RedactString(l.Context)
	l.Observation = RedactString(l.Observation)
	l.ReusableLesson = RedactString(l.ReusableLesson)
	l.Limits = RedactString(l.Limits)
	for i, step := range l.RecommendedProcedure {
		l.RecommendedProcedure[i] = RedactString(step)
	}
	for i, term := range l.RetrievalTerms {
		l.RetrievalTerms[i] = RedactString(term)
	}
}
