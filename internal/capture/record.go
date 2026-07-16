package capture

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// WriteRecord writes a Markdown record for a learning into dir/<learningID>.md.
// The record uses YAML front matter followed by structured body sections.
func WriteRecord(dir string, learning *domain.Learning) error {
	if learning == nil {
		return fmt.Errorf("record: nil learning")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("record: mkdir: %w", err)
	}

	content := buildRecordContent(learning)
	path := filepath.Join(dir, string(learning.ID)+".md")

	// Atomic write: write to temp, then rename.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("record: write: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("record: rename: %w", err)
	}

	return nil
}

func buildRecordContent(l *domain.Learning) string {
	now := time.Now().UTC().Format(time.RFC3339)
	recordHash := computeRecordHash(l)

	var sb strings.Builder

	// YAML front matter.
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", l.ID))
	sb.WriteString(fmt.Sprintf("status: %s\n", l.Status))
	sb.WriteString(fmt.Sprintf("type: %s\n", l.Type))
	sb.WriteString(fmt.Sprintf("scope_guess: %s\n", l.ScopeGuess))
	sb.WriteString(fmt.Sprintf("confidence: %s\n", l.Confidence))
	sb.WriteString(fmt.Sprintf("evidence_level: %s\n", l.EvidenceLevel))
	sb.WriteString(fmt.Sprintf("normalized_hash: %s\n", l.NormalizedHash))
	sb.WriteString(fmt.Sprintf("fingerprint: %s\n", l.Fingerprint))
	sb.WriteString(fmt.Sprintf("record_hash: %s\n", recordHash))
	sb.WriteString(fmt.Sprintf("created_at: %s\n", l.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", now))
	if len(l.RetrievalTerms) > 0 {
		sb.WriteString(fmt.Sprintf("retrieval_terms:\n"))
		for _, t := range l.RetrievalTerms {
			sb.WriteString(fmt.Sprintf("  - %s\n", t))
		}
	}
	sb.WriteString("---\n\n")

	// Body.
	sb.WriteString(fmt.Sprintf("# %s\n\n", l.Title))
	sb.WriteString("## Context\n\n")
	sb.WriteString(l.Context + "\n\n")
	sb.WriteString("## Observation\n\n")
	sb.WriteString(l.Observation + "\n\n")
	sb.WriteString("## Reusable Lesson\n\n")
	sb.WriteString(l.ReusableLesson + "\n\n")

	if len(l.RecommendedProcedure) > 0 {
		sb.WriteString("## Recommended Procedure\n\n")
		for _, step := range l.RecommendedProcedure {
			sb.WriteString(fmt.Sprintf("- %s\n", step))
		}
		sb.WriteString("\n")
	}

	if l.Limits != "" {
		sb.WriteString("## Limits\n\n")
		sb.WriteString(l.Limits + "\n\n")
	}

	return sb.String()
}

// RecordHash returns the content hash a materialized Markdown record embeds for
// the given learning. Coherence checks compare this against the hash stored in
// the on-disk record to detect SQLite<->Markdown divergence (D6).
func RecordHash(l *domain.Learning) string {
	return computeRecordHash(l)
}

// ReadRecordHash reads the record_hash field from a materialized record file.
// found is false when the file does not exist, so a missing record is
// distinguishable from a divergent one.
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
	return "", true, nil // file exists but has no record_hash line
}

func computeRecordHash(l *domain.Learning) string {
	data := strings.Join([]string{
		string(l.ID),
		string(l.Status),
		string(l.Type),
		l.Title,
		l.Context,
		l.Observation,
		l.ReusableLesson,
	}, "\x00")
	sum := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", sum)
}
