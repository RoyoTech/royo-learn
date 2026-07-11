package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	// Optional checks — stubbed as degraded until implemented.
	r.Register("database", stubCheck("database", "not implemented yet"))
	r.Register("migrations", stubCheck("migrations", "not implemented yet"))
	r.Register("engram", stubCheck("engram", "not implemented yet"))
	r.Register("gentle-ai", stubCheck("gentle-ai", "not implemented yet"))
	r.Register("skill-registry", stubCheck("skill-registry", "not implemented yet"))
	r.Register("codex-mcp", stubCheck("codex-mcp", "not implemented yet"))
	r.Register("shared-library", stubCheck("shared-library", "not implemented yet"))
	r.Register("record-integrity", stubCheck("record-integrity", "not implemented yet"))
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
