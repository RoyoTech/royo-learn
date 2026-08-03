package opencode

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"agent-royo-learn/internal/domain"
	projectpath "agent-royo-learn/internal/project"
)

// opencodeDBName is the exact filename the adapter recognizes as an OpenCode
// session store. Future versions that key on schema content (rather than
// name) should still treat this literal as the canonical case.
const opencodeDBName = "opencode.db"

// maxDiscoveryDepth caps how deep the recursive walk descends into the
// project tree. The contract does not pin a number; eight levels is far
// more than any realistic project layout and bounds the worst-case scan
// cost without limiting common configurations.
const maxDiscoveryDepth = 8

// Discover locates OpenCode session stores reachable from projectRoot.
// The walk only descends into real directories (symlinks to directories are
// skipped), names every candidate file exactly "opencode.db", canonicalizes
// each candidate and verifies it stays inside the canonical project root.
//
// Symlinks whose target lies outside the trust boundary are skipped
// silently: the threat-model rule (docs/24-EXPERIENCE-THREAT-MODEL.md §3 T4)
// requires the adapter to never widen its discovery surface, and exposing
// the path of the rejected link would leak filesystem layout to the caller.
//
// Discover never mutates the filesystem and never opens any candidate
// database; Health is the gate that confirms a candidate is a readable
// OpenCode store.
func (a *Adapter) Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectRoot) == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"opencode discover: project root is required")
	}

	canonRoot, err := projectpath.Canonicalize(projectRoot)
	if err != nil {
		return nil, locatorError(err)
	}

	var instances []SourceInstance
	walkErr := filepath.WalkDir(canonRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			// WalkDir reports filesystem errors per entry. We skip the
			// offending subtree and keep walking the rest; the caller's
			// contract is "find every reachable opencode.db" not
			// "guarantee every directory is readable".
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if projectpath.IsProtectedPath(path) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != canonRoot {
				rel, relErr := filepath.Rel(canonRoot, path)
				if relErr == nil && depthOf(rel) > maxDiscoveryDepth {
					return fs.SkipDir
				}
			}
			return nil
		}
		// Symlinks to directories report IsDir()==false here because
		// WalkDir does not follow them, so the name check below naturally
		// rejects them when they are not named opencode.db. A symlink
		// named opencode.db passes the name check and is canonicalized
		// like any other candidate; the IsInsideRoot check then either
		// accepts (target inside root) or rejects (target outside root).
		if d.Name() != opencodeDBName {
			return nil
		}
		canonDB, canonErr := projectpath.Canonicalize(path)
		if canonErr != nil {
			return nil
		}
		if !projectpath.IsInsideRoot(canonDB, canonRoot) {
			return nil
		}
		instances = append(instances, SourceInstance{
			Source:      domain.SourceOpenCode,
			ProjectRoot: canonRoot,
			DBPath:      canonDB,
			Schema:      SchemaTag,
			Discovered:  a.now(),
		})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, context.Canceled) {
		return nil, walkErr
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].DBPath < instances[j].DBPath
	})
	return instances, nil
}

// depthOf returns how many nested directories a relative path contains.
// "a/b/c" -> 3, "a/b/" -> 3, "." -> 0, "" -> 0.
// Counts both '/' and the OS separator so literal forward-slash test
// inputs and filepath.Rel output (OS separator) produce the same depth.
func depthOf(rel string) int {
	if rel == "" || rel == "." {
		return 0
	}
	n := strings.Count(rel, "/") + strings.Count(rel, string(filepath.Separator))
	return n + 1
}

// locatorError maps projectpath errors onto the experience error vocabulary.
func locatorError(err error) error {
	var pathErr *projectpath.Error
	if errors.As(err, &pathErr) {
		switch pathErr.Code {
		case projectpath.ErrSymlinkEscape:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"opencode discover: project root symlink escapes the trusted root")
		case projectpath.ErrPathOutsideRoot:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"opencode discover: project root is outside the trusted root")
		}
	}
	return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
		"opencode discover: project root is invalid")
}
