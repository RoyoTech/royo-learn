package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkills_CopiesToTarget(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a skill dir with SKILL.md.
	skillDir := filepath.Join(srcDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	skillContent := []byte("# Test Skill\n\nThis is a test skill.\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Add an extra file.
	if err := os.WriteFile(filepath.Join(skillDir, "extra.txt"), []byte("extra"), 0o644); err != nil {
		t.Fatalf("WriteFile extra: %v", err)
	}

	result, err := InstallSkills(srcDir, dstDir)
	if err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if result.Installed == 0 {
		t.Fatal("expected at least one skill installed")
	}

	// Verify SKILL.md was copied.
	installedPath := filepath.Join(dstDir, "test-skill", "SKILL.md")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile(installed): %v", err)
	}
	if string(data) != string(skillContent) {
		t.Fatalf("content mismatch: got %q, want %q", string(data), string(skillContent))
	}
}

func TestInstallSkills_SkipsExisting(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source skill.
	skillDir := filepath.Join(srcDir, "existing-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("new content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Pre-create destination with different content.
	dstSkillDir := filepath.Join(dstDir, "existing-skill")
	if err := os.MkdirAll(dstSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll dst: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dstSkillDir, "SKILL.md"), []byte("existing content"), 0o644); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}

	result, err := InstallSkills(srcDir, dstDir)
	if err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if result.Installed != 0 {
		t.Fatalf("expected 0 installed (skipped), got %d", result.Installed)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", result.Skipped)
	}

	// Verify existing content was NOT overwritten.
	data, err := os.ReadFile(filepath.Join(dstSkillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "existing content" {
		t.Fatalf("existing file was overwritten: got %q", string(data))
	}
}

func TestInstallSkills_EmptySource(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	result, err := InstallSkills(srcDir, dstDir)
	if err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if result.Installed != 0 {
		t.Fatalf("expected 0 installed, got %d", result.Installed)
	}
}
