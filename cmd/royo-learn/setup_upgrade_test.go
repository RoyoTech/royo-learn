package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Recorrido F: safe upgrade of already-installed skills ----
//
// These seven tests drive the public `royo-learn setup ...` interface only.
// They prove that updating the binary offers a SAFE path to upgrade the
// incompatible skills already installed on a user's machine (BASELINE-GAP
// Hallazgo 11): fixing the bundled skills in the repo does not repair the
// copies already installed, because InstallSkills never overwrites.

// upgradeManifest mirrors the on-disk per-skill manifest for assertions.
type upgradeManifest struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	SourceSHA256    string `json:"source_sha256"`
	InstalledSHA256 string `json:"installed_sha256"`
	ManagedBy       string `json:"managed_by"`
}

// writeSourceSkill creates <root>/skills/<name>/SKILL.md with a frontmatter
// version and body, and returns the project root.
func writeSourceSkill(t *testing.T, root, name, version, body string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	content := "---\nname: " + name + "\nmetadata:\n  version: \"" + version + "\"\n---\n\n# " + name + "\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}
}

// fakeBinary returns a path to an empty file that satisfies --binary.
func fakeBinary(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "royo-learn.exe")
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile fake binary: %v", err)
	}
	return p
}

// installClaudeSkill installs the skills under projectRoot into the isolated
// Claude Code home, skipping MCP registration.
func installClaudeSkill(t *testing.T, projectRoot string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "install",
		"--agent", "claude-code",
		"--binary", fakeBinary(t),
		"--project-root", projectRoot,
		"--skip-mcp",
		"--json",
	}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("install: code=%d stderr=%s out=%s", code, errOut.String(), out.String())
	}
}

// claudeSkillsDir is the isolated Claude Code skills dir for the current home.
func claudeSkillsDir(home string) string {
	return filepath.Join(home, ".claude", "skills")
}

func manifestPathFor(home, name string) string {
	return filepath.Join(claudeSkillsDir(home), ".royo-learn", "manifests", name+".json")
}

func readManifest(t *testing.T, home, name string) upgradeManifest {
	t.Helper()
	data, err := os.ReadFile(manifestPathFor(home, name))
	if err != nil {
		t.Fatalf("read manifest %s: %v", name, err)
	}
	var m upgradeManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("manifest not JSON: %v\n%s", err, data)
	}
	return m
}

func installedSkillMD(home, name string) string {
	return filepath.Join(claudeSkillsDir(home), name, "SKILL.md")
}

// runUpgrade drives `setup upgrade-skills` for claude-code and returns exit
// code and parsed JSON.
func runUpgrade(t *testing.T, projectRoot string, apply bool) (int, upgradeJSON, string) {
	t.Helper()
	args := []string{
		"setup", "upgrade-skills",
		"--agent", "claude-code",
		"--project-root", projectRoot,
		"--json",
	}
	if apply {
		args = append(args, "--apply")
	}
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	var parsed upgradeJSON
	if len(out.Bytes()) > 0 {
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("upgrade output not JSON: %v\n%s", err, out.String())
		}
	}
	return code, parsed, errOut.String()
}

type upgradeJSON struct {
	Command string `json:"command"`
	Apply   bool   `json:"apply"`
	Results []struct {
		Agent     string `json:"agent"`
		SkillsDir string `json:"skills_dir"`
		Actions   []struct {
			Name        string `json:"name"`
			Status      string `json:"status"`
			FromVersion string `json:"from_version"`
			ToVersion   string `json:"to_version"`
			Backup      string `json:"backup"`
			Candidate   string `json:"candidate"`
			Diff        string `json:"diff"`
			Detail      string `json:"detail"`
		} `json:"actions"`
		Errors []string `json:"errors"`
	} `json:"results"`
}

func actionFor(t *testing.T, j upgradeJSON, name string) (string, string, string, string) {
	t.Helper()
	for _, r := range j.Results {
		for _, a := range r.Actions {
			if a.Name == name {
				return a.Status, a.Backup, a.Candidate, a.Diff
			}
		}
	}
	t.Fatalf("no action for skill %q in %+v", name, j)
	return "", "", "", ""
}

// Test 1: fresh install (no prior skills) installs and writes a manifest.
func TestUpgradeSkills_FreshInstallWritesManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	src := t.TempDir()
	writeSourceSkill(t, src, "demo", "1.0.0", "original body")
	installClaudeSkill(t, src)

	m := readManifest(t, home, "demo")
	if m.Name != "demo" {
		t.Errorf("manifest name = %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("manifest version = %q, want 1.0.0", m.Version)
	}
	if m.ManagedBy != "royo-learn" {
		t.Errorf("managed_by = %q, want royo-learn", m.ManagedBy)
	}
	if m.SourceSHA256 == "" || m.InstalledSHA256 == "" {
		t.Errorf("hashes must be recorded: %+v", m)
	}
	if m.SourceSHA256 != m.InstalledSHA256 {
		t.Errorf("fresh install: source and installed hash must match: %+v", m)
	}
}

// Test 2: upgrade with no user modifications backs up and updates cleanly.
func TestUpgradeSkills_CleanUpgrade(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	code, j, stderr := runUpgrade(t, v2, true)
	if code != exitSuccess {
		t.Fatalf("upgrade apply: code=%d stderr=%s", code, stderr)
	}
	status, backup, _, _ := actionFor(t, j, "demo")
	if status != "upgrade" {
		t.Errorf("status = %q, want upgrade", status)
	}
	if backup == "" {
		t.Errorf("backup path must be recorded")
	}

	got, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "body v2 updated") {
		t.Errorf("installed skill not updated: %s", got)
	}
	m := readManifest(t, home, "demo")
	if m.Version != "2.0.0" {
		t.Errorf("manifest version after upgrade = %q, want 2.0.0", m.Version)
	}
}

// Test 3: upgrade WITH user personalization does NOT overwrite; creates a
// candidate and conflict, original preserved byte-for-byte.
func TestUpgradeSkills_UserModifiedNotOverwritten(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	// User personalizes the installed skill.
	personalized := "---\nname: demo\nmetadata:\n  version: \"1.0.0\"\n---\n\n# demo\n\nMY OWN EDITS\n"
	if err := os.WriteFile(installedSkillMD(home, "demo"), []byte(personalized), 0o644); err != nil {
		t.Fatal(err)
	}

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	code, j, stderr := runUpgrade(t, v2, true)
	if code != exitSuccess {
		t.Fatalf("upgrade apply: code=%d stderr=%s", code, stderr)
	}
	status, _, candidate, diff := actionFor(t, j, "demo")
	if status != "conflict" {
		t.Errorf("status = %q, want conflict", status)
	}
	if candidate == "" {
		t.Errorf("candidate path must be recorded on conflict")
	}
	if diff == "" {
		t.Errorf("diff must be recorded on conflict")
	}

	// Original must be preserved byte-for-byte.
	got, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != personalized {
		t.Fatalf("user-modified skill was overwritten:\n got=%q\nwant=%q", got, personalized)
	}
	// Candidate must contain the new version.
	candMD, err := os.ReadFile(filepath.Join(candidate, "SKILL.md"))
	if err != nil {
		t.Fatalf("candidate SKILL.md missing: %v", err)
	}
	if !strings.Contains(string(candMD), "body v2 updated") {
		t.Errorf("candidate does not hold new version: %s", candMD)
	}
}

// Test 4: a backup is created before any overwrite and is restorable.
func TestUpgradeSkills_BackupIsRestorable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	original, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	code, j, stderr := runUpgrade(t, v2, true)
	if code != exitSuccess {
		t.Fatalf("upgrade apply: code=%d stderr=%s", code, stderr)
	}
	_, backup, _, _ := actionFor(t, j, "demo")
	if backup == "" {
		t.Fatal("backup path must be recorded")
	}
	backupMD, err := os.ReadFile(filepath.Join(backup, "SKILL.md"))
	if err != nil {
		t.Fatalf("backup SKILL.md missing: %v", err)
	}
	if string(backupMD) != string(original) {
		t.Fatalf("backup does not match original:\n got=%q\nwant=%q", backupMD, original)
	}
}

// Test 5: --dry-run (default) writes nothing but reports what WOULD happen.
func TestUpgradeSkills_DryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	before, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	manBefore, err := os.ReadFile(manifestPathFor(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	code, j, stderr := runUpgrade(t, v2, false) // dry-run is default
	if code != exitSuccess {
		t.Fatalf("upgrade dry-run: code=%d stderr=%s", code, stderr)
	}
	if j.Apply {
		t.Errorf("dry-run must report apply=false")
	}
	status, _, _, _ := actionFor(t, j, "demo")
	if status != "upgrade" {
		t.Errorf("dry-run status = %q, want upgrade (would upgrade)", status)
	}

	after, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("dry-run modified the installed skill")
	}
	manAfter, err := os.ReadFile(manifestPathFor(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manAfter) != string(manBefore) {
		t.Errorf("dry-run modified the manifest")
	}
	// No backups directory should have been created.
	backupsDir := filepath.Join(claudeSkillsDir(home), ".royo-learn", "backups")
	if entries, err := os.ReadDir(backupsDir); err == nil && len(entries) > 0 {
		t.Errorf("dry-run created backups: %v", entries)
	}
}

// Test 6: idempotent repeat — the second apply run is a no-op.
func TestUpgradeSkills_IdempotentRepeat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	if code, _, stderr := runUpgrade(t, v2, true); code != exitSuccess {
		t.Fatalf("first upgrade: code=%d stderr=%s", code, stderr)
	}
	// Second run against the same source: nothing to do.
	code, j, stderr := runUpgrade(t, v2, true)
	if code != exitSuccess {
		t.Fatalf("second upgrade: code=%d stderr=%s", code, stderr)
	}
	status, _, _, _ := actionFor(t, j, "demo")
	if status != "up_to_date" {
		t.Errorf("second run status = %q, want up_to_date", status)
	}
}

// Test 7: recovery after a failure mid-upgrade — no half-written skill; the
// original is preserved and a subsequent run recovers.
func TestUpgradeSkills_RecoveryAfterFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	v1 := t.TempDir()
	writeSourceSkill(t, v1, "demo", "1.0.0", "body v1")
	installClaudeSkill(t, v1)

	original, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	v2 := t.TempDir()
	writeSourceSkill(t, v2, "demo", "2.0.0", "body v2 updated")

	// Plant an obstacle: a regular FILE where the staging directory must be
	// created. This forces the upgrade to fail before touching the original.
	stagingObstacle := filepath.Join(claudeSkillsDir(home), ".royo-learn", "staging")
	if err := os.MkdirAll(filepath.Dir(stagingObstacle), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagingObstacle, []byte("obstacle"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, _ := runUpgrade(t, v2, true)
	if code == exitSuccess {
		t.Fatalf("upgrade should fail while staging is blocked")
	}
	// Original preserved byte-for-byte.
	got, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("failed upgrade left a half-written skill:\n got=%q\nwant=%q", got, original)
	}
	// Manifest still at v1.
	if m := readManifest(t, home, "demo"); m.Version != "1.0.0" {
		t.Errorf("manifest changed on failed upgrade: %q", m.Version)
	}

	// Recovery path: remove the obstacle and re-run.
	if err := os.Remove(stagingObstacle); err != nil {
		t.Fatal(err)
	}
	code, j, stderr := runUpgrade(t, v2, true)
	if code != exitSuccess {
		t.Fatalf("recovery upgrade: code=%d stderr=%s", code, stderr)
	}
	status, _, _, _ := actionFor(t, j, "demo")
	if status != "upgrade" {
		t.Errorf("recovery status = %q, want upgrade", status)
	}
	after, err := os.ReadFile(installedSkillMD(home, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "body v2 updated") {
		t.Errorf("recovery did not update the skill: %s", after)
	}
}
