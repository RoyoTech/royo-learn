package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/setup"
)

func TestSetupStatus_JSON_AllAgentsListed(t *testing.T) {
	// Use isolated home so we don't read the user's real configs.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	var out, errOut bytes.Buffer
	code := run([]string{"setup", "status", "--json"}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("status exit code = %d, stderr=%s", code, errOut.String())
	}

	var parsed struct {
		Command string                   `json:"command"`
		Agents  []map[string]interface{} `json:"agents"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if parsed.Command != "setup status" {
		t.Errorf("command = %q", parsed.Command)
	}
	if len(parsed.Agents) != len(setup.AllAgents) {
		t.Errorf("expected %d agents, got %d", len(setup.AllAgents), len(parsed.Agents))
	}
	for _, e := range parsed.Agents {
		kind, _ := e["agent"].(string)
		switch setup.AgentKind(kind) {
		case setup.AgentClaudeCode, setup.AgentCodex, setup.AgentOpenCode:
		default:
			t.Errorf("unexpected agent kind in status: %q", kind)
		}
	}
}

func TestSetupStatus_HumanOutput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	var out, errOut bytes.Buffer
	code := run([]string{"setup", "status"}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("status exit code = %d, stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"Claude Code", "Codex CLI", "OpenCode"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\n%s", want, out.String())
		}
	}
}

func TestSetupInstall_DryRun_JSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	projectRoot := t.TempDir()
	skillsDir := filepath.Join(projectRoot, "skills", "demo-skill")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a minimal binary path to satisfy resolveRoyoLearnBinary
	// without invoking the actual toolchain.
	fakeBin := filepath.Join(t.TempDir(), "royo-learn.exe")
	if err := os.WriteFile(fakeBin, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "install",
		"--agent", "all",
		"--binary", fakeBin,
		"--project-root", projectRoot,
		"--dry-run",
		"--json",
	}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("install dry-run exit code = %d, stderr=%s", code, errOut.String())
	}

	var parsed struct {
		Results []setupResult `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if len(parsed.Results) != len(setup.AllAgents) {
		t.Fatalf("expected %d results, got %d", len(setup.AllAgents), len(parsed.Results))
	}
	for _, r := range parsed.Results {
		// Dry-run must never mutate, never record a backup, never set Error.
		if r.Error != "" {
			t.Errorf("[%s] dry-run should not error: %s", r.Agent, r.Error)
		}
	}
}

func TestSetupInstall_DryRun_ClaudeCodeWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	projectRoot := t.TempDir()
	skillsDir := filepath.Join(projectRoot, "skills", "demo")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(t.TempDir(), "royo-learn.exe")
	_ = os.WriteFile(fakeBin, []byte(""), 0o644)

	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "install",
		"--agent", "claude-code",
		"--binary", fakeBin,
		"--project-root", projectRoot,
		"--dry-run",
	}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("dry-run install: code=%d stderr=%s", code, errOut.String())
	}

	// Claude Code config must NOT exist after dry-run.
	cfgPath := filepath.Join(tmp, ".claude", "mcp.json")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote %s; err = %v", cfgPath, err)
	}
}

func TestSetupInstall_Real_ClaudeCodeWritesJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	projectRoot := t.TempDir()
	skillsDir := filepath.Join(projectRoot, "skills", "demo")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(t.TempDir(), "royo-learn.exe")
	_ = os.WriteFile(fakeBin, []byte(""), 0o644)

	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "install",
		"--agent", "claude-code",
		"--binary", fakeBin,
		"--project-root", projectRoot,
		"--skip-skills",
		"--json",
	}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("install: code=%d stderr=%s out=%s", code, errOut.String(), out.String())
	}

	cfgPath := filepath.Join(tmp, ".claude", "mcp.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected config at %s: %v", cfgPath, err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config is not JSON: %v\n%s", err, data)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	entry, ok := servers[royoLearnMCPServerName].(map[string]any)
	if !ok {
		t.Fatalf("entry missing in %s:\n%s", royoLearnMCPServerName, data)
	}
	if got := entry["command"]; got != fakeBin {
		t.Errorf("command = %v, want %s", got, fakeBin)
	}
}

func TestSetupUninstall_RemovesEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// Pre-register via Claude Code agent.
	c := setup.NewClaudeCodeWithHome(tmp)
	if _, err := c.RegisterMCP(setup.MCPServerEntry{
		Name: royoLearnMCPServerName, Command: "royo-learn", Args: []string{"mcp-serve"},
	}); err != nil {
		t.Fatalf("seed RegisterMCP: %v", err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "uninstall",
		"--agent", "claude-code",
		"--json",
	}, &out, &errOut)
	if code != exitSuccess {
		t.Fatalf("uninstall: code=%d stderr=%s", code, errOut.String())
	}
	ok, err := c.VerifyMCP(royoLearnMCPServerName)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("entry still present after uninstall")
	}
}

func TestSetupInstall_UnknownAgentFails(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"setup", "install", "--agent", "bogus-agent", "--dry-run"}, &out, &errOut)
	if code == exitSuccess {
		t.Errorf("expected failure for unknown agent")
	}
	if !strings.Contains(errOut.String(), "unknown agent") {
		t.Errorf("stderr should mention 'unknown agent'; got %q", errOut.String())
	}
}

func TestSetupInstall_RequiresBinary(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{
		"setup", "install",
		"--agent", "claude-code",
		"--binary", filepath.Join(t.TempDir(), "definitely-missing-binary"),
		"--dry-run",
	}, &out, &errOut)
	if code == exitSuccess {
		t.Errorf("expected failure for missing binary")
	}
	if !strings.Contains(errOut.String(), "binary not found") {
		t.Errorf("stderr should mention missing binary; got %q", errOut.String())
	}
}
