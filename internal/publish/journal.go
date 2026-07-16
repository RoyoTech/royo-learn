package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// Journal is an append-only publish journal confined to the project store.
type Journal struct {
	projectRoot string
	journalPath string
}

// JournalEntry represents a single durable journal line.
type JournalEntry struct {
	Timestamp      string                    `json:"timestamp"`
	PublicationID  string                    `json:"publication_id"`
	LearningID     string                    `json:"learning_id"`
	Targets        []domain.TargetEntry      `json:"targets"`
	BackupPaths    []string                  `json:"backup_paths"`
	Recovery       []domain.RollbackEntry    `json:"recovery,omitempty"`
	Diff           string                    `json:"diff"`
	Verification   []domain.ValidationResult `json:"verification"`
	RollbackStatus string                    `json:"rollback_status,omitempty"`
}

// NewJournal creates a journal under a non-symlink directory inside projectRoot.
func NewJournal(projectRoot, journalDir string) (*Journal, error) {
	absProject, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("NewJournal: project root: %w", err)
	}
	absJournal, err := filepath.Abs(journalDir)
	if err != nil {
		return nil, fmt.Errorf("NewJournal: journal root: %w", err)
	}
	rel, err := filepath.Rel(absProject, absJournal)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("NewJournal: journal root %q must be inside project root %q", journalDir, projectRoot)
	}
	if unsafeRootForm(absJournal) {
		return nil, fmt.Errorf("NewJournal: journal root uses a forbidden path form")
	}
	if info, statErr := os.Lstat(absJournal); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("NewJournal: journal root must not be a symlink: %s", journalDir)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("NewJournal: journal root: %w", statErr)
	}
	journalPath := filepath.Join(rel, "publish-journal.jsonl")
	if _, err := secureRelativePath(absProject, journalPath, "journal", true); err != nil {
		return nil, fmt.Errorf("NewJournal: journal path: %w", err)
	}
	return &Journal{projectRoot: absProject, journalPath: journalPath}, nil
}

// Append writes and syncs one journal entry. The opened inode is compared with
// the path before any bytes are written so a symlink swap cannot redirect it.
func (j *Journal) Append(entry JournalEntry) error {
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("Journal.Append: marshal: %w", err)
	}
	data = append(data, '\n')

	root, err := openRootNoFollow(j.projectRoot)
	if err != nil {
		return fmt.Errorf("Journal.Append: open root: %w", err)
	}
	defer root.Close()
	before, statErr := root.Lstat(j.journalPath)
	created := os.IsNotExist(statErr)
	if statErr == nil && before.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Journal.Append: journal path is a symlink")
	}
	if statErr != nil && !created {
		return fmt.Errorf("Journal.Append: lstat: %w", statErr)
	}
	f, err := root.OpenFile(j.journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("Journal.Append: open: %w", err)
	}
	opened, openedErr := f.Stat()
	after, afterErr := root.Lstat(j.journalPath)
	if openedErr != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, after) {
		_ = f.Close()
		return fmt.Errorf("Journal.Append: journal path changed during open")
	}
	if !created && !os.SameFile(before, opened) {
		_ = f.Close()
		return fmt.Errorf("Journal.Append: journal file was replaced during open")
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("Journal.Append: chmod: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("Journal.Append: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("Journal.Append: sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("Journal.Append: close: %w", err)
	}
	if created {
		if _, err := syncParentDirectoryRequired(filepath.Join(j.projectRoot, j.journalPath)); err != nil {
			return fmt.Errorf("Journal.Append: %w", err)
		}
	}
	return nil
}

// ReadAll validates and returns every durable journal entry.
func (j *Journal) ReadAll() ([]JournalEntry, error) {
	root, err := openRootNoFollow(j.projectRoot)
	if err != nil {
		return nil, fmt.Errorf("Journal.ReadAll: open root: %w", err)
	}
	defer root.Close()
	info, err := root.Lstat(j.journalPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Journal.ReadAll: lstat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Journal.ReadAll: journal is not a regular non-symlink file")
	}
	data, err := root.ReadFile(j.journalPath)
	if err != nil {
		return nil, fmt.Errorf("Journal.ReadAll: read: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	entries := make([]JournalEntry, 0, len(lines))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("Journal.ReadAll: line %d: %w", index+1, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (j *Journal) LatestStatus(publicationID domain.PublicationID) (string, error) {
	entries, err := j.ReadAll()
	if err != nil {
		return "", err
	}
	status := ""
	for _, entry := range entries {
		if entry.PublicationID == string(publicationID) {
			status = entry.RollbackStatus
		}
	}
	return status, nil
}
