package setup

import (
	"encoding/json"
	"fmt"
	"os"
)

// MCPServerEntry describes an MCP server to register in the agent config.
type MCPServerEntry struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// MCPRegisterResult reports the outcome of MCP server registration.
type MCPRegisterResult struct {
	Added   bool
	Skipped bool
	Reason  string
}

// RegisterMCPServer adds an MCP server entry to the agent config file.
// It checks for duplicates before adding and returns whether the entry
// was added or skipped.
func RegisterMCPServer(configPath string, entry MCPServerEntry) (*MCPRegisterResult, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("setup: cannot read config %q: %w", configPath, err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("setup: cannot parse config %q: %w", configPath, err)
	}

	// Ensure mcpServers map exists.
	mcpServers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
		cfg["mcpServers"] = mcpServers
	}

	// Check duplicate.
	if _, exists := mcpServers[entry.Name]; exists {
		return &MCPRegisterResult{
			Skipped: true,
			Reason:  fmt.Sprintf("MCP server %q already registered", entry.Name),
		}, nil
	}

	// Build the server entry.
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

	mcpServers[entry.Name] = serverEntry

	// Write back.
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("setup: cannot marshal config: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return nil, fmt.Errorf("setup: cannot write config %q: %w", configPath, err)
	}

	return &MCPRegisterResult{
		Added:  true,
		Reason: fmt.Sprintf("registered MCP server %q", entry.Name),
	}, nil
}
