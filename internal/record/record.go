// Package record materializes Markdown derived from the SQLite learning truth.
// It depends only on domain so application services can use it without importing
// one another.
package record

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// WriteRecord atomically and durably writes dir/<learningID>.md.
func WriteRecord(dir string, learning *domain.Learning) error {
	if learning == nil {
		return fmt.Errorf("record: nil learning")
	}
	if dir == "" {
		return fmt.Errorf("record: directory is required")
	}
	id := string(learning.ID)
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("record: unsafe learning id %q", id)
	}
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("record: directory must not be a symlink: %s", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("record: lstat directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("record: mkdir: %w", err)
	}
	path := filepath.Join(dir, id+".md")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("record: target must not be a symlink: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("record: lstat target: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".royo-learn-record-*.tmp")
	if err != nil {
		return fmt.Errorf("record: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("record: chmod temp: %w", err)
	}
	if _, err := tmp.Write([]byte(buildRecordContent(learning))); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("record: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("record: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("record: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("record: rename: %w", err)
	}
	complete = true
	if runtime.GOOS != "windows" {
		parent, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("record: open parent for sync: %w", err)
		}
		if err := parent.Sync(); err != nil {
			_ = parent.Close()
			return fmt.Errorf("record: sync parent: %w", err)
		}
		if err := parent.Close(); err != nil {
			return fmt.Errorf("record: close parent: %w", err)
		}
	}
	return nil
}

func buildRecordContent(l *domain.Learning) string {
	now := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", l.ID))
	sb.WriteString(fmt.Sprintf("status: %s\n", l.Status))
	sb.WriteString(fmt.Sprintf("type: %s\n", l.Type))
	sb.WriteString(fmt.Sprintf("scope_guess: %s\n", l.ScopeGuess))
	sb.WriteString(fmt.Sprintf("confidence: %s\n", l.Confidence))
	sb.WriteString(fmt.Sprintf("evidence_level: %s\n", l.EvidenceLevel))
	sb.WriteString(fmt.Sprintf("normalized_hash: %s\n", l.NormalizedHash))
	sb.WriteString(fmt.Sprintf("fingerprint: %s\n", l.Fingerprint))
	sb.WriteString(fmt.Sprintf("record_hash: %s\n", RecordHash(l)))
	sb.WriteString(fmt.Sprintf("created_at: %s\n", l.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", now))
	if len(l.RetrievalTerms) > 0 {
		sb.WriteString("retrieval_terms:\n")
		for _, term := range l.RetrievalTerms {
			sb.WriteString(fmt.Sprintf("  - %s\n", term))
		}
	}
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", l.Title))
	sb.WriteString("## Context\n\n" + l.Context + "\n\n")
	sb.WriteString("## Observation\n\n" + l.Observation + "\n\n")
	sb.WriteString("## Reusable Lesson\n\n" + l.ReusableLesson + "\n\n")
	if len(l.RecommendedProcedure) > 0 {
		sb.WriteString("## Recommended Procedure\n\n")
		for _, step := range l.RecommendedProcedure {
			sb.WriteString(fmt.Sprintf("- %s\n", step))
		}
		sb.WriteString("\n")
	}
	if l.Limits != "" {
		sb.WriteString("## Limits\n\n" + l.Limits + "\n\n")
	}
	return sb.String()
}

// RecordHash returns the hash embedded in the materialized record.
func RecordHash(l *domain.Learning) string {
	data := strings.Join([]string{
		string(l.ID), string(l.Status), string(l.Type), l.Title, l.Context,
		l.Observation, l.ReusableLesson,
	}, "\x00")
	sum := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", sum[:])
}

// ReadRecordHash reads record_hash. found distinguishes a missing record.
func ReadRecordHash(path string) (hash string, found bool, err error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("record: read %s: %w", path, readErr)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "record_hash:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "record_hash:")), true, nil
		}
	}
	return "", true, nil
}
