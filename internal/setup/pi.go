package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// Pi implements Agent for the Pi coding agent (pi).
//
// Configuration layout:
//   - MCP servers:  ~/.pi/agent/mcp.json  (JSON, "mcpServers" key)
//   - Skills:       ~/.pi/agent/skills/<skill>/SKILL.md
//
// Pi uses the same "mcpServers" JSON shape as Claude Code, so the
// registration logic is identical; only the paths differ.
type Pi struct {
	homeDir string
}

// NewPi returns a Pi agent bound to the platform-default home directory.
// An empty homeDir falls back to HomeDir().
func NewPi() *Pi {
	return &Pi{homeDir: HomeDir()}
}

// NewPiWithHome is for tests: bind to an explicit home directory.
func NewPiWithHome(home string) *Pi {
	return &Pi{homeDir: home}
}

func (p *Pi) Kind() AgentKind     { return AgentPi }
func (p *Pi) DisplayName() string { return "Pi" }
func (p *Pi) IsInstalled() bool   { return binaryOnPath("pi") }

func (p *Pi) MCPConfigPath() string {
	if p.homeDir == "" {
		return ""
	}
	return filepath.Join(p.homeDir, ".pi", "agent", "mcp.json")
}

func (p *Pi) SkillsDir() (string, error) {
	if p.homeDir == "" {
		return "", fmt.Errorf("setup: cannot resolve Pi skills dir: HOME/USERPROFILE not set")
	}
	return filepath.Join(p.homeDir, ".pi", "agent", "skills"), nil
}

func (p *Pi) BackupMCPConfig() (string, error) {
	path := p.MCPConfigPath()
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		// No config yet — nothing to back up.
		return "", nil
	}
	return BackupConfig(path)
}

// RegisterMCP adds the MCP server entry to ~/.pi/agent/mcp.json under
// the "mcpServers" key. If the file does not exist it is created.
// If the entry already exists (same name) the call is a no-op.
func (p *Pi) RegisterMCP(entry MCPServerEntry) (*MCPRegisterResult, error) {
	path := p.MCPConfigPath()
	if path == "" {
		return nil, fmt.Errorf("setup: cannot resolve Pi MCP config path")
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
func (p *Pi) UnregisterMCP(name string) error {
	path := p.MCPConfigPath()
	if path == "" {
		return fmt.Errorf("setup: cannot resolve Pi MCP config path")
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
func (p *Pi) VerifyMCP(name string) (bool, error) {
	path := p.MCPConfigPath()
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
