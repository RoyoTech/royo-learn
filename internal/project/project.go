package project

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Stable error codes for project resolution.
const (
	ErrProjectNotFound  = "project_not_found"
	ErrAmbiguousProject = "ambiguous_project"
)

// Project holds the resolved project identity.
type Project struct {
	// Root is the absolute canonical path to the project root directory.
	Root string

	// Key is a stable human-readable project key derived from Git remote
	// information or the filesystem path.
	Key string

	// Relative is the path from the nearest Git root to the project root,
	// or empty when the project is the Git root.
	Relative string

	// GitRoot is the absolute path of the nearest Git repository root,
	// or empty when the project is not inside a Git repo.
	GitRoot string

	// GitRemote is the URL of the "origin" remote, or empty.
	GitRemote string
}

// ResolveRequest carries the hints used to locate a project.
type ResolveRequest struct {
	// CWD is the current working directory used as fallback when no other
	// hint is provided.
	CWD string

	// ExplicitRoot is a user-specified project root path. When non-empty
	// and valid, it takes the highest precedence.
	ExplicitRoot string

	// MCPRoot is the MCP working root, used when neither ExplicitRoot nor
	// a project marker is found.
	MCPRoot string
}

// Resolver finds and validates projects against a set of trusted roots.
type Resolver struct {
	trustedRoots []string
	logger       *slog.Logger
	keyDeriver   func(root string) (string, error)
}

// ResolverOption is a functional option for configuring a Resolver.
type ResolverOption func(*Resolver)

// WithTrustedRoots sets the trusted root directories.
func WithTrustedRoots(roots []string) ResolverOption {
	return func(r *Resolver) {
		r.trustedRoots = roots
	}
}

// WithLogger sets a structured logger for debugging.
func WithLogger(logger *slog.Logger) ResolverOption {
	return func(r *Resolver) {
		r.logger = logger
	}
}

// WithKeyDeriver overrides the default key derivation function.
func WithKeyDeriver(fn func(root string) (string, error)) ResolverOption {
	return func(r *Resolver) {
		r.keyDeriver = fn
	}
}

// NewResolver creates a Resolver with the given options.
func NewResolver(opts ...ResolverOption) *Resolver {
	r := &Resolver{
		keyDeriver: DeriveKey,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Error is a typed project resolution error carrying a stable error code.
type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for errors.Is/errors.As chains.
func (e *Error) Unwrap() error { return e.Err }

// Resolve locates a project given the hints in req.
//
// Precedence:
//  1. ExplicitRoot — when valid and within a trusted root.
//  2. CWD ancestor scan — walks up from CWD looking for `.royo-learn/config.yaml`.
//  3. CWD inside a Git repo — falls back to the Git root when no project marker exists.
//  4. MCPRoot — when neither of the above produces a result.
//
// If multiple equally-valid candidates exist, Resolve returns
// ErrAmbiguousProject. If no project is found, Resolve returns
// ErrProjectNotFound.
func (r *Resolver) Resolve(ctx context.Context, req *ResolveRequest) (*Project, error) {
	if req == nil {
		return nil, &Error{Code: ErrProjectNotFound, Message: "request is nil"}
	}

	// 1. Explicit root — highest precedence.
	if req.ExplicitRoot != "" {
		return r.resolveExplicit(req.ExplicitRoot)
	}

	// 2. Walk up from CWD looking for .royo-learn/config.yaml.
	if req.CWD != "" {
		return r.resolveFromCWD(req.CWD)
	}

	// 3. MCP root — lowest precedence hint.
	if req.MCPRoot != "" {
		return r.resolveExplicit(req.MCPRoot)
	}

	return nil, &Error{Code: ErrProjectNotFound, Message: "no resolution hint provided"}
}

func (r *Resolver) resolveExplicit(path string) (*Project, error) {
	canon, err := Canonicalize(path)
	if err != nil {
		return nil, &Error{Code: ErrPathOutsideRoot, Message: "explicit root is outside trusted roots", Err: err}
	}

	if err := r.ensureWithinTrusted(canon); err != nil {
		return nil, err
	}

	return r.buildProject(canon)
}

// resolveFromCWD walks up from cwd looking for the nearest project marker.
// When multiple markers are found at the same depth (siblings), ambiguity
// is returned.
func (r *Resolver) resolveFromCWD(cwd string) (*Project, error) {
	canonCWD, err := Canonicalize(cwd)
	if err != nil {
		return nil, &Error{Code: ErrProjectNotFound, Message: "cannot canonicalize CWD", Err: err}
	}

	markerRoot, err := r.walkUpForMarker(canonCWD)
	if err != nil {
		// No marker found. Try Git root fallback.
		gitRoot, gitErr := gitRoot(canonCWD)
		if gitErr != nil || gitRoot == "" {
			return nil, &Error{Code: ErrProjectNotFound, Message: "no project marker or git repo found"}
		}
		if err := r.ensureWithinTrusted(gitRoot); err != nil {
			return nil, err
		}
		return r.buildProject(gitRoot)
	}

	// Found a marker. Check for sibling ambiguity.
	parent := filepath.Dir(markerRoot)
	if parent != markerRoot {
		if err := r.checkSiblingMarkers(parent, markerRoot); err != nil {
			return nil, err
		}
	}

	if err := r.ensureWithinTrusted(markerRoot); err != nil {
		return nil, err
	}

	return r.buildProject(markerRoot)
}

// walkUpForMarker walks up from dir looking for .royo-learn/config.yaml.
// Returns the directory containing the marker, or an error.
func (r *Resolver) walkUpForMarker(dir string) (string, error) {
	current := dir
	for {
		markerPath := filepath.Join(current, ".royo-learn", "config.yaml")
		if info, statErr := os.Stat(markerPath); statErr == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			return "", &Error{Code: ErrProjectNotFound, Message: "no project marker found"}
		}
		current = parent
	}
}

// checkSiblingMarkers checks if there are multiple directories under parent
// that contain .royo-learn/config.yaml. If there are two or more, and the
// walker found one, it returns ambiguous_project.
func (r *Resolver) checkSiblingMarkers(parent, found string) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil // Can't read, don't block resolution.
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		markerPath := filepath.Join(parent, entry.Name(), ".royo-learn", "config.yaml")
		if info, statErr := os.Stat(markerPath); statErr == nil && !info.IsDir() {
			count++
			if count > 1 {
				return &Error{
					Code:    ErrAmbiguousProject,
					Message: fmt.Sprintf("multiple project markers found under %s", parent),
				}
			}
		}
	}
	return nil
}

// ensureWithinTrusted checks that path is inside at least one trusted root.
func (r *Resolver) ensureWithinTrusted(path string) error {
	if len(r.trustedRoots) == 0 {
		return nil // No restrictions configured.
	}
	for _, root := range r.trustedRoots {
		if IsInsideRoot(path, root) {
			return nil
		}
	}
	return &Error{
		Code:    ErrPathOutsideRoot,
		Message: fmt.Sprintf("project path %s is outside trusted roots", path),
	}
}

// buildProject constructs a Project from an absolute root path.
func (r *Resolver) buildProject(root string) (*Project, error) {
	canon, err := Canonicalize(root)
	if err != nil {
		return nil, err
	}

	key, err := r.keyDeriver(canon)
	if err != nil {
		return nil, &Error{Code: ErrProjectNotFound, Message: "key derivation failed", Err: err}
	}

	proj := &Project{
		Root: canon,
		Key:  key,
	}

	// Populate Git information.
	if gr, gErr := gitRoot(canon); gErr == nil && gr != "" {
		proj.GitRoot = gr
		proj.GitRemote = gitRemote(gr)
		if gr != canon {
			rel, rErr := filepath.Rel(gr, canon)
			if rErr == nil {
				proj.Relative = filepath.ToSlash(rel)
			}
		}
	}

	if r.logger != nil {
		r.logger.Debug("resolved project",
			"root", proj.Root,
			"key", proj.Key,
			"git_root", proj.GitRoot,
		)
	}

	return proj, nil
}
