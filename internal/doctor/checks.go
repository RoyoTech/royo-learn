package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"agent-royo-learn/internal/coherence"
	"agent-royo-learn/internal/config"
	"agent-royo-learn/internal/project"
)

// ---------------------------------------------------------------------------
// Check registration
// ---------------------------------------------------------------------------

func (r *Runner) registerBuiltinChecks() {
	// Core checks — failure means the system cannot operate.
	r.Register("config", configCheck)
	r.Register("project", projectCheck)
	r.Register("git", gitCheck)
	r.Register("filesystem", filesystemCheck)

	// Optional checks — now implemented.
	r.Register("gentle-ai", gentleAICheck)
	r.Register("skill-registry", skillRegistryCheck)
	r.Register("codex-mcp", codexMCPCheck)
	r.Register("database", stubCheck("database", "not implemented yet"))
	r.Register("migrations", stubCheck("migrations", "not implemented yet"))
	r.Register("engram", stubCheck("engram", "not implemented yet"))
	r.Register("shared-library", stubCheck("shared-library", "not implemented yet"))
	r.Register("record-integrity", recordIntegrityCheck)
}

// recordIntegrityCheck detects divergences between SQLite (the operational
// source of truth) and the derived Markdown records (D6). It stays degraded when
// no store is bound (WithStore), so it never blocks an environment-only doctor
// run; when a store is bound it passes on full coherence and fails on any
// divergence, pointing at `rebuild-index` to repair.
func recordIntegrityCheck(ctx context.Context, r *Runner) *Check {
	if r.store == nil {
		return &Check{
			Name:    "record-integrity",
			Status:  StatusDegraded,
			Message: "no database bound; run via `royo-learn doctor` to check SQLite<->Markdown coherence",
			Detail:  "this check reports pass/fail only when doctor binds the project store",
		}
	}

	divergences, err := coherence.Audit(ctx, r.store, r.projectID, r.recordsDir)
	if err != nil {
		return &Check{
			Name:    "record-integrity",
			Status:  StatusFail,
			Message: fmt.Sprintf("coherence audit failed: %v", err),
		}
	}
	if len(divergences) == 0 {
		return &Check{
			Name:    "record-integrity",
			Status:  StatusPass,
			Message: "SQLite and Markdown records are coherent",
		}
	}

	counts := map[coherence.DivergenceKind]int{}
	for _, d := range divergences {
		counts[d.Kind]++
	}
	return &Check{
		Name:    "record-integrity",
		Status:  StatusFail,
		Message: fmt.Sprintf("%d divergence(s) between SQLite and Markdown; run `royo-learn rebuild-index` to repair", len(divergences)),
		Detail: fmt.Sprintf("missing=%d divergent=%d orphan=%d",
			counts[coherence.MissingRecord], counts[coherence.DivergentRecord], counts[coherence.OrphanRecord]),
	}
}

// stubCheck returns a CheckFn that reports degraded with a reason.
func stubCheck(name, reason string) CheckFn {
	return func(ctx context.Context, r *Runner) *Check {
		return &Check{
			Name:    name,
			Status:  StatusDegraded,
			Message: reason,
			Detail:  "this check is a stub and will be implemented in a future version",
		}
	}
}

// ---------------------------------------------------------------------------
// configCheck — validates the project configuration.
// ---------------------------------------------------------------------------

func configCheck(ctx context.Context, r *Runner) *Check {
	if r.projectRoot == "" {
		return &Check{
			Name:    "config",
			Status:  StatusFail,
			Message: "project root is not set",
		}
	}

	cfg, err := config.LoadAndValidate(r.projectRoot, r.trustedRoots)
	if err != nil {
		return &Check{
			Name:    "config",
			Status:  StatusFail,
			Message: fmt.Sprintf("config validation failed: %v", err),
		}
	}

	return &Check{
		Name:    "config",
		Status:  StatusPass,
		Message: fmt.Sprintf("config loaded (version %d)", cfg.Version),
	}
}

// ---------------------------------------------------------------------------
// projectCheck — verifies the project is resolved and within trusted roots.
// ---------------------------------------------------------------------------

func projectCheck(ctx context.Context, r *Runner) *Check {
	if r.projectRoot == "" {
		return &Check{
			Name:    "project",
			Status:  StatusFail,
			Message: "project root is not set",
		}
	}

	resolver := project.NewResolver(
		project.WithTrustedRoots(r.trustedRoots),
	)

	req := &project.ResolveRequest{
		ExplicitRoot: r.projectRoot,
	}

	proj, err := resolver.Resolve(ctx, req)
	if err != nil {
		return &Check{
			Name:    "project",
			Status:  StatusFail,
			Message: fmt.Sprintf("project resolution failed: %v", err),
		}
	}

	msg := fmt.Sprintf("project resolved: key=%s root=%s", proj.Key, proj.Root)
	if proj.GitRoot != "" {
		msg += fmt.Sprintf(" git_root=%s", proj.GitRoot)
	}
	if proj.GitRemote != "" {
		msg += fmt.Sprintf(" remote=%s", proj.GitRemote)
	}

	return &Check{
		Name:    "project",
		Status:  StatusPass,
		Message: msg,
		Detail:  proj.Key,
	}
}

// ---------------------------------------------------------------------------
// gitCheck — verifies the git repository is accessible and clean.
// ---------------------------------------------------------------------------

func gitCheck(ctx context.Context, r *Runner) *Check {
	if r.projectRoot == "" {
		return &Check{
			Name:    "git",
			Status:  StatusFail,
			Message: "project root is not set",
		}
	}

	// Check if .git directory exists at the project root or above.
	gitRoot := findGitRoot(r.projectRoot)
	if gitRoot == "" {
		// Not a git repo — this is a degraded condition, not a failure.
		return &Check{
			Name:    "git",
			Status:  StatusDegraded,
			Message: "not a git repository — version control is recommended",
			Detail:  "no .git directory found in project ancestry",
		}
	}

	return &Check{
		Name:    "git",
		Status:  StatusPass,
		Message: fmt.Sprintf("git repository found at %s", gitRoot),
		Detail:  gitRoot,
	}
}

// findGitRoot walks up from dir looking for a .git directory.
func findGitRoot(dir string) string {
	current := dir
	for {
		gitPath := filepath.Join(current, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// ---------------------------------------------------------------------------
// filesystemCheck — verifies paths exist, permissions are sane.
// fix-safe creates missing .royo-learn directories.
// ---------------------------------------------------------------------------

const royoLearnDir = ".royo-learn"

func filesystemCheck(ctx context.Context, r *Runner) *Check {
	if r.projectRoot == "" {
		return &Check{
			Name:    "filesystem",
			Status:  StatusFail,
			Message: "project root is not set",
		}
	}

	royoDir := filepath.Join(r.projectRoot, royoLearnDir)

	info, err := os.Stat(royoDir)
	if err != nil {
		if os.IsNotExist(err) {
			// The .royo-learn directory is missing.
			if r.fixSafe {
				// Auto-repair: create the directory.
				if mkErr := os.MkdirAll(royoDir, 0o755); mkErr != nil {
					return &Check{
						Name:    "filesystem",
						Status:  StatusFail,
						Message: fmt.Sprintf("cannot create %s: %v", royoDir, mkErr),
					}
				}
				return &Check{
					Name:    "filesystem",
					Status:  StatusPass,
					Message: fmt.Sprintf("created missing directory: %s", royoDir),
					Detail:  "fix-safe: directory auto-created",
				}
			}
			return &Check{
				Name:    "filesystem",
				Status:  StatusFail,
				Message: fmt.Sprintf("%s directory does not exist (run with --fix-safe to create)", royoDir),
			}
		}
		return &Check{
			Name:    "filesystem",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot stat %s: %v", royoDir, err),
		}
	}

	if !info.IsDir() {
		return &Check{
			Name:    "filesystem",
			Status:  StatusFail,
			Message: fmt.Sprintf("%s exists but is not a directory", royoDir),
		}
	}

	// Verify the path is not a symlink escaping trusted roots.
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(royoDir)
		if err != nil {
			return &Check{
				Name:    "filesystem",
				Status:  StatusFail,
				Message: fmt.Sprintf("cannot resolve symlink %s: %v", royoDir, err),
			}
		}
		if !project.IsInsideRoot(resolved, r.projectRoot) {
			return &Check{
				Name:    "filesystem",
				Status:  StatusFail,
				Message: fmt.Sprintf("%s symlink escapes project root: %s -> %s", royoDir, royoDir, resolved),
			}
		}
	}

	return &Check{
		Name:    "filesystem",
		Status:  StatusPass,
		Message: fmt.Sprintf("%s directory exists", royoDir),
	}
}

// ---------------------------------------------------------------------------
// setup checks — verify agent environment integration
// ---------------------------------------------------------------------------

// gentleAICheck verifies that the Gentle-AI/Codex config file exists
// and is a valid JSON file.
func gentleAICheck(ctx context.Context, r *Runner) *Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return &Check{
			Name:    "gentle-ai",
			Status:  StatusDegraded,
			Message: "cannot determine home directory for gentle-ai config",
			Detail:  err.Error(),
		}
	}

	cfgPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	info, err := os.Stat(cfgPath)
	if err != nil {
		return &Check{
			Name:    "gentle-ai",
			Status:  StatusDegraded,
			Message: fmt.Sprintf("gentle-ai config not found at %s", cfgPath),
			Detail:  "run royo-learn setup to register",
		}
	}

	return &Check{
		Name:    "gentle-ai",
		Status:  StatusPass,
		Message: fmt.Sprintf("gentle-ai config found (%d bytes)", info.Size()),
		Detail:  cfgPath,
	}
}

// skillRegistryCheck verifies that the project has skills installed
// in the agent's skill directory.
func skillRegistryCheck(ctx context.Context, r *Runner) *Check {
	if r.projectRoot == "" {
		return &Check{
			Name:    "skill-registry",
			Status:  StatusFail,
			Message: "project root is not set",
		}
	}

	skillsDir := filepath.Join(r.projectRoot, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return &Check{
			Name:    "skill-registry",
			Status:  StatusDegraded,
			Message: fmt.Sprintf("cannot read skills dir %s: %v", skillsDir, err),
		}
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() {
			// Check for SKILL.md.
			skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				count++
			}
		}
	}

	if count == 0 {
		return &Check{
			Name:    "skill-registry",
			Status:  StatusDegraded,
			Message: "no skills found in project",
			Detail:  "run royo-learn setup to install skills",
		}
	}

	return &Check{
		Name:    "skill-registry",
		Status:  StatusPass,
		Message: fmt.Sprintf("%d project skill(s) found", count),
		Detail:  skillsDir,
	}
}

// codexMCPCheck verifies that royo-learn is registered as an MCP server
// in the agent config.
func codexMCPCheck(ctx context.Context, r *Runner) *Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return &Check{
			Name:    "codex-mcp",
			Status:  StatusDegraded,
			Message: "cannot determine home directory",
			Detail:  err.Error(),
		}
	}

	cfgPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return &Check{
			Name:    "codex-mcp",
			Status:  StatusDegraded,
			Message: fmt.Sprintf("cannot read config at %s", cfgPath),
		}
	}

	var raw map[string]any
	if err := json.Unmarshal(cfgBytes, &raw); err != nil {
		return &Check{
			Name:    "codex-mcp",
			Status:  StatusDegraded,
			Message: "cannot parse agent config as JSON",
		}
	}

	for _, providers := range []string{"mcpServers", "mcp_servers"} {
		if servers, ok := raw[providers].(map[string]any); ok {
			for name, entry := range servers {
				if _, isMap := entry.(map[string]any); isMap {
					if name == "royo-learn" {
						return &Check{
							Name:    "codex-mcp",
							Status:  StatusPass,
							Message: "royo-learn is registered as MCP server",
							Detail:  cfgPath,
						}
					}
				}
			}
		}
	}

	return &Check{
		Name:    "codex-mcp",
		Status:  StatusDegraded,
		Message: "royo-learn is not registered as an MCP server",
		Detail:  "run royo-learn setup to register",
	}
}
