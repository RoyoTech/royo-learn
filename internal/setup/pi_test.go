package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/testutil"
)

func TestPi_MCPConfigPath(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)
	got := p.MCPConfigPath()
	want := filepath.Join(home, ".pi", "agent", "mcp.json")
	if got != want {
		t.Errorf("MCPConfigPath = %q, want %q", got, want)
	}
}

func TestPi_SkillsDir(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)
	got, err := p.SkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".pi", "agent", "skills")
	if got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
}

func TestPi_RegisterMCP_NewEntry(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "/usr/local/bin/royo-learn",
		Args:    []string{"mcp-serve"},
	}
	res, err := p.RegisterMCP(entry)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if !res.Added || res.Skipped {
		t.Fatalf("unexpected result: %+v", res)
	}

	data, err := os.ReadFile(p.MCPConfigPath())
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

func TestPi_RegisterMCP_DuplicateIsNoop(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)
	entry := MCPServerEntry{Name: "royo-learn", Command: "royo-learn", Args: []string{"mcp-serve"}}

	if _, err := p.RegisterMCP(entry); err != nil {
		t.Fatalf("first RegisterMCP: %v", err)
	}
	res, err := p.RegisterMCP(entry)
	if err != nil {
		t.Fatalf("second RegisterMCP: %v", err)
	}
	if !res.Skipped || res.Added {
		t.Errorf("expected Skipped=true Added=false, got %+v", res)
	}
}

func TestPi_RegisterMCP_AppendsToExistingConfig(t *testing.T) {
	home := testutil.TempDir(t)
	cfgPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := map[string]any{
		"mcpServers": map[string]any{
			"other-server": map[string]any{"command": "other"},
		},
	}
	data, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPiWithHome(home)
	res, err := p.RegisterMCP(MCPServerEntry{
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
	servers := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Errorf("other-server lost")
	}
	if _, ok := servers["royo-learn"]; !ok {
		t.Errorf("royo-learn not added")
	}
}

func TestPi_UnregisterMCP(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)
	entry := MCPServerEntry{Name: "royo-learn", Command: "royo-learn"}
	if _, err := p.RegisterMCP(entry); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if err := p.UnregisterMCP("royo-learn"); err != nil {
		t.Fatalf("UnregisterMCP: %v", err)
	}
	ok, err := p.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatalf("VerifyMCP: %v", err)
	}
	if ok {
		t.Errorf("entry still present after unregister")
	}
}

func TestPi_VerifyMCP_TrueAndFalse(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)

	ok, err := p.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatalf("VerifyMCP (empty): %v", err)
	}
	if ok {
		t.Errorf("expected false on empty config")
	}

	if _, err := p.RegisterMCP(MCPServerEntry{Name: "royo-learn", Command: "x"}); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	ok, _ = p.VerifyMCP("royo-learn")
	if !ok {
		t.Errorf("expected true after register")
	}
}

func TestPi_BackupMCPConfig_EmptyWhenAbsent(t *testing.T) {
	home := testutil.TempDir(t)
	p := NewPiWithHome(home)
	backup, err := p.BackupMCPConfig()
	if err != nil {
		t.Fatalf("BackupMCPConfig: %v", err)
	}
	if backup != "" {
		t.Errorf("expected empty backup when no config exists, got %q", backup)
	}
}
