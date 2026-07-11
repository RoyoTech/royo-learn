package setup

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// AgentKind identifies a supported AI coding agent.
type AgentKind string

const (
	AgentClaudeCode AgentKind = "claude-code"
	AgentCodex      AgentKind = "codex"
	AgentOpenCode   AgentKind = "opencode"
)

// AllAgents lists every supported agent in deterministic order.
var AllAgents = []AgentKind{AgentClaudeCode, AgentCodex, AgentOpenCode}

// Agent is the interface implemented by every supported AI coding agent.
//
// Each agent has a distinct configuration file layout (JSON, TOML) and
// distinct paths for MCP server registration and skill installation.
// All paths returned by Agent MUST be absolute.
type Agent interface {
	// Kind returns the canonical identifier (e.g. "claude-code").
	Kind() AgentKind
	// DisplayName returns a human-friendly name (e.g. "Claude Code").
	DisplayName() string
	// IsInstalled reports whether the agent's binary is available on PATH.
	IsInstalled() bool
	// MCPConfigPath returns the absolute path to the MCP configuration file.
	// Returns an empty string if the agent does not use a config file
	// (for example, Codex when registering via its CLI).
	MCPConfigPath() string
	// SkillsDir returns the absolute path to the per-user skills directory.
	SkillsDir() (string, error)
	// BackupMCPConfig creates a timestamped backup of the MCP config file.
	// Returns the backup path or "" if the agent has no config file.
	BackupMCPConfig() (string, error)
	// RegisterMCP adds the MCP server entry to the agent's configuration.
	// Implementations MUST be idempotent — calling twice with the same
	// entry must not produce duplicates.
	RegisterMCP(entry MCPServerEntry) (*MCPRegisterResult, error)
	// UnregisterMCP removes the MCP server entry by name.
	UnregisterMCP(name string) error
	// VerifyMCP reports whether an MCP entry with the given name is registered.
	VerifyMCP(name string) (bool, error)
}

// ResolveAgent returns the Agent implementation for kind, or an error
// if kind is not recognized.
func ResolveAgent(kind AgentKind) (Agent, error) {
	switch kind {
	case AgentClaudeCode:
		return NewClaudeCode(), nil
	case AgentCodex:
		return NewCodex(), nil
	case AgentOpenCode:
		return NewOpenCode(), nil
	default:
		return nil, fmt.Errorf("setup: unknown agent %q (supported: claude-code, codex, opencode)", kind)
	}
}

// HomeDir returns the user's home directory with platform-correct overrides.
// On Windows it honors USERPROFILE; on POSIX it honors HOME.
func HomeDir() string {
	if runtime.GOOS == "windows" {
		if v := envOr("USERPROFILE", ""); v != "" {
			return v
		}
	}
	return envOr("HOME", "")
}

func envOr(key, fallback string) string {
	// Lazy import-style: keep zero deps by reading os.Getenv directly.
	v, ok := lookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	return v
}

// lookupEnv is a thin wrapper over os.LookupEnv that avoids an import in
// the interface header. It exists so this file can stay small and focused.
func lookupEnv(key string) (string, bool) {
	return getenv(key)
}

// binaryOnPath reports whether name resolves to an executable on PATH.
func binaryOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// writeFileAtomic writes data to path by first writing to path+".tmp"
// then renaming it into place. This avoids partially-written files if
// the process is interrupted mid-write.
func writeFileAtomic(path string, data []byte, perm uint32) error {
	if err := writeFile(path+".tmp", data, perm); err != nil {
		return err
	}
	if err := renameFile(path+".tmp", path); err != nil {
		_ = removeFile(path + ".tmp")
		return err
	}
	return nil
}

// runCommand is a small helper that runs name with args, returning the
// combined stdout and stderr. It is used by agents that shell out to
// installer CLIs (e.g. `codex mcp add`).
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("setup: %s %v failed: %w (output: %s)", name, args, err, string(out))
	}
	return string(out), nil
}

// Discard is io.Discard, re-exported so agents can wire silent writers
// without importing io themselves.
var Discard io.Writer = io.Discard
