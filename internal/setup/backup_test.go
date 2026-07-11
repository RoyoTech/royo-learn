package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupConfig_CreatesTimestampedCopy(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	original := []byte(`{"mcpServers":{},"skills":{}}`)
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	backupPath, err := BackupConfig(configPath)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}
	if backupPath == "" {
		t.Fatal("expected non-empty backup path")
	}

	// Verify backup exists with same content.
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backup): %v", err)
	}
	if string(backupData) != string(original) {
		t.Fatalf("backup content mismatch: got %q, want %q", string(backupData), string(original))
	}

	// Backup should be in same directory.
	if filepath.Dir(backupPath) != filepath.Dir(configPath) {
		t.Fatalf("backup dir = %q, want %q", filepath.Dir(backupPath), filepath.Dir(configPath))
	}
}

func TestBackupConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nonexistent.json")

	_, err := BackupConfig(configPath)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBackupConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(configPath, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	backupPath, err := BackupConfig(configPath)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}

	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backup): %v", err)
	}
	if len(backupData) != 0 {
		t.Fatalf("expected empty backup, got %q", string(backupData))
	}
}
