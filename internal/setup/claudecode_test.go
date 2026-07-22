package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/testutil"
)

func TestClaudeCode_RegisterMCP_NewEntry(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "/usr/local/bin/royo-learn",
		Args:    []string{"mcp-serve"},
	}
	res, err := c.RegisterMCP(entry)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if !res.Added || res.Skipped {
		t.Fatalf("unexpected result: %+v", res)
	}

	cfgPath := c.MCPConfigPath()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	entry2, ok := servers["royo-learn"].(map[string]any)
	if !ok {
		t.Fatalf("entry missing: servers=%v", servers)
	}
	if entry2["command"] != "/usr/local/bin/royo-learn" {
		t.Errorf("command = %v", entry2["command"])
	}
	args, _ := entry2["args"].([]any)
	if len(args) != 1 || args[0] != "mcp-serve" {
		t.Errorf("args = %v", args)
	}
}

func TestClaudeCode_RegisterMCP_DuplicateIsNoop(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)
	entry := MCPServerEntry{Name: "royo-learn", Command: "royo-learn", Args: []string{"mcp-serve"}}

	if _, err := c.RegisterMCP(entry); err != nil {
		t.Fatalf("first RegisterMCP: %v", err)
	}
	res, err := c.RegisterMCP(entry)
	if err != nil {
		t.Fatalf("second RegisterMCP: %v", err)
	}
	if !res.Skipped || res.Added {
		t.Errorf("expected Skipped=true Added=false, got %+v", res)
	}
}

func TestClaudeCode_RegisterMCP_AppendsToExistingConfig(t *testing.T) {
	home := testutil.TempDir(t)
	cfgPath := filepath.Join(home, ".claude", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := map[string]any{
		"$schema": "https://example.com/schema.json",
		"mcpServers": map[string]any{
			"other-server": map[string]any{"command": "other"},
		},
	}
	data, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewClaudeCodeWithHome(home)
	res, err := c.RegisterMCP(MCPServerEntry{
		Name: "royo-learn", Command: "royo-learn", Args: []string{"mcp-serve"},
	})
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if !res.Added {
		t.Fatalf("expected Added=true, got %+v", res)
	}

	final, _ := os.ReadFile(cfgPath)
	var cfg map[string]any
	_ = json.Unmarshal(final, &cfg)
	if cfg["$schema"] != "https://example.com/schema.json" {
		t.Errorf("schema lost: %v", cfg["$schema"])
	}
	servers := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Errorf("other-server lost")
	}
	if _, ok := servers["royo-learn"]; !ok {
		t.Errorf("royo-learn not added")
	}
}

func TestClaudeCode_UnregisterMCP(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)
	entry := MCPServerEntry{Name: "royo-learn", Command: "royo-learn"}
	if _, err := c.RegisterMCP(entry); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if err := c.UnregisterMCP("royo-learn"); err != nil {
		t.Fatalf("UnregisterMCP: %v", err)
	}
	ok, err := c.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatalf("VerifyMCP: %v", err)
	}
	if ok {
		t.Errorf("entry still present after unregister")
	}
}

func TestClaudeCode_VerifyMCP_TrueAndFalse(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)

	ok, err := c.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatalf("VerifyMCP (empty): %v", err)
	}
	if ok {
		t.Errorf("expected false on empty config")
	}

	if _, err := c.RegisterMCP(MCPServerEntry{Name: "royo-learn", Command: "x"}); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	ok, _ = c.VerifyMCP("royo-learn")
	if !ok {
		t.Errorf("expected true after register")
	}
}

func TestClaudeCode_BackupMCPConfig_EmptyWhenAbsent(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)
	backup, err := c.BackupMCPConfig()
	if err != nil {
		t.Fatalf("BackupMCPConfig: %v", err)
	}
	if backup != "" {
		t.Errorf("expected empty backup when no config exists, got %q", backup)
	}
}

func TestClaudeCode_BackupMCPConfig_CreatesTimestamped(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)
	if _, err := c.RegisterMCP(MCPServerEntry{Name: "x", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	backup, err := c.BackupMCPConfig()
	if err != nil {
		t.Fatalf("BackupMCPConfig: %v", err)
	}
	if backup == "" {
		t.Fatal("expected non-empty backup path")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
}

func TestClaudeCode_SkillsDir(t *testing.T) {
	home := testutil.TempDir(t)
	c := NewClaudeCodeWithHome(home)
	got, err := c.SkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".claude", "skills")
	if got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
}
