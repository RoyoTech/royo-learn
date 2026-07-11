package project

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// DeriveKey computes a stable, human-friendly project key from the project
// root directory.
//
// When the directory is inside a Git repository the key is derived from the
// remote URL and the relative path from the Git root. When no Git metadata
// exists it falls back to the first 12 hex characters of the SHA-256 digest
// of the canonical absolute path.
//
// The resulting key is always lowercase with separators replaced by dashes
// (kebab-case).
func DeriveKey(root string) (string, error) {
	canon, err := Canonicalize(root)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	gitRoot, err := gitRoot(canon)
	if err != nil || gitRoot == "" {
		// Fallback: hash the canonical absolute path.
		return pathHash(canon), nil
	}

	remote := gitRemote(gitRoot)
	if remote == "" {
		return pathHash(canon), nil
	}

	// Extract a meaningful slug from the remote URL.
	slug := extractRepoSlug(remote)

	// If the project is not the git root, append the relative path.
	if gitRoot != canon {
		rel, err := filepath.Rel(gitRoot, canon)
		if err == nil && rel != "." {
			relSlug := strings.ReplaceAll(strings.ReplaceAll(rel, string(filepath.Separator), "-"), "_", "-")
			slug = slug + "-" + relSlug
		}
	}

	return normalizeKey(slug), nil
}

// normalizeKey ensures the key is lowercase and uses kebab-case.
func normalizeKey(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// extractRepoSlug extracts a human-readable slug from a Git remote URL.
func extractRepoSlug(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")

	// Handle SSH URLs: git@github.com:owner/repo
	if idx := strings.LastIndex(url, ":"); idx >= 0 && !strings.Contains(url, "://") {
		url = url[idx+1:]
	}

	// Handle HTTPS URLs: https://github.com/owner/repo
	// Find the last path segment.
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}

// pathHash returns the first 12 hex characters of the SHA-256 digest of path.
func pathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", sum[:6]) // 6 bytes = 12 hex chars
}

// gitRoot returns the absolute path of the nearest Git repository root
// containing dir, or empty string if not inside a Git repo.
func gitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", nil // Not a git repo — not an error.
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", nil
	}
	return Canonicalize(root)
}

// gitRemote returns the URL of the "origin" remote, or empty if none.
func gitRemote(gitRoot string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
