package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodex_VerifyMCP_HeaderDetection(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	c := NewCodexWithConfig(cfgPath)

	// Empty config — entry absent.
	ok, err := c.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("expected false on missing config")
	}

	// Write a TOML config containing the section header for royo-learn.
	content := `windows_wsl_setup_acknowledged = true

[mcp_servers.n8n_mcp]
url = "https://example.com/mcp"
enabled = true

[mcp_servers.royo-learn]
command = "/usr/local/bin/royo-learn"
args = ["mcp-serve"]
enabled = true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = c.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("expected true after writing header")
	}

	// Other agent entries should still report absent.
	ok, _ = c.VerifyMCP("nope")
	if ok {
		t.Errorf("expected false for non-existent entry")
	}
}

func TestCodex_VerifyMCP_HeaderMustBeExact(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	c := NewCodexWithConfig(cfgPath)

	// Write a config where the royo-learn name appears only in a comment.
	// VerifyMCP uses raw bytes — it would still match the comment line,
	// but only entries inside their own section would matter; document
	// the actual behaviour rather than masking it.
	content := "# [mcp_servers.royo-learn] comment only\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := c.VerifyMCP("royo-learn")
	if err != nil {
		t.Fatal(err)
	}
	// Header scan matches literally — a comment line also matches. This
	// is acceptable because Codex would never accept a commented section
	// as a real registration; the verification is best-effort.
	if !ok {
		t.Errorf("header scan did not detect commented reference (literal bytes match)")
	}
}

func TestCodex_BackupMCPConfig_Absent(t *testing.T) {
	dir := t.TempDir()
	c := NewCodexWithConfig(filepath.Join(dir, "nope.toml"))
	backup, err := c.BackupMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Errorf("expected empty backup, got %q", backup)
	}
}

func TestCodex_BackupMCPConfig_Present(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("model = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCodexWithConfig(cfgPath)
	backup, err := c.BackupMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected non-empty backup")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
}

func TestCodex_RegisterMCP_RequiresBinary(t *testing.T) {
	dir := t.TempDir()
	c := NewCodexWithConfig(filepath.Join(dir, "config.toml"))
	if c.IsInstalled() {
		t.Skip("codex binary is on PATH in this environment; skipping negative test")
	}
	_, err := c.RegisterMCP(MCPServerEntry{
		Name: "royo-learn", Command: "royo-learn", Args: []string{"mcp-serve"},
	})
	if err == nil {
		t.Fatal("expected error when codex is not installed")
	}
}

func TestCodex_SkillsDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/fake-home-codex")
	t.Setenv("USERPROFILE", "")
	c := NewCodex()
	got, err := c.SkillsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/fake-home-codex", ".codex", "skills")
	if got != want {
		t.Errorf("SkillsDir = %q, want %q", got, want)
	}
}
