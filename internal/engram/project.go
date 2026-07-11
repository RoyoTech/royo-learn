package engram

import (
	"errors"
	"fmt"
	"strings"
)

// ProjectSource indicates how the project name was resolved.
type ProjectSource string

const (
	// SourceExplicit means the user provided an explicit project name.
	SourceExplicit ProjectSource = "explicit"
	// SourceGitRemote means the name was extracted from a git remote URL.
	SourceGitRemote ProjectSource = "git_remote"
)

// ErrAmbiguousProject is returned when the project cannot be determined.
var ErrAmbiguousProject = errors.New("engram: ambiguous project — provide an explicit project name or ensure a git remote is configured")

// ProjectResolution describes the outcome of project name resolution.
type ProjectResolution struct {
	Name   string        `json:"name"`
	Source ProjectSource `json:"source"`
}

// ResolveOptions holds the inputs for project name resolution.
type ResolveOptions struct {
	// ExplicitName is a user-provided project override. Always wins.
	ExplicitName string
	// GitRemote is the name of the git remote (e.g., "origin").
	GitRemote string
	// GitURL is the fetch/push URL of the git remote.
	GitURL string
}

// ResolveProject resolves the Engram project name from available sources.
// Precedence: explicit name > git remote name.
// Returns ErrAmbiguousProject when no name can be determined.
func ResolveProject(opts ResolveOptions) (string, ProjectSource, error) {
	if opts.ExplicitName != "" {
		return opts.ExplicitName, SourceExplicit, nil
	}

	name := extractRepoName(opts.GitURL)
	if name != "" {
		return name, SourceGitRemote, nil
	}

	return "", "", ErrAmbiguousProject
}

// extractRepoName extracts the repository name from a git URL.
// Supports HTTPS and SSH formats. Returns "" if extraction fails.
func extractRepoName(url string) string {
	if url == "" {
		return ""
	}

	// Normalize: strip trailing .git
	cleaned := strings.TrimSuffix(url, ".git")

	// Handle SSH format: git@host:user/repo.git
	if idx := strings.LastIndex(cleaned, ":"); idx != -1 {
		cleaned = cleaned[idx+1:]
	}

	// Extract the last path component
	if idx := strings.LastIndex(cleaned, "/"); idx != -1 {
		return cleaned[idx+1:]
	}

	return cleaned
}

// ResolveResult is a convenience wrapper that returns a ProjectResolution
// or the error.
func (opts ResolveOptions) Resolve() (*ProjectResolution, error) {
	name, source, err := ResolveProject(opts)
	if err != nil {
		return nil, err
	}
	return &ProjectResolution{Name: name, Source: source}, nil
}

// String returns a human-readable representation of the resolution.
func (r *ProjectResolution) String() string {
	return fmt.Sprintf("%s (source: %s)", r.Name, r.Source)
}
