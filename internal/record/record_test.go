package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/testutil"
)

func newTestLearning() *domain.Learning {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.Learning{
		ID:                   "learn-rec-001",
		ProjectID:            "proj-rec-001",
		Status:               domain.StatusCaptured,
		Type:                 domain.TypeProcedure,
		Title:                "Test Record",
		Context:              "Testing the record materializer",
		Observation:          "The record is written correctly",
		ReusableLesson:       "A derived record must follow the truth it derives from",
		RecommendedProcedure: []string{"verify dedup", "check markdown"},
		Limits:               "n/a",
		ScopeGuess:           domain.ScopeProject,
		Confidence:           domain.ConfidenceHigh,
		EvidenceLevel:        domain.EvidenceModerate,
		ProposedDestination:  domain.DestProject,
		RetrievalTerms:       []string{"record", "test"},
		Fingerprint:          "fp-rec-001",
		Actor: domain.Actor{
			Kind:      "agent",
			Name:      "test-agent",
			Model:     "test-model",
			SessionID: "sess-001",
		},
		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestWriteRecord(t *testing.T) {
	t.Parallel()

	dir := testutil.TempDir(t)
	l := newTestLearning()
	l.NormalizedHash = "abc123def456"

	recordsDir := filepath.Join(dir, "records")
	if err := WriteRecord(recordsDir, l); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	// Verify the file exists.
	recordPath := filepath.Join(recordsDir, string(l.ID)+".md")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("ReadFile %q: %v", recordPath, err)
	}

	content := string(data)

	// Check YAML front matter.
	if !strings.Contains(content, "---") {
		t.Error("record missing YAML front matter")
	}

	// Check key fields.
	if !strings.Contains(content, string(l.ID)) {
		t.Error("record does not contain learning ID")
	}
	if !strings.Contains(content, l.Title) {
		t.Error("record does not contain title")
	}
	if !strings.Contains(content, l.ReusableLesson) {
		t.Error("record does not contain reusable lesson")
	}
	if !strings.Contains(content, "normalized_hash") {
		t.Error("record missing normalized_hash in front matter")
	}
	if !strings.Contains(content, "status") {
		t.Error("record missing status in front matter")
	}
	if !strings.Contains(content, "record_hash") {
		t.Error("record missing record_hash in front matter")
	}
}

func TestWriteRecordCreatesDir(t *testing.T) {
	t.Parallel()

	dir := testutil.TempDir(t)
	l := newTestLearning()
	recordsDir := filepath.Join(dir, "new-records")

	if err := WriteRecord(recordsDir, l); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	if _, err := os.Stat(recordsDir); err != nil {
		t.Fatalf("records directory was not created: %v", err)
	}
}

func TestWriteRecordNilLearning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := WriteRecord(dir, nil)
	if err == nil {
		t.Fatal("WriteRecord(nil): expected error")
	}
}
