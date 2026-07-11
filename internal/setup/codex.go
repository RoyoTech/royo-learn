package setup

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Codex implements Agent for OpenAI's Codex CLI.
//
// Configuration layout:
//   - MCP servers:  ~/.codex/config.toml  (TOML, "mcp_servers" key)
//   - Skills:       ~/.codex/skills/<skill>/SKILL.md
//
// Codex shares the same MCP configuration between CLI, desktop app and
// IDE on the same host. Per docs/08 the installer MUST prefer
// `codex mcp add` over manual TOML editing because the CLI guarantees
// correct escaping and idempotency.
type Codex struct {
	configPath string
}

// NewCodex returns a Codex agent bound to the platform-default config path.
func NewCodex() *Codex {
	return NewCodexWithConfig(defaultCodexConfigPath())
}

// NewCodexWithConfig is for tests: bind to an explicit config path.
func NewCodexWithConfig(path string) *Codex {
	return &Codex{configPath: path}
}

func defaultCodexConfigPath() string {
	if h := HomeDir(); h != "" {
		return filepath.Join(h, ".codex", "config.toml")
	}
	return ""
}

func (c *Codex) Kind() AgentKind     { return AgentCodex }
func (c *Codex) DisplayName() string { return "Codex CLI" }
func (c *Codex) IsInstalled() bool   { return binaryOnPath("codex") }

func (c *Codex) MCPConfigPath() string { return c.configPath }

func (c *Codex) SkillsDir() (string, error) {
	if h := HomeDir(); h != "" {
		return filepath.Join(h, ".codex", "skills"), nil
	}
	return "", fmt.Errorf("setup: cannot resolve Codex skills dir: HOME/USERPROFILE not set")
}

func (c *Codex) BackupMCPConfig() (string, error) {
	if c.configPath == "" {
		return "", nil
	}
	if _, err := os.Stat(c.configPath); err != nil {
		return "", nil
	}
	return BackupConfig(c.configPath)
}

// RegisterMCP shells out to `codex mcp add` to register the entry.
// It is idempotent: re-running with the same name is a no-op.
func (c *Codex) RegisterMCP(entry MCPServerEntry) (*MCPRegisterResult, error) {
	if !c.IsInstalled() {
		return nil, fmt.Errorf("setup: codex CLI not found on PATH; install Codex before registering")
	}

	// Skip duplicate detection via direct config inspection when possible
	// to avoid spawning a `codex mcp add` that fails with "already exists".
	exists, err := c.VerifyMCP(entry.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return &MCPRegisterResult{
			Skipped: true,
			Reason:  fmt.Sprintf("MCP server %q already registered", entry.Name),
		}, nil
	}

	args := []string{"mcp", "add", entry.Name, "--", entry.Command}
	args = append(args, entry.Args...)
	cmd := exec.Command("codex", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stdout.String() + " " + stderr.String())
		// Treat "already exists" as a successful skip — covers race conditions
		// where another invocation added the same entry concurrently.
		if strings.Contains(strings.ToLower(out), "already") {
			return &MCPRegisterResult{
				Skipped: true,
				Reason:  fmt.Sprintf("MCP server %q already registered", entry.Name),
			}, nil
		}
		return nil, fmt.Errorf("setup: codex mcp add failed: %w (output: %s)", err, out)
	}

	return &MCPRegisterResult{
		Added:  true,
		Reason: fmt.Sprintf("registered MCP server %q", entry.Name),
	}, nil
}

// UnregisterMCP shells out to `codex mcp remove`.
func (c *Codex) UnregisterMCP(name string) error {
	if !c.IsInstalled() {
		return fmt.Errorf("setup: codex CLI not found on PATH")
	}
	cmd := exec.Command("codex", "mcp", "remove", name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stdout.String() + " " + stderr.String())
		if strings.Contains(strings.ToLower(out), "not found") ||
			strings.Contains(strings.ToLower(out), "does not exist") {
			return nil
		}
		return fmt.Errorf("setup: codex mcp remove failed: %w (output: %s)", err, out)
	}
	return nil
}

// VerifyMCP scans ~/.codex/config.toml for the [mcp_servers.<name>] header.
// This avoids the cost of spawning `codex mcp list` for the common case.
func (c *Codex) VerifyMCP(name string) (bool, error) {
	if c.configPath == "" {
		return false, nil
	}
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("setup: cannot read %q: %w", c.configPath, err)
	}
	header := "[mcp_servers." + name + "]"
	return bytes.Contains(data, []byte(header)), nil
}
