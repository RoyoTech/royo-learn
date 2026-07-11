package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/domain"
)

// Journal is an append-only publish journal stored in the project's .royo-learn directory.
type Journal struct {
	journalPath string
}

// JournalEntry represents a single journal line entry.
type JournalEntry struct {
	Timestamp      string                    `json:"timestamp"`
	PublicationID  string                    `json:"publication_id"`
	LearningID     string                    `json:"learning_id"`
	Targets        []domain.TargetEntry      `json:"targets"`
	BackupPaths    []string                  `json:"backup_paths"`
	Diff           string                    `json:"diff"`
	Verification   []domain.ValidationResult `json:"verification"`
	RollbackStatus string                    `json:"rollback_status,omitempty"`
}

// NewJournal creates a new Journal writer.
func NewJournal(journalDir string) (*Journal, error) {
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		return nil, fmt.Errorf("NewJournal: mkdir: %w", err)
	}

	journalPath := filepath.Join(journalDir, "publish-journal.jsonl")
	return &Journal{journalPath: journalPath}, nil
}

// Append writes a journal entry to the append-only journal file.
func (j *Journal) Append(entry JournalEntry) error {
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("Journal.Append: marshal: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(j.journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("Journal.Append: open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("Journal.Append: write: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("Journal.Append: sync: %w", err)
	}

	return nil
}
