package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agent-royo-learn/internal/setup"
)

// royoLearnSkillsSubdir is the subdirectory under the project root that
// holds the canonical royo-learn skills to be installed into agents.
const royoLearnSkillsSubdir = "skills"

// royoLearnMCPServerName is the canonical MCP server name registered
// across all supported agents.
const royoLearnMCPServerName = "royo-learn"

// setupResult is the JSON shape for the setup command family.
type setupResult struct {
	Agent   string        `json:"agent"`
	Action  string        `json:"action"`
	Path    string        `json:"path,omitempty"`
	Backup  string        `json:"backup,omitempty"`
	Added   bool          `json:"added,omitempty"`
	Skipped bool          `json:"skipped,omitempty"`
	Reason  string        `json:"reason,omitempty"`
	MCP     bool          `json:"mcp_registered,omitempty"`
	Skills  *skillSummary `json:"skills,omitempty"`
	Error   string        `json:"error,omitempty"`
	Notes   []string      `json:"notes,omitempty"`
}

type skillSummary struct {
	Installed int      `json:"installed"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
	Target    string   `json:"target"`
}

func runSetup(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeSetupError(stderr, "setup requires a subcommand: install | uninstall | status")
	}

	switch args[0] {
	case "install":
		return runSetupInstall(args[1:], stdout, stderr)
	case "uninstall":
		return runSetupUninstall(args[1:], stdout, stderr)
	case "status":
		return runSetupStatus(args[1:], stdout, stderr)
	default:
		return writeSetupError(stderr, "unknown setup subcommand %q: use install, uninstall, or status", args[0])
	}
}

func runSetupInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup install", flag.ContinueOnError)
	agentFlag := fs.String("agent", "all", "target agent: claude-code | codex | opencode | all")
	binaryPath := fs.String("binary", "", "absolute path to the royo-learn binary (default: resolve from PATH)")
	projectRoot := fs.String("project-root", "", "project root containing skills/ (default: current dir or repo root)")
	dryRun := fs.Bool("dry-run", false, "report actions without applying changes")
	skipMCP := fs.Bool("skip-mcp", false, "skip MCP server registration")
	skipSkills := fs.Bool("skip-skills", false, "skip skill installation")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writeSetupError(stderr, "setup install: %v", err)
	}

	kinds, err := parseAgentKinds(*agentFlag)
	if err != nil {
		return writeSetupError(stderr, "%v", err)
	}

	binary, err := resolveRoyoLearnBinary(*binaryPath)
	if err != nil {
		return writeSetupError(stderr, "%v", err)
	}

	skillsSrc, err := resolveSkillsSource(*projectRoot)
	if err != nil {
		return writeSetupError(stderr, "%v", err)
	}

	results := make([]setupResult, 0, len(kinds))
	exitCode := exitSuccess

	for _, kind := range kinds {
		res := installForAgent(kind, binary, skillsSrc, *dryRun, *skipMCP, *skipSkills)
		results = append(results, res)
		if res.Error != "" {
			exitCode = exitFailure
		}
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"command": "setup install",
			"binary":  binary,
			"dry_run": *dryRun,
			"results": results,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, r := range results {
			printInstallSummary(stdout, r)
		}
	}

	return exitCode
}

func runSetupUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup uninstall", flag.ContinueOnError)
	agentFlag := fs.String("agent", "all", "target agent: claude-code | codex | opencode | all")
	dryRun := fs.Bool("dry-run", false, "report actions without applying changes")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writeSetupError(stderr, "setup uninstall: %v", err)
	}

	kinds, err := parseAgentKinds(*agentFlag)
	if err != nil {
		return writeSetupError(stderr, "%v", err)
	}

	results := make([]setupResult, 0, len(kinds))
	exitCode := exitSuccess

	for _, kind := range kinds {
		res := uninstallForAgent(kind, *dryRun)
		results = append(results, res)
		if res.Error != "" {
			exitCode = exitFailure
		}
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"command": "setup uninstall",
			"dry_run": *dryRun,
			"results": results,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, r := range results {
			printInstallSummary(stdout, r)
		}
	}

	return exitCode
}

func runSetupStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup status", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writeSetupError(stderr, "setup status: %v", err)
	}

	type statusEntry struct {
		Agent         string `json:"agent"`
		DisplayName   string `json:"display_name"`
		Installed     bool   `json:"installed"`
		ConfigPath    string `json:"config_path"`
		ConfigExists  bool   `json:"config_exists"`
		MCPRegistered bool   `json:"mcp_registered"`
		SkillsDir     string `json:"skills_dir"`
	}

	entries := make([]statusEntry, 0, len(setup.AllAgents))
	for _, kind := range setup.AllAgents {
		a, err := setup.ResolveAgent(kind)
		if err != nil {
			return writeSetupError(stderr, "%v", err)
		}
		cfgPath := a.MCPConfigPath()
		cfgExists := false
		if cfgPath != "" {
			if _, statErr := os.Stat(cfgPath); statErr == nil {
				cfgExists = true
			}
		}
		registered, _ := a.VerifyMCP(royoLearnMCPServerName)
		skillsDir, _ := a.SkillsDir()
		entries = append(entries, statusEntry{
			Agent:         string(a.Kind()),
			DisplayName:   a.DisplayName(),
			Installed:     a.IsInstalled(),
			ConfigPath:    cfgPath,
			ConfigExists:  cfgExists,
			MCPRegistered: registered,
			SkillsDir:     skillsDir,
		})
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"command": "setup status",
			"agents":  entries,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, e := range entries {
			_, _ = fmt.Fprintf(stdout, "%s (%s)\n", e.DisplayName, e.Agent)
			_, _ = fmt.Fprintf(stdout, "  binary installed:    %v\n", e.Installed)
			_, _ = fmt.Fprintf(stdout, "  MCP config path:     %s\n", fallbackIfEmpty(e.ConfigPath, "(none)"))
			_, _ = fmt.Fprintf(stdout, "  MCP config exists:   %v\n", e.ConfigExists)
			_, _ = fmt.Fprintf(stdout, "  MCP registered:      %v\n", e.MCPRegistered)
			_, _ = fmt.Fprintf(stdout, "  skills directory:    %s\n", fallbackIfEmpty(e.SkillsDir, "(none)"))
		}
	}
	return exitSuccess
}

// parseAgentKinds validates --agent flag and expands "all".
func parseAgentKinds(spec string) ([]setup.AgentKind, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" {
		return nil, fmt.Errorf("setup: --agent is required")
	}
	if spec == "all" {
		return setup.AllAgents, nil
	}
	parts := strings.Split(spec, ",")
	out := make([]setup.AgentKind, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "all" {
			out = append(out, setup.AllAgents...)
			continue
		}
		a, err := setup.ResolveAgent(setup.AgentKind(p))
		if err != nil {
			return nil, err
		}
		out = append(out, a.Kind())
	}
	return dedupeAgents(out), nil
}

func dedupeAgents(in []setup.AgentKind) []setup.AgentKind {
	seen := make(map[setup.AgentKind]bool, len(in))
	out := make([]setup.AgentKind, 0, len(in))
	for _, k := range in {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// resolveRoyoLearnBinary returns the absolute path to the royo-learn
// binary to register. If binaryPath is set it is returned verbatim;
// otherwise os.Executable() is used.
func resolveRoyoLearnBinary(binaryPath string) (string, error) {
	if binaryPath != "" {
		abs, err := filepath.Abs(binaryPath)
		if err != nil {
			return "", fmt.Errorf("setup: cannot resolve --binary: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("setup: binary not found at %q: %w", abs, err)
		}
		return abs, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("setup: cannot determine current executable: %w", err)
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("setup: cannot absolutize executable: %w", err)
	}
	return abs, nil
}

// resolveSkillsSource returns the absolute path to the skills directory
// to install. It prefers an explicit --project-root, then the current
// working directory, then walks upward looking for a `skills/` folder.
func resolveSkillsSource(projectRoot string) (string, error) {
	if projectRoot != "" {
		abs, err := filepath.Abs(projectRoot)
		if err != nil {
			return "", fmt.Errorf("setup: cannot resolve --project-root: %w", err)
		}
		candidate := filepath.Join(abs, royoLearnSkillsSubdir)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		return "", fmt.Errorf("setup: skills directory not found at %s", candidate)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("setup: cannot read current directory: %w", err)
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, royoLearnSkillsSubdir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("setup: cannot locate a %q directory from %s; pass --project-root", royoLearnSkillsSubdir, cwd)
}

func installForAgent(kind setup.AgentKind, binary, skillsSrc string, dryRun, skipMCP, skipSkills bool) setupResult {
	a, err := setup.ResolveAgent(kind)
	if err != nil {
		return setupResult{Agent: string(kind), Action: "install", Error: err.Error()}
	}

	res := setupResult{
		Agent:  string(kind),
		Action: "install",
		Notes:  []string{},
	}

	if !a.IsInstalled() {
		res.Notes = append(res.Notes, fmt.Sprintf("%s binary not on PATH; MCP registration may still succeed if config exists", a.DisplayName()))
	}

	cfgPath := a.MCPConfigPath()
	if cfgPath != "" {
		res.Path = cfgPath
	}

	if !skipMCP {
		if dryRun {
			res.Notes = append(res.Notes, fmt.Sprintf("would register MCP server in %s", fallbackIfEmpty(cfgPath, a.DisplayName()+"'s native config")))
		} else {
			// Backup before mutation.
			if cfgPath != "" {
				if backup, berr := a.BackupMCPConfig(); berr != nil {
					res.Error = fmt.Sprintf("backup failed: %v", berr)
					return res
				} else if backup != "" {
					res.Backup = backup
				}
			}
			entry := setup.MCPServerEntry{
				Name:    royoLearnMCPServerName,
				Command: binary,
				Args:    []string{"mcp-serve"},
			}
			mcpResult, mcpErr := a.RegisterMCP(entry)
			if mcpErr != nil {
				res.Error = fmt.Sprintf("MCP registration failed: %v", mcpErr)
				return res
			}
			res.Added = mcpResult.Added
			res.Skipped = mcpResult.Skipped
			res.Reason = mcpResult.Reason
			res.MCP = true
		}
	}

	if !skipSkills {
		skillsDir, sErr := a.SkillsDir()
		if sErr != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("skills dir unavailable: %v", sErr))
		} else if dryRun {
			res.Notes = append(res.Notes, fmt.Sprintf("would install skills from %s to %s", skillsSrc, skillsDir))
		} else {
			if err := os.MkdirAll(skillsDir, 0o755); err != nil {
				res.Error = fmt.Sprintf("cannot create skills dir %s: %v", skillsDir, err)
				return res
			}
			insResult, iErr := setup.InstallSkills(skillsSrc, skillsDir)
			if iErr != nil {
				res.Error = fmt.Sprintf("skill install failed: %v", iErr)
				return res
			}
			res.Skills = &skillSummary{
				Installed: insResult.Installed,
				Skipped:   insResult.Skipped,
				Errors:    insResult.Errors,
				Target:    skillsDir,
			}
		}
	}

	return res
}

func uninstallForAgent(kind setup.AgentKind, dryRun bool) setupResult {
	a, err := setup.ResolveAgent(kind)
	if err != nil {
		return setupResult{Agent: string(kind), Action: "uninstall", Error: err.Error()}
	}
	res := setupResult{Agent: string(kind), Action: "uninstall"}

	cfgPath := a.MCPConfigPath()
	if cfgPath != "" {
		res.Path = cfgPath
	}

	if dryRun {
		res.Notes = append(res.Notes, fmt.Sprintf("would remove MCP server %q from %s", royoLearnMCPServerName, fallbackIfEmpty(cfgPath, a.DisplayName())))
		return res
	}

	if cfgPath != "" {
		if backup, berr := a.BackupMCPConfig(); berr == nil && backup != "" {
			res.Backup = backup
		}
	}
	if err := a.UnregisterMCP(royoLearnMCPServerName); err != nil {
		res.Error = fmt.Sprintf("MCP unregister failed: %v", err)
		return res
	}
	res.MCP = true
	res.Added = false
	res.Reason = fmt.Sprintf("removed MCP server %q", royoLearnMCPServerName)
	return res
}

func printInstallSummary(w io.Writer, r setupResult) {
	_, _ = fmt.Fprintf(w, "[%s] %s\n", r.Agent, r.Action)
	if r.Path != "" {
		_, _ = fmt.Fprintf(w, "  config: %s\n", r.Path)
	}
	if r.Backup != "" {
		_, _ = fmt.Fprintf(w, "  backup: %s\n", r.Backup)
	}
	if r.MCP {
		if r.Skipped {
			_, _ = fmt.Fprintf(w, "  MCP:    skipped — %s\n", r.Reason)
		} else {
			_, _ = fmt.Fprintf(w, "  MCP:    added — %s\n", r.Reason)
		}
	}
	if r.Skills != nil {
		_, _ = fmt.Fprintf(w, "  skills: installed=%d skipped=%d target=%s\n",
			r.Skills.Installed, r.Skills.Skipped, r.Skills.Target)
		for _, e := range r.Skills.Errors {
			_, _ = fmt.Fprintf(w, "    error: %s\n", e)
		}
	}
	for _, note := range r.Notes {
		_, _ = fmt.Fprintf(w, "  note:   %s\n", note)
	}
	if r.Error != "" {
		_, _ = fmt.Fprintf(w, "  ERROR:  %s\n", r.Error)
	}
}

func writeSetupError(stderr io.Writer, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(stderr, "[royo-learn] %s\n", msg)
	_, _ = fmt.Fprintf(stderr, "usage: royo-learn setup {install|uninstall|status} [--agent claude-code|codex|opencode|all] [--json]\n")
	return exitFailure
}

func fallbackIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
