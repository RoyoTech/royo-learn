package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"agent-royo-learn/internal/config"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func TestRunVersionJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"version", "--json"}, &stdout, &stderr); got != exitSuccess {
		t.Fatalf("run() exit code = %d, want %d", got, exitSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful stderr = %q, want empty", stderr.String())
	}

	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
}

func TestRunVersionHuman(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"version"}, &stdout, &stderr); got != exitSuccess {
		t.Fatalf("run() exit code = %d, want %d (stderr=%q)", got, exitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "royo-learn") {
		t.Errorf("stdout = %q, want it to contain %q", out, "royo-learn")
	}
	if json.Valid(stdout.Bytes()) && strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("stdout = %q, want human-readable text, not JSON", out)
	}
}

func TestRunRejectsInvalidArgumentsOnStderr(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"unknown"}, &stdout, &stderr); got != exitInvalidArguments {
		t.Fatalf("run() exit code = %d, want %d", got, exitInvalidArguments)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertInvalidArgumentsDiagnostic(t, stderr.Bytes())
}

func TestRunDoesNotCreateDatabaseState(t *testing.T) {
	directory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	before, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir before run: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("temporary directory is not empty: %v", before)
	}

	var stdout, stderr bytes.Buffer
	if got := run([]string{"version", "--json"}, &stdout, &stderr); got != exitSuccess {
		t.Fatalf("run() exit code = %d, want %d", got, exitSuccess)
	}
	after, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir after run: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("version command created files: %v", after)
	}
}

// ---------------------------------------------------------------------------
// Init tests (RED — init subcommand not implemented yet)
// ---------------------------------------------------------------------------

func TestRunInitCreatesProjectLayout(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"init", "--project-root", root}, &stdout, &stderr)
	if exitCode != exitSuccess {
		t.Fatalf("init exit code = %d, want %d\nstderr: %s", exitCode, exitSuccess, stderr.String())
	}

	// Verify .royo-learn/ directory exists.
	royoDir := filepath.Join(root, ".royo-learn")
	info, err := os.Stat(royoDir)
	if err != nil {
		t.Fatalf(".royo-learn not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".royo-learn is not a directory")
	}

	// Verify subdirectories.
	for _, sub := range []string{"records", "evidence", "backups"} {
		path := filepath.Join(royoDir, sub)
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			t.Fatalf("subdirectory %s not created: %v", sub, err)
		}
	}

	// Verify config.yaml is valid YAML and matches defaults.
	configPath := filepath.Join(royoDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.yaml is not valid YAML: %v", err)
	}
	if cfg.Version != config.DefaultSchemaVersion {
		t.Fatalf("config version = %d, want %d", cfg.Version, config.DefaultSchemaVersion)
	}

	// Verify .gitignore exists.
	gitignorePath := filepath.Join(royoDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
}

func TestRunInitDoesNotOverwriteWithoutForce(t *testing.T) {
	root := t.TempDir()

	// First init.
	var buf bytes.Buffer
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("first init failed: %s", buf.String())
	}

	// Write a marker file in records/.
	markerPath := filepath.Join(root, ".royo-learn", "records", "marker.txt")
	if err := os.WriteFile(markerPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Second init WITHOUT --force must succeed and NOT overwrite existing files.
	buf.Reset()
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("second init without --force failed: exit=%d stderr=%s", got, buf.String())
	}

	// Marker file must still exist.
	if content, err := os.ReadFile(markerPath); err != nil || string(content) != "keep me" {
		t.Fatalf("marker was overwritten: %v / content=%q", err, content)
	}
}

func TestRunInitForceRecreatesGeneratedFiles(t *testing.T) {
	root := t.TempDir()

	// First init.
	var buf bytes.Buffer
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("first init failed: %s", buf.String())
	}

	// Write a marker in records/ to verify it is NOT touched.
	markerPath := filepath.Join(root, ".royo-learn", "records", "marker.txt")
	if err := os.WriteFile(markerPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Write a custom config to verify --force overwrites it.
	configPath := filepath.Join(root, ".royo-learn", "config.yaml")
	customCfg := "version: 99\nproject:\n  name: custom\n"
	if err := os.WriteFile(configPath, []byte(customCfg), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}

	// Second init WITH --force must recreate generated files.
	buf.Reset()
	if got := run([]string{"init", "--project-root", root, "--force"}, &buf, &buf); got != exitSuccess {
		t.Fatalf("init --force failed: %s", buf.String())
	}

	// config.yaml must be back to defaults.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after --force: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.yaml after --force is not valid YAML: %v", err)
	}
	if cfg.Version != config.DefaultSchemaVersion {
		t.Fatalf("config version after --force = %d, want %d", cfg.Version, config.DefaultSchemaVersion)
	}
	// Custom field should be gone.
	if cfg.Project.Name == "custom" {
		t.Fatal("--force did not overwrite custom config")
	}

	// Marker in records/ must still exist (never overwritten).
	if content, err := os.ReadFile(markerPath); err != nil || string(content) != "keep me" {
		t.Fatalf("records marker was overwritten by --force: %v / content=%q", err, content)
	}
}

func TestRunInitPreservesExistingFilesWithoutForce(t *testing.T) {
	root := t.TempDir()
	royoDir := filepath.Join(root, ".royo-learn")
	os.MkdirAll(royoDir, 0o755)

	// Pre-create a conflicting file with garbage content.
	configPath := filepath.Join(royoDir, "config.yaml")
	garbageContent := []byte("garbage")
	if err := os.WriteFile(configPath, garbageContent, 0o644); err != nil {
		t.Fatalf("write garbage config: %v", err)
	}

	// init WITHOUT --force must succeed and NOT overwrite existing config.yaml.
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"init", "--project-root", root}, &stdout, &stderr)
	if exitCode != exitSuccess {
		t.Fatalf("init on existing config.yaml without --force failed: exit=%d\nstderr: %s", exitCode, stderr.String())
	}

	// Verify the garbage content was preserved.
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after init: %v", err)
	}
	if string(content) != string(garbageContent) {
		t.Fatalf("config was overwritten: got %q, want %q", string(content), string(garbageContent))
	}
}

// ---------------------------------------------------------------------------
// Doctor tests (RED — doctor subcommand not implemented yet)
// ---------------------------------------------------------------------------

func TestRunDoctorInsideProject(t *testing.T) {
	root := t.TempDir()

	// Initialize the project first.
	var buf bytes.Buffer
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("init failed: %s", buf.String())
	}

	// Doctor inside the initialized project must exit 0.
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"doctor", "--project-root", root, "--json"}, &stdout, &stderr)
	if exitCode != exitSuccess {
		t.Fatalf("doctor exit code = %d, want %d\nstderr: %s", exitCode, exitSuccess, stderr.String())
	}

	// Verify JSON output has ok: true and a project check.
	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	ok, _ := report["ok"].(bool)
	if !ok {
		t.Fatalf("doctor report ok=false: %v", report)
	}

	checks, ok := report["checks"].([]interface{})
	if !ok {
		t.Fatalf("doctor report missing checks array")
	}

	foundProject := false
	for _, c := range checks {
		cm, _ := c.(map[string]interface{})
		if cm["name"] == "project" {
			foundProject = true
			if cm["status"] != "pass" {
				t.Fatalf("project check status=%v want pass: %v", cm["status"], cm["message"])
			}
		}
	}
	if !foundProject {
		t.Fatal("doctor report missing 'project' check")
	}
}

func TestRunDoctorOutsideProject(t *testing.T) {
	root := t.TempDir()
	// No .royo-learn/ exists — this is a bare directory.

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"doctor", "--project-root", root}, &stdout, &stderr)
	if exitCode != exitProjectNotFound {
		t.Fatalf("doctor outside project exit code = %d, want %d\nstdout: %s\nstderr: %s",
			exitCode, exitProjectNotFound, stdout.String(), stderr.String())
	}

	// Verify stderr contains a proper error envelope with project_not_found code.
	assertErrorEnvelopeCode(t, stderr.Bytes(), "project_not_found")
}

func TestRunDoctorAmbiguousProject(t *testing.T) {
	parent := t.TempDir()

	// Create two sibling directories, both with .royo-learn/config.yaml.
	for _, name := range []string{"a", "b"} {
		sub := filepath.Join(parent, name)
		royoDir := filepath.Join(sub, ".royo-learn")
		os.MkdirAll(royoDir, 0o755)
		os.WriteFile(filepath.Join(royoDir, "config.yaml"),
			[]byte(fmt.Sprintf("project:\n  name: %s\n", name)), 0o644)
		os.MkdirAll(filepath.Join(royoDir, "records"), 0o755)
		os.MkdirAll(filepath.Join(royoDir, "evidence"), 0o755)
		os.MkdirAll(filepath.Join(royoDir, "backups"), 0o755)
	}

	// CWD is inside "a" but "b" also has a marker at the same level.
	cwd := filepath.Join(parent, "a", "src")
	os.MkdirAll(cwd, 0o755)

	previous, _ := os.Getwd()
	os.Chdir(cwd)
	t.Cleanup(func() { os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"doctor"}, &stdout, &stderr)
	if exitCode != exitAmbiguousProject {
		t.Fatalf("doctor ambiguous exit code = %d, want %d\nstderr: %s",
			exitCode, exitAmbiguousProject, stderr.String())
	}

	assertErrorEnvelopeCode(t, stderr.Bytes(), "ambiguous_project")
}

func TestRunDoctorSingleCheck(t *testing.T) {
	root := t.TempDir()
	// Initialize first.
	var buf bytes.Buffer
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("init failed: %s", buf.String())
	}

	// Doctor with --check filter.
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"doctor", "--project-root", root, "--check", "config", "--json"}, &stdout, &stderr)
	if exitCode != exitSuccess {
		t.Fatalf("doctor --check config exit = %d, want %d\nstderr: %s", exitCode, exitSuccess, stderr.String())
	}

	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Subprocess acceptance tests
// ---------------------------------------------------------------------------

func TestBinaryInitAndDoctorFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	binary := filepath.Join(t.TempDir(), "royo-learn")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	projectRoot := t.TempDir()

	// Step 1: init
	initCmd := exec.Command(binary, "init", "--project-root", projectRoot)
	initOut, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, initOut)
	}

	// Step 2: doctor --json inside initialized project
	doctorCmd := exec.Command(binary, "doctor", "--project-root", projectRoot, "--json")
	var doctorStdout, doctorStderr bytes.Buffer
	doctorCmd.Stdout = &doctorStdout
	doctorCmd.Stderr = &doctorStderr
	if err := doctorCmd.Run(); err != nil {
		t.Fatalf("doctor failed: %v\nstderr: %s", err, doctorStderr.String())
	}

	var report map[string]any
	if err := json.Unmarshal(doctorStdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor stdout is not valid JSON: %v\nstdout: %s", err, doctorStdout.String())
	}
	ok, _ := report["ok"].(bool)
	if !ok {
		t.Fatalf("doctor report ok=false: %v", report)
	}
	foundProject := false
	if checks, ok := report["checks"].([]interface{}); ok {
		for _, c := range checks {
			cm, _ := c.(map[string]interface{})
			if cm["name"] == "project" {
				foundProject = true
				break
			}
		}
	}
	if !foundProject {
		t.Fatal("doctor report missing 'project' check")
	}
}

func TestBinaryDoctorOutsideProject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	binary := filepath.Join(t.TempDir(), "royo-learn")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	// Run doctor in a bare directory (no project).
	cmd := exec.Command(binary, "doctor", "--project-root", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected exit error, got: %v", err)
	}
	if exitErr.ExitCode() != exitProjectNotFound {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), exitProjectNotFound)
	}

	// Verify stderr contains error envelope.
	assertErrorEnvelopeCode(t, stderr.Bytes(), "project_not_found")
}

func TestVersionBinaryStreamContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	binary := filepath.Join(t.TempDir(), "royo-learn")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	tests := []struct {
		name      string
		args      []string
		exitCode  int
		assertion func(*testing.T, []byte, []byte)
	}{
		{
			name:     "version JSON",
			args:     []string{"version", "--json"},
			exitCode: exitSuccess,
			assertion: func(t *testing.T, stdout, stderr []byte) {
				t.Helper()
				if !json.Valid(stdout) {
					t.Errorf("stdout = %q, want valid JSON", stdout)
				}
				if len(stderr) != 0 {
					t.Errorf("stderr = %q, want empty", stderr)
				}
			},
		},
		{
			name:     "invalid arguments",
			args:     []string{"unknown"},
			exitCode: exitInvalidArguments,
			assertion: func(t *testing.T, stdout, stderr []byte) {
				t.Helper()
				if len(stdout) != 0 {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				assertInvalidArgumentsDiagnostic(t, stderr)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := exec.Command(binary, tt.args...)
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if tt.exitCode == exitSuccess {
				if err != nil {
					t.Fatalf("command failed: %v", err)
				}
			} else if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != tt.exitCode {
				t.Fatalf("command error = %v, want exit code %d", err, tt.exitCode)
			}
			tt.assertion(t, stdout.Bytes(), stderr.Bytes())
		})
	}
}

// ---------------------------------------------------------------------------
// Preview, Publish, Rollback tests (RED — subcommands not implemented yet)
// ---------------------------------------------------------------------------

func TestRunPreviewMissingLearningID(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"preview"}, &stdout, &stderr)
	if exitCode != exitFailure {
		t.Fatalf("preview without --learning-id exit = %d, want %d", exitCode, exitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "invalid_argument")
}

func TestRunPublishMissingLearningID(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"publish"}, &stdout, &stderr)
	if exitCode != exitFailure {
		t.Fatalf("publish without --learning-id exit = %d, want %d", exitCode, exitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "invalid_argument")
}

func TestRunRollbackMissingPublicationID(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"rollback"}, &stdout, &stderr)
	if exitCode != exitFailure {
		t.Fatalf("rollback without --journal-id exit = %d, want %d", exitCode, exitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "invalid_argument")
}

func TestRunPreviewEndToEnd(t *testing.T) {
	root := t.TempDir()
	learningID := setupApprovedLearning(t, root)
	if learningID == "" {
		t.Fatal("failed to set up approved learning")
	}

	// Preview must succeed.
	var prevOut, prevErr bytes.Buffer
	exitCode := run([]string{
		"preview",
		"--project-root", root,
		"--learning-id", learningID,
		"--json",
	}, &prevOut, &prevErr)
	if exitCode != exitSuccess {
		t.Fatalf("preview exit = %d, want %d\nstderr: %s", exitCode, exitSuccess, prevErr.String())
	}

	var prevResult map[string]interface{}
	if err := json.Unmarshal(prevOut.Bytes(), &prevResult); err != nil {
		t.Fatalf("preview stdout not valid JSON: %v\n%s", err, prevOut.String())
	}
	if _, ok := prevResult["preview_hash"]; !ok {
		t.Fatal("preview JSON missing preview_hash")
	}
}

func TestRunPublishAndRollbackEndToEnd(t *testing.T) {
	root := t.TempDir()
	learningID := setupApprovedLearning(t, root)
	if learningID == "" {
		t.Fatal("failed to set up approved learning")
	}

	// Generate preview to get preview hash.
	var prevOut, prevErr bytes.Buffer
	if got := run([]string{
		"preview",
		"--project-root", root,
		"--learning-id", learningID,
		"--json",
	}, &prevOut, &prevErr); got != exitSuccess {
		t.Fatalf("preview failed: stderr=%s", prevErr.String())
	}

	var prevResult map[string]interface{}
	json.Unmarshal(prevOut.Bytes(), &prevResult)
	previewHash, _ := prevResult["preview_hash"].(string)

	// Publish with force (bypass dirty check in non-git dir).
	var pubOut, pubErr bytes.Buffer
	pubExit := run([]string{
		"publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--apply",
		"--force",
		"--json",
	}, &pubOut, &pubErr)
	if pubExit != exitSuccess {
		t.Fatalf("publish exit = %d, want %d\nstderr: %s", pubExit, exitSuccess, pubErr.String())
	}

	var pubResult map[string]interface{}
	if err := json.Unmarshal(pubOut.Bytes(), &pubResult); err != nil {
		t.Fatalf("publish stdout not valid JSON: %v\n%s", err, pubOut.String())
	}
	publicationID, _ := pubResult["publication_id"].(string)
	if publicationID == "" {
		t.Fatal("publish result missing publication_id")
	}

	// Rollback.
	var rbOut, rbErr bytes.Buffer
	rbExit := run([]string{
		"rollback",
		"--project-root", root,
		"--journal-id", publicationID,
		"--json",
	}, &rbOut, &rbErr)
	if rbExit != exitSuccess {
		t.Fatalf("rollback exit = %d, want %d\nstderr: %s", rbExit, exitSuccess, rbErr.String())
	}
}

func TestRunPreviewWithoutProjectMarker(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"preview",
		"--project-root", root,
		"--learning-id", "nonexistent",
	}, &stdout, &stderr)
	if exitCode != exitProjectNotFound {
		t.Fatalf("preview outside project exit = %d, want %d", exitCode, exitProjectNotFound)
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "project_not_found")
}

// setupApprovedLearning initializes a project, opens the DB, inserts a learning
// with approved status and a curation targeting a skill, and returns the learning ID.
func setupApprovedLearning(t *testing.T, root string) string {
	t.Helper()

	// Initialize project.
	var buf bytes.Buffer
	if got := run([]string{"init", "--project-root", root}, &buf, &buf); got != exitSuccess {
		t.Fatalf("init failed: %s", buf.String())
		return ""
	}

	// Capture via CLI to get a learning ID.
	var capOut, capErr bytes.Buffer
	if got := run([]string{
		"capture",
		"--project-root", root,
		"--title", "E2E Test Skill",
		"--context", "end to end testing",
		"--observation", "observed during e2e",
		"--lesson", "always e2e test publish flows",
		"--json",
	}, &capOut, &capErr); got != exitSuccess {
		t.Fatalf("capture failed: stderr=%s", capErr.String())
		return ""
	}

	var capResult map[string]interface{}
	if err := json.Unmarshal(capOut.Bytes(), &capResult); err != nil {
		t.Fatalf("capture JSON: %v", err)
		return ""
	}
	learningID, _ := capResult["learning_id"].(string)

	// Update learning directly in DB: set status=approved and add curation.
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
		return ""
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
		return ""
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
		return ""
	}

	// Read learning to get current fields.
	learning, err := storage.GetLearning(ctx, tx, domain.LearningID(learningID))
	if err != nil {
		tx.Rollback()
		t.Fatalf("get learning: %v", err)
		return ""
	}

	// Set approved status, destination, and evidence level.
	learning.Status = domain.StatusApproved
	learning.EvidenceLevel = domain.EvidenceModerate
	learning.ApprovedDestination = &domain.Destination{
		Type:     domain.DestSkill,
		Root:     "skills",
		Path:     "e2e-test/SKILL.md",
		Required: false,
	}
	scope := domain.ScopeProject
	learning.ApprovedScope = &scope

	if err := storage.UpdateLearning(ctx, tx, learning); err != nil {
		tx.Rollback()
		t.Fatalf("update learning: %v", err)
		return ""
	}

	// Insert a curation record for this learning.
	curationID := domain.CurationID(uuid.Must(uuid.NewV7()).String())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO curations (id, learning_id, decision, rationale, destination_json, validation_json, acceptance_checks_json, rollback_condition, actor_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(curationID),
		learningID,
		string(domain.CurationApproveNewSkill),
		"e2e test curation",
		`{"type":"skill","root":"skills","path":"e2e-test/SKILL.md","required":false}`,
		`[]`,
		`[]`,
		"",
		`{"kind":"human","name":"cli-user","model":"","session_id":""}`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		tx.Rollback()
		t.Fatalf("insert curation: %v", err)
		return ""
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
		return ""
	}

	return learningID
}

// ---------------------------------------------------------------------------
// E2E tests
// ---------------------------------------------------------------------------

func TestRunE2ETempCompletesAllSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"e2e", "--temp"}, &stdout, &stderr)
	if exitCode != exitSuccess {
		t.Fatalf("e2e exit code = %d, want %d\nstderr: %s\nstdout: %s",
			exitCode, exitSuccess, stderr.String(), stdout.String())
	}

	var result e2eResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("e2e stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if result.Total == 0 {
		t.Fatal("e2e result has zero steps")
	}
	if result.Failed > 0 {
		t.Fatalf("e2e had %d failing steps:\n", result.Failed)
		for _, s := range result.Steps {
			if !s.Passed {
				t.Logf("  FAIL %s: %s", s.Step, s.Error)
			}
		}
	}
	if result.Summary == "" {
		t.Error("e2e result missing summary")
	}
}

func TestRunE2ERequiresTempFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"e2e"}, &stdout, &stderr)
	if exitCode != exitFailure {
		t.Fatalf("e2e without --temp exit = %d, want %d", exitCode, exitFailure)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertInvalidArgumentsDiagnostic(t *testing.T, output []byte) {
	t.Helper()

	var diagnostic map[string]json.RawMessage
	if err := json.Unmarshal(output, &diagnostic); err != nil {
		t.Fatalf("stderr = %q, want one JSON diagnostic: %v", output, err)
	}
	if len(diagnostic) != 5 {
		t.Errorf("diagnostic field count = %d, want 5: %s", len(diagnostic), output)
	}
	assertDiagnosticField(t, diagnostic, "code", `"invalid_argument"`)
	assertDiagnosticField(t, diagnostic, "message", `"invalid arguments: expected \"version --json\""`)
	assertDiagnosticField(t, diagnostic, "recoverable", "true")
	assertDiagnosticField(t, diagnostic, "details", "{}")
	assertDiagnosticField(t, diagnostic, "next_action", `"run \"royo-learn version --json\""`)
}

func assertDiagnosticField(t *testing.T, diagnostic map[string]json.RawMessage, field, want string) {
	t.Helper()
	if got, ok := diagnostic[field]; !ok {
		t.Errorf("diagnostic omitted %q", field)
	} else if string(got) != want {
		t.Errorf("diagnostic[%q] = %s, want %s", field, got, want)
	}
}

func assertErrorEnvelopeCode(t *testing.T, output []byte, wantCode string) {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", err, output)
	}
	if code, ok := envelope["code"]; !ok || string(code) != `"`+wantCode+`"` {
		t.Fatalf("stderr error code = %s, want %q", code, wantCode)
	}
}
