package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/testutil"
)

func TestRegisterMCPServer_NewEntry(t *testing.T) {
	dir := testutil.TempDir(t)
	cfgPath := filepath.Join(dir, "opencode.json")

	// Start with a config that has no MCP servers.
	original := map[string]any{
		"mcpServers": map[string]any{},
	}
	writeJSON(t, cfgPath, original)

	// Backup the original.
	_, err := BackupConfig(cfgPath)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "royo-learn",
		Args:    []string{"mcp-serve", "--project-root", "."},
	}

	result, err := RegisterMCPServer(cfgPath, entry)
	if err != nil {
		t.Fatalf("RegisterMCPServer: %v", err)
	}
	if !result.Added {
		t.Fatal("expected Added=true")
	}

	// Read back and verify.
	cfg := readJSON(t, cfgPath)
	mcpServers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers not found in config")
	}
	royo, ok := mcpServers["royo-learn"]
	if !ok {
		t.Fatal("royo-learn entry not found in mcpServers")
	}
	royoMap, ok := royo.(map[string]any)
	if !ok {
		t.Fatal("royo-learn entry is not a map")
	}
	if royoMap["command"] != "royo-learn" {
		t.Fatalf("command = %q, want royo-learn", royoMap["command"])
	}
}

func TestRegisterMCPServer_DuplicateSkip(t *testing.T) {
	dir := testutil.TempDir(t)
	cfgPath := filepath.Join(dir, "opencode.json")

	// Config already has royo-learn.
	original := map[string]any{
		"mcpServers": map[string]any{
			"royo-learn": map[string]any{
				"command": "royo-learn",
				"args":    []any{"mcp-serve"},
			},
		},
	}
	writeJSON(t, cfgPath, original)

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "royo-learn",
		Args:    []string{"mcp-serve"},
	}

	result, err := RegisterMCPServer(cfgPath, entry)
	if err != nil {
		t.Fatalf("RegisterMCPServer: %v", err)
	}
	if result.Added {
		t.Fatal("expected Added=false for duplicate")
	}
	if !result.Skipped {
		t.Fatal("expected Skipped=true for duplicate")
	}
}

func TestRegisterMCPServer_MissingConfig(t *testing.T) {
	dir := testutil.TempDir(t)
	cfgPath := filepath.Join(dir, "nonexistent.json")

	entry := MCPServerEntry{
		Name:    "royo-learn",
		Command: "royo-learn",
		Args:    []string{"mcp-serve"},
	}

	_, err := RegisterMCPServer(cfgPath, entry)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return v
}
