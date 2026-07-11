package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// RED phase: these tests reference production code that does NOT exist yet.
// Compilation will fail until doctor.go and checks.go are implemented.
// ---------------------------------------------------------------------------

func TestReportJSONSchema(t *testing.T) {
	// Verify Report marshals to stable JSON with expected structure.
	r := &Report{
		Ok:      true,
		Summary: "all checks passed",
		Checks: []Check{
			{Name: "config", Status: "pass", Message: "valid"},
			{Name: "git", Status: "pass", Message: "clean"},
			{Name: "database", Status: "degraded", Message: "not implemented yet", Detail: "stub"},
		},
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Verify top-level fields.
	if ok, _ := parsed["ok"].(bool); !ok {
		t.Fatalf("ok field is not true: %v", parsed["ok"])
	}
	if s, ok := parsed["summary"].(string); !ok || s == "" {
		t.Fatalf("summary field missing or empty: %v", parsed["summary"])
	}

	checks, ok := parsed["checks"].([]interface{})
	if !ok {
		t.Fatalf("checks is not an array: %T", parsed["checks"])
	}
	if len(checks) != 3 {
		t.Fatalf("got %d checks, want 3", len(checks))
	}

	// Each check must have name, status, message.
	for i, c := range checks {
		cm, ok := c.(map[string]interface{})
		if !ok {
			t.Fatalf("check[%d] is not an object: %T", i, c)
		}
		if _, ok := cm["name"].(string); !ok {
			t.Fatalf("check[%d] missing name", i)
		}
		if _, ok := cm["status"].(string); !ok {
			t.Fatalf("check[%d] missing status", i)
		}
		if _, ok := cm["message"].(string); !ok {
			t.Fatalf("check[%d] missing message", i)
		}
	}

	// Verify stable JSON output: no diagnostic fields leaked.
	raw := string(data)
	if strings.Contains(raw, "logger") || strings.Contains(raw, "fixSafe") {
		t.Fatalf("JSON output contains internal runner fields: %s", raw)
	}
}

func TestRunCheckSingleFilter(t *testing.T) {
	// RunCheck("project") should return ONLY the project check.
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: test\n"), 0o644)

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	defer runner.Close()

	ctx := context.Background()

	// RunCheck should return only the named check, not all.
	check, err := runner.RunCheck(ctx, "project")
	if err != nil {
		t.Fatalf("RunCheck project: %v", err)
	}
	if check == nil {
		t.Fatal("RunCheck returned nil check")
	}
	if check.Name != "project" {
		t.Fatalf("check name=%q want project", check.Name)
	}
	if check.Status != "pass" {
		t.Fatalf("project check status=%q want pass: %s", check.Status, check.Message)
	}

	// Verify Run returns ALL checks (more than RunCheck).
	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Checks) <= 1 {
		t.Fatalf("Run returned %d checks, want more than 1 (RunCheck filtered)", len(report.Checks))
	}

	// Verify RunCheck for unknown name returns error.
	_, err = runner.RunCheck(ctx, "nonexistent_check")
	if err == nil {
		t.Fatal("RunCheck for unknown check should return error")
	}
}

func TestFixSafeCreatesMissingDir(t *testing.T) {
	root := t.TempDir()
	royoDir := filepath.Join(root, ".royo-learn")

	// The directory does NOT exist before the runner is created.
	if _, err := os.Stat(royoDir); err == nil {
		t.Fatalf("%s already exists, test setup broken", royoDir)
	}

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
		WithFixSafe(true),
	)
	defer runner.Close()

	ctx := context.Background()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify the .royo-learn directory was created by fix-safe.
	if info, err := os.Stat(royoDir); err != nil {
		t.Fatalf(".royo-learn dir was not created by fix-safe: %v", err)
	} else if !info.IsDir() {
		t.Fatal(".royo-learn is not a directory")
	}

	// Verify the filesystem check passes after fix.
	for _, c := range report.Checks {
		if c.Name == "filesystem" {
			if c.Status != "pass" {
				t.Fatalf("filesystem check status=%q want pass after fix-safe: %s", c.Status, c.Message)
			}
		}
	}
}

func TestFixSafeNoopWhenHealthy(t *testing.T) {
	root := t.TempDir()
	royoDir := filepath.Join(root, ".royo-learn")

	// Pre-create the directory.
	if err := os.MkdirAll(royoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a marker file to prove no overwrite.
	marker := filepath.Join(royoDir, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
		WithFixSafe(true),
	)
	defer runner.Close()

	ctx := context.Background()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Marker file must still exist and be unchanged.
	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker was deleted: %v", err)
	}
	if string(content) != "keep me" {
		t.Fatalf("marker content changed: %q", string(content))
	}

	if !report.Ok {
		t.Fatalf("report.Ok=false with healthy setup: summary=%s", report.Summary)
	}
}

func TestDegradedOptionalChecks(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: test\n"), 0o644)

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	defer runner.Close()

	ctx := context.Background()
	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Find all optional checks (database, migrations, engram, etc.)
	optionalSet := map[string]bool{
		"database":        true,
		"migrations":      true,
		"engram":          true,
		"gentle-ai":       true,
		"skill-registry":  true,
		"codex-mcp":       true,
		"shared-library":  true,
		"record-integrity": true,
	}

	for _, c := range report.Checks {
		if optionalSet[c.Name] {
			if c.Status != "degraded" {
				t.Fatalf("optional check %q status=%q want degraded: %s", c.Name, c.Status, c.Message)
			}
			if c.Message == "" {
				t.Fatalf("optional check %q has empty message", c.Name)
			}
		}
	}

	// Core checks (config, project, filesystem) must be in the report.
	coreFound := map[string]bool{}
	for _, c := range report.Checks {
		coreFound[c.Name] = true
	}
	for _, name := range []string{"config", "project", "filesystem"} {
		if !coreFound[name] {
			t.Fatalf("core check %q not found in report", name)
		}
	}
}

func TestRunOkWhenAllCorePass(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: test\n"), 0o644)

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	defer runner.Close()

	ctx := context.Background()
	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Core checks should all pass or degraded (not fail).
	// ok: true means all required checks passed.
	if !report.Ok {
		t.Fatalf("report.Ok=false: summary=%s", report.Summary)
	}
	if report.Summary == "" {
		t.Fatal("report.Summary is empty")
	}

	// Verify no core check has status "fail".
	for _, c := range report.Checks {
		if c.Name == "config" || c.Name == "project" {
			if c.Status == "fail" {
				t.Fatalf("core check %q failed: %s", c.Name, c.Message)
			}
		}
	}
}

func TestRunChecksAreDeterministicOrder(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: test\n"), 0o644)

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	defer runner.Close()

	ctx := context.Background()

	// Run twice — order should be the same.
	r1, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	r2, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	if len(r1.Checks) != len(r2.Checks) {
		t.Fatalf("check count differs: %d vs %d", len(r1.Checks), len(r2.Checks))
	}
	for i := range r1.Checks {
		if r1.Checks[i].Name != r2.Checks[i].Name {
			t.Fatalf("check order differs at index %d: %q vs %q", i, r1.Checks[i].Name, r2.Checks[i].Name)
		}
	}
}

func TestRunnerClose(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	err := runner.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double-close should be safe.
	err = runner.Close()
	if err != nil {
		t.Fatalf("Close (double): %v", err)
	}
}

func TestCheckStatusValidation(t *testing.T) {
	// Verify only valid status values are used.
	validStatuses := map[string]bool{
		"pass":     true,
		"fail":     true,
		"degraded": true,
		"skipped":  true,
	}

	check := Check{Name: "test", Status: "invalid", Message: "bad"}
	data, _ := json.Marshal(check)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	// The status field is a free string at the data layer — validation
	// happens at the check function level. This test documents the contract.

	// Verify valid statuses serialize correctly.
	for s := range validStatuses {
		c := Check{Name: "test", Status: s, Message: s}
		d, _ := json.Marshal(c)
		if !strings.Contains(string(d), `"status":"`+s+`"`) {
			t.Fatalf("status %q not serialized correctly: %s", s, string(d))
		}
	}
	_ = check // use the check variable
}

// ---------------------------------------------------------------------------
// TRIANGULATE — additional test cases covering edge cases and failures.
// ---------------------------------------------------------------------------

func TestConfigCheckFailsWithBadConfig(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	// Malformed YAML.
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("invalid: ["), 0o644)

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
	)
	defer runner.Close()

	ctx := context.Background()
	check, err := runner.RunCheck(ctx, "config")
	if err != nil {
		t.Fatalf("RunCheck config: %v", err)
	}
	if check.Status != StatusFail {
		t.Fatalf("config check status=%q want fail: %s", check.Status, check.Message)
	}
}

func TestProjectCheckFailsOutsideTrustedRoots(t *testing.T) {
	trusted := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(outside, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(outside, ".royo-learn", "config.yaml"), []byte("project:\n  name: outside\n"), 0o644)

	runner := NewRunner(
		WithProjectRoot(outside),
		WithTrustedRoots([]string{trusted}),
	)
	defer runner.Close()

	ctx := context.Background()
	check, err := runner.RunCheck(ctx, "project")
	if err != nil {
		t.Fatalf("RunCheck project: %v", err)
	}
	if check.Status != StatusFail {
		t.Fatalf("project check status=%q want fail: %s", check.Status, check.Message)
	}
}

func TestFilesystemCheckFailsWithoutFixSafe(t *testing.T) {
	root := t.TempDir()
	// .royo-learn does NOT exist, and fix-safe is OFF.
	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
		WithFixSafe(false),
	)
	defer runner.Close()

	ctx := context.Background()
	check, err := runner.RunCheck(ctx, "filesystem")
	if err != nil {
		t.Fatalf("RunCheck filesystem: %v", err)
	}
	if check.Status != StatusFail {
		t.Fatalf("filesystem without fix-safe status=%q want fail: %s", check.Status, check.Message)
	}

	// Verify the directory was NOT created.
	royoDir := filepath.Join(root, ".royo-learn")
	if _, err := os.Stat(royoDir); err == nil {
		t.Fatal(".royo-learn was created even with fix-safe OFF")
	}
}

func TestFixSafeOnlyCreatesRoyoLearnDir(t *testing.T) {
	// Fix-safe must NOT create arbitrary directories outside its scope.
	root := t.TempDir()

	runner := NewRunner(
		WithProjectRoot(root),
		WithTrustedRoots([]string{root}),
		WithFixSafe(true),
	)
	defer runner.Close()

	ctx := context.Background()
	_, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Only .royo-learn should have been created.
	royoDir := filepath.Join(root, ".royo-learn")
	if _, err := os.Stat(royoDir); err != nil {
		t.Fatalf(".royo-learn was not created: %v", err)
	}

	// No other directories should have been touched.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != ".royo-learn" {
		t.Fatalf("unexpected entries in root: %v", dirNames(entries))
	}
}

func TestRunWithEmptyProjectRoot(t *testing.T) {
	runner := NewRunner(
		WithTrustedRoots([]string{t.TempDir()}),
	)
	defer runner.Close()

	ctx := context.Background()
	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// With no project root, core checks should fail.
	for _, c := range report.Checks {
		if c.Name == "config" || c.Name == "project" || c.Name == "filesystem" {
			if c.Status != StatusFail {
				t.Fatalf("check %q with empty root status=%q want fail", c.Name, c.Status)
			}
		}
	}
	if report.Ok {
		t.Fatal("report.Ok=true but core checks failed")
	}
}

func TestToJSON(t *testing.T) {
	r := &Report{
		Ok:      true,
		Summary: "all 2 checks passed",
		Checks: []Check{
			{Name: "a", Status: "pass", Message: "ok"},
			{Name: "b", Status: "pass", Message: "ok"},
		},
	}

	data, err := r.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	var reparsed Report
	if err := json.Unmarshal(data, &reparsed); err != nil {
		t.Fatalf("re-parse ToJSON output: %v", err)
	}
	if reparsed.Ok != r.Ok || reparsed.Summary != r.Summary || len(reparsed.Checks) != 2 {
		t.Fatalf("round-trip mismatch: %+v", reparsed)
	}
}

func dirNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}
