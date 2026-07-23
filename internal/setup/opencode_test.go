package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/testutil"
)

func TestOpenCode_RegisterMCP_StoresArrayCommand(t *testing.T) {
	dir := testutil.TempDir(t)
	cfgPath := filepath.Join(dir, "opencode.json")
	o := NewOpenCodeWithConfig(cfgPath)

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "royo-learn",
		Args:    []string{"mcp-serve", "--project-root", "."},
	}
	res, err := o.RegisterMCP(entry)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if !res.Added {
		t.Fatalf("expected Added=true, got %+v", res)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	mcp, _ := cfg["mcp"].(map[string]any)
	stored, ok := mcp["royo-learn"].(map[string]any)
	if !ok {
		t.Fatalf("entry missing: mcp=%v", mcp)
	}
	// OpenCode requires array command, not string.
	cmd, ok := stored["command"].([]any)
	if !ok {
		t.Fatalf("command is not array: %T %v", stored["command"], stored["command"])
	}
	want := []string{"royo-learn", "mcp-serve", "--project-root", "."}
	if len(cmd) != len(want) {
		t.Fatalf("command len = %d, want %d", len(cmd), len(want))
	}
	for i, v := range want {
		if cmd[i] != v {
			t.Errorf("cmd[%d] = %v, want %v", i, cmd[i], v)
		}
	}
	if stored["type"] != "local" {
		t.Errorf("type = %v", stored["type"])
	}
	if stored["enabled"] != true {
		t.Errorf("enabled = %v", stored["enabled"])
	}
}

func TestOpenCode_RegisterMCP_DuplicateIsNoop(t *testing.T) {
	dir := testutil.TempDir(t)
	o := NewOpenCodeWithConfig(filepath.Join(dir, "opencode.json"))
	entry := MCPServerEntry{Name: "royo-learn", Command: "royo-learn"}

	if _, err := o.RegisterMCP(entry); err != nil {
		t.Fatal(err)
	}
	res, err := o.RegisterMCP(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped=true, got %+v", res)
	}
}

func TestOpenCode_RegisterMCP_PreservesExistingConfig(t *testing.T) {
	dir := testutil.TempDir(t)
	cfgPath := filepath.Join(dir, "opencode.json")
	original := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"share":   "disabled",
		"mcp": map[string]any{
			"context7": map[string]any{
				"type":    "remote",
				"url":     "https://mcp.context7.com/mcp",
				"enabled": true,
			},
		},
	}
	data, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOpenCodeWithConfig(cfgPath)
	if _, err := o.RegisterMCP(MCPServerEntry{
		Name: "royo-learn", Command: "royo-learn", Args: []string{"mcp-serve"},
	}); err != nil {
		t.Fatal(err)
	}

	final, _ := os.ReadFile(cfgPath)
	var cfg map[string]any
	_ = json.Unmarshal(final, &cfg)
	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("schema lost: %v", cfg["$schema"])
	}
	if cfg["share"] != "disabled" {
		t.Errorf("share lost: %v", cfg["share"])
	}
	mcp := cfg["mcp"].(map[string]any)
	if _, ok := mcp["context7"]; !ok {
		t.Errorf("context7 lost")
	}
	if _, ok := mcp["royo-learn"]; !ok {
		t.Errorf("royo-learn not added")
	}
}

func TestOpenCode_VerifyMCP(t *testing.T) {
	dir := testutil.TempDir(t)
	o := NewOpenCodeWithConfig(filepath.Join(dir, "opencode.json"))

	ok, err := o.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("expected false on empty config")
	}

	if _, err := o.RegisterMCP(MCPServerEntry{Name: "royo-learn", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	ok, _ = o.VerifyMCP("royo-learn")
	if !ok {
		t.Errorf("expected true after register")
	}
}

func TestOpenCode_UnregisterMCP(t *testing.T) {
	dir := testutil.TempDir(t)
	o := NewOpenCodeWithConfig(filepath.Join(dir, "opencode.json"))
	if _, err := o.RegisterMCP(MCPServerEntry{Name: "royo-learn", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := o.UnregisterMCP("royo-learn"); err != nil {
		t.Fatal(err)
	}
	ok, _ := o.VerifyMCP("royo-learn")
	if ok {
		t.Errorf("entry still present after unregister")
	}
}

func TestOpenCode_SkillsDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/fake-home-opencode")
	t.Setenv("USERPROFILE", "")
	o := NewOpenCode()
	got, err := o.SkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/fake-home-opencode", ".config", "opencode", "skills")
	if got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
}

func TestOpenCode_BackupMCPConfig_Absent(t *testing.T) {
	dir := testutil.TempDir(t)
	o := NewOpenCodeWithConfig(filepath.Join(dir, "nope.json"))
	backup, err := o.BackupMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Errorf("expected empty backup when no config exists, got %q", backup)
	}
}
