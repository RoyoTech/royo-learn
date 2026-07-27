package claudecode

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

// maxDiscoveryDepth matches the opencode adapter; bounds the worst-case
// scan cost without limiting realistic layouts.
const maxDiscoveryDepth = 8

// projectsRel is the relative path under the project root where Claude Code
// stores its session files. The adapter never decodes the encoded
// sub-directory name; the caller supplies projectRoot so IsInsideRoot can
// enforce the trust boundary.
const projectsRel = ".claude/projects"

// Discover locates Claude Code session JSONL files reachable from
// projectRoot. It walks `.claude/projects/<encoded>/<session-uuid>.jsonl`
// only and rejects any candidate whose canonical path lies outside the
// canonical project root, per docs/24 §3 T4.
func (a *Adapter) Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectRoot) == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"claudecode discover: project root is required")
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
			return descendDecision(canonRoot, path)
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".jsonl") {
			return nil
		}
		canonJSONL, canonErr := projectpath.Canonicalize(path)
		if canonErr != nil {
			return nil
		}
		if !projectpath.IsInsideRoot(canonJSONL, canonRoot) {
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"claudecode discover: session JSONL escapes the trusted root")
		}
		instances = append(instances, SourceInstance{
			Source:      domain.SourceClaudeCode,
			ProjectRoot: canonRoot,
			JSONLPath:   canonJSONL,
			Schema:      SchemaTag,
			Discovered:  a.now(),
		})
		return nil
	})
	if walkErr != nil {
		var verr *domain.ValidationError
		if errors.As(walkErr, &verr) {
			return nil, verr
		}
		if !errors.Is(walkErr, context.Canceled) {
			return nil, walkErr
		}
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].JSONLPath < instances[j].JSONLPath
	})
	return instances, nil
}

// descendDecision descends into directories under .claude/projects up to
// maxDiscoveryDepth and skips everything else at the top level.
func descendDecision(canonRoot, dir string) error {
	rel, relErr := filepath.Rel(canonRoot, dir)
	if relErr != nil {
		return fs.SkipDir
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == "." || relSlash == ".claude" || relSlash == projectsRel {
		return nil
	}
	if strings.HasPrefix(relSlash, projectsRel+"/") {
		inside := strings.TrimPrefix(relSlash, projectsRel+"/")
		if strings.Count(inside, "/")+1 > maxDiscoveryDepth {
			return fs.SkipDir
		}
		return nil
	}
	return fs.SkipDir
}

// locatorError maps projectpath errors onto the experience error vocabulary.
func locatorError(err error) error {
	var pathErr *projectpath.Error
	if errors.As(err, &pathErr) {
		switch pathErr.Code {
		case projectpath.ErrSymlinkEscape:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"claudecode discover: project root symlink escapes the trusted root")
		case projectpath.ErrPathOutsideRoot:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"claudecode discover: project root is outside the trusted root")
		}
	}
	return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
		"claudecode discover: project root is invalid")
}
