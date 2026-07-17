package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/testutil"
)

func TestWriteRecord(t *testing.T) {
	t.Parallel()

	dir := testutil.TempDir(t)
	l := newTestCaptureLearning()
	l.NormalizedHash = "abc123def456"

	recordsDir := filepath.Join(dir, "records")
	if err := record.WriteRecord(recordsDir, l); err != nil {
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
	l := newTestCaptureLearning()
	recordsDir := filepath.Join(dir, "new-records")

	if err := record.WriteRecord(recordsDir, l); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	if _, err := os.Stat(recordsDir); err != nil {
		t.Fatalf("records directory was not created: %v", err)
	}
}

func TestWriteRecordNilLearning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := record.WriteRecord(dir, nil)
	if err == nil {
		t.Fatal("WriteRecord(nil): expected error")
	}
}
