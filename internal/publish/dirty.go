package publish

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// utcNowPublish returns the current UTC time for publish operations.
func utcNowPublish() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// DirtyTargetResult describes the dirty state of target files.
type DirtyTargetResult struct {
	IsDirty    bool
	DirtyFiles []string
	Reason     string
}

// CheckDirtyWorktree checks if any of the specified target files have
// uncommitted changes in the git working tree. Returns a result indicating
// whether a publish should be blocked due to dirty state.
func CheckDirtyWorktree(projectRoot string, targets []TargetResolution) (*DirtyTargetResult, error) {
	result := &DirtyTargetResult{}

	// Check if the project root is a git repository.
	gitDir := filepath.Join(projectRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Not a git repo — cannot check dirty state, allow publish.
		return result, nil
	}

	// Run git status --porcelain to detect uncommitted changes.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("CheckDirtyWorktree: git status: %w", err)
	}

	if len(out) == 0 {
		// Clean working tree.
		return result, nil
	}

	// Parse dirty files.
	dirtyFiles := parseGitStatus(string(out))

	// Check if any of our targets overlap with dirty files.
	for _, target := range targets {
		absTarget := filepath.Join(projectRoot, target.Root, target.Path)
		for _, dirty := range dirtyFiles {
			dirtyAbs := filepath.Join(projectRoot, dirty)
			if absTarget == dirtyAbs {
				result.DirtyFiles = append(result.DirtyFiles, target.Path)
			}
		}
	}

	if len(result.DirtyFiles) > 0 {
		result.IsDirty = true
		result.Reason = fmt.Sprintf("target files have uncommitted changes: %s",
			strings.Join(result.DirtyFiles, ", "))
	}

	return result, nil
}

// parseGitStatus parses `git status --porcelain` output and returns a list of
// modified file paths (relative to repo root).
func parseGitStatus(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// Status format: XY filename (where X=index status, Y=worktree status)
		file := strings.TrimSpace(line[3:])
		if file != "" {
			files = append(files, file)
		}
	}
	return files
}
