package record

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestWriteRecordRoundTripHash(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	learning := &domain.Learning{
		ID: "learning-1", Status: domain.StatusApproved, Type: domain.TypeProcedure,
		Title: "Title", Context: "Context", Observation: "Observation",
		ReusableLesson: "Lesson", CreatedAt: time.Now().UTC(),
	}
	if err := WriteRecord(dir, learning); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	hash, found, err := ReadRecordHash(filepath.Join(dir, "learning-1.md"))
	if err != nil {
		t.Fatalf("ReadRecordHash: %v", err)
	}
	if !found || hash != RecordHash(learning) {
		t.Fatalf("record hash = %q found=%v, want %q", hash, found, RecordHash(learning))
	}
}

func TestWriteRecordRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "records")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}
	learning := &domain.Learning{ID: "learning-1", CreatedAt: time.Now().UTC()}
	if err := WriteRecord(link, learning); err == nil {
		t.Fatal("WriteRecord followed a symlinked record directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read outside directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("record escaped through symlink: %v", entries)
	}
}
