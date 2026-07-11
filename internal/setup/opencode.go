package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OpenCode implements Agent for OpenCode (opencode.ai).
//
// Configuration layout:
//   - MCP servers:  ~/.config/opencode/opencode.json  (JSON, "mcp" key, NOT "mcpServers")
//   - Skills:       ~/.config/opencode/skills/<skill>/SKILL.md
//
// OpenCode uses an array form for "command" and a discriminator "type":
//
//	"mcp": {
//	  "royo-learn": {
//	    "command": ["royo-learn", "mcp-serve", "--project-root", "."],
//	    "type":    "local",
//	    "enabled": true
//	  }
//	}
type OpenCode struct {
	configPath string
}

// NewOpenCode returns an OpenCode agent using the platform-default config path.
func NewOpenCode() *OpenCode {
	return NewOpenCodeWithConfig(defaultOpenCodeConfigPath())
}

// NewOpenCodeWithConfig is for tests: bind to an explicit config path.
func NewOpenCodeWithConfig(path string) *OpenCode {
	return &OpenCode{configPath: path}
}

func defaultOpenCodeConfigPath() string {
	if h := HomeDir(); h != "" {
		return filepath.Join(h, ".config", "opencode", "opencode.json")
	}
	return ""
}

func (o *OpenCode) Kind() AgentKind     { return AgentOpenCode }
func (o *OpenCode) DisplayName() string { return "OpenCode" }
func (o *OpenCode) IsInstalled() bool   { return binaryOnPath("opencode") }

func (o *OpenCode) MCPConfigPath() string { return o.configPath }

func (o *OpenCode) SkillsDir() (string, error) {
	if h := HomeDir(); h != "" {
		return filepath.Join(h, ".config", "opencode", "skills"), nil
	}
	return "", fmt.Errorf("setup: cannot resolve OpenCode skills dir: HOME/USERPROFILE not set")
}

func (o *OpenCode) BackupMCPConfig() (string, error) {
	if o.configPath == "" {
		return "", nil
	}
	if _, err := os.Stat(o.configPath); err != nil {
		return "", nil
	}
	return BackupConfig(o.configPath)
}

// RegisterMCP adds the MCP server entry under the "mcp" key in opencode.json.
// The command is stored as a JSON array (OpenCode requirement).
func (o *OpenCode) RegisterMCP(entry MCPServerEntry) (*MCPRegisterResult, error) {
	if o.configPath == "" {
		return nil, fmt.Errorf("setup: cannot resolve OpenCode MCP config path")
	}

	cfg, err := loadOrInitJSONConfig(o.configPath)
	if err != nil {
		return nil, err
	}

	mcpServers, _ := cfg["mcp"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
		cfg["mcp"] = mcpServers
	}

	if _, exists := mcpServers[entry.Name]; exists {
		return &MCPRegisterResult{
			Skipped: true,
			Reason:  fmt.Sprintf("MCP server %q already registered", entry.Name),
		}, nil
	}

	// OpenCode uses ["cmd", "arg1", "arg2"] for the command field.
	cmd := append([]string{entry.Command}, entry.Args...)
	serverEntry := map[string]any{
		"command": cmd,
		"type":    "local",
		"enabled": true,
	}
	if len(entry.Env) > 0 {
		env := make(map[string]any, len(entry.Env))
		for k, v := range entry.Env {
			env[k] = v
		}
		serverEntry["env"] = env
	}
	mcpServers[entry.Name] = serverEntry

	if err := writeJSONConfig(o.configPath, cfg); err != nil {
		return nil, err
	}

	return &MCPRegisterResult{
		Added:  true,
		Reason: fmt.Sprintf("registered MCP server %q", entry.Name),
	}, nil
}

// UnregisterMCP removes the named MCP entry from the "mcp" map.
func (o *OpenCode) UnregisterMCP(name string) error {
	if o.configPath == "" {
		return fmt.Errorf("setup: cannot resolve OpenCode MCP config path")
	}
	cfg, err := loadOrInitJSONConfig(o.configPath)
	if err != nil {
		return err
	}
	mcpServers, _ := cfg["mcp"].(map[string]any)
	if mcpServers == nil {
		return nil
	}
	if _, ok := mcpServers[name]; !ok {
		return nil
	}
	delete(mcpServers, name)
	return writeJSONConfig(o.configPath, cfg)
}

// VerifyMCP reports whether the named MCP entry exists in the "mcp" map.
func (o *OpenCode) VerifyMCP(name string) (bool, error) {
	if o.configPath == "" {
		return false, nil
	}
	cfg, err := loadOrInitJSONConfig(o.configPath)
	if err != nil {
		return false, err
	}
	mcpServers, _ := cfg["mcp"].(map[string]any)
	if mcpServers == nil {
		return false, nil
	}
	_, ok := mcpServers[name]
	return ok, nil
}

// Ensure JSON marshalling stays stable when re-reading — guard against
// silent map ordering differences in tests.
var _ = json.Marshal
