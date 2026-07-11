package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeCode implements Agent for Anthropic's Claude Code CLI.
//
// Configuration layout:
//   - MCP servers:  ~/.claude/mcp.json  (JSON, "mcpServers" key)
//   - Skills:       ~/.claude/skills/<skill>/SKILL.md
type ClaudeCode struct {
	homeDir string
}

// NewClaudeCode returns a Claude Code agent bound to the given home directory.
// An empty homeDir falls back to HomeDir().
func NewClaudeCode() *ClaudeCode {
	return &ClaudeCode{homeDir: HomeDir()}
}

// NewClaudeCodeWithHome is for tests: bind to an explicit home directory.
func NewClaudeCodeWithHome(home string) *ClaudeCode {
	return &ClaudeCode{homeDir: home}
}

func (c *ClaudeCode) Kind() AgentKind     { return AgentClaudeCode }
func (c *ClaudeCode) DisplayName() string { return "Claude Code" }
func (c *ClaudeCode) IsInstalled() bool   { return binaryOnPath("claude") }

func (c *ClaudeCode) MCPConfigPath() string {
	if c.homeDir == "" {
		return ""
	}
	return filepath.Join(c.homeDir, ".claude", "mcp.json")
}

func (c *ClaudeCode) SkillsDir() (string, error) {
	if c.homeDir == "" {
		return "", fmt.Errorf("setup: cannot resolve Claude Code skills dir: HOME/USERPROFILE not set")
	}
	return filepath.Join(c.homeDir, ".claude", "skills"), nil
}

func (c *ClaudeCode) BackupMCPConfig() (string, error) {
	path := c.MCPConfigPath()
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		// No config yet — nothing to back up.
		return "", nil
	}
	return BackupConfig(path)
}

// RegisterMCP adds the MCP server entry to ~/.claude/mcp.json under
// the "mcpServers" key. If the file does not exist it is created.
// If the entry already exists (same name) the call is a no-op.
func (c *ClaudeCode) RegisterMCP(entry MCPServerEntry) (*MCPRegisterResult, error) {
	path := c.MCPConfigPath()
	if path == "" {
		return nil, fmt.Errorf("setup: cannot resolve Claude Code MCP config path")
	}

	cfg, err := loadOrInitJSONConfig(path)
	if err != nil {
		return nil, err
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		cfg["mcpServers"] = servers
	}

	if _, exists := servers[entry.Name]; exists {
		return &MCPRegisterResult{
			Skipped: true,
			Reason:  fmt.Sprintf("MCP server %q already registered", entry.Name),
		}, nil
	}

	serverEntry := map[string]any{
		"command": entry.Command,
	}
	if len(entry.Args) > 0 {
		args := make([]any, len(entry.Args))
		for i, a := range entry.Args {
			args[i] = a
		}
		serverEntry["args"] = args
	}
	if len(entry.Env) > 0 {
		env := make(map[string]any, len(entry.Env))
		for k, v := range entry.Env {
			env[k] = v
		}
		serverEntry["env"] = env
	}
	servers[entry.Name] = serverEntry

	if err := writeJSONConfig(path, cfg); err != nil {
		return nil, err
	}

	return &MCPRegisterResult{
		Added:  true,
		Reason: fmt.Sprintf("registered MCP server %q", entry.Name),
	}, nil
}

// UnregisterMCP removes the named MCP server entry if present.
func (c *ClaudeCode) UnregisterMCP(name string) error {
	path := c.MCPConfigPath()
	if path == "" {
		return fmt.Errorf("setup: cannot resolve Claude Code MCP config path")
	}
	cfg, err := loadOrInitJSONConfig(path)
	if err != nil {
		return err
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		return nil
	}
	if _, ok := servers[name]; !ok {
		return nil
	}
	delete(servers, name)
	return writeJSONConfig(path, cfg)
}

// VerifyMCP reports whether the named MCP entry exists.
func (c *ClaudeCode) VerifyMCP(name string) (bool, error) {
	path := c.MCPConfigPath()
	if path == "" {
		return false, nil
	}
	cfg, err := loadOrInitJSONConfig(path)
	if err != nil {
		return false, err
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		return false, nil
	}
	_, ok := servers[name]
	return ok, nil
}

// loadOrInitJSONConfig reads path as JSON, returning an empty map if the
// file does not exist. It refuses to overwrite non-JSON content.
func loadOrInitJSONConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("setup: cannot read config %q: %w", path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("setup: cannot parse config %q: %w", path, err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// writeJSONConfig writes cfg as pretty JSON to path atomically.
func writeJSONConfig(path string, cfg map[string]any) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("setup: cannot marshal config: %w", err)
	}
	out = append(out, '\n')
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("setup: cannot create config dir: %w", err)
		}
	}
	return writeFileAtomic(path, out, 0o644)
}
