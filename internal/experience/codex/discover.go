package codex

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

const maxDiscoveryDepth = 8

const (
	sessionsRel = ".codex/sessions"
	archiveRel  = ".codex/archived_sessions"
)

// Discover finds Codex rollout JSONL files under the caller's trusted root.
func (a *Adapter) Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectRoot) == "" {
		return nil, domain.NewValidationError(domain.ErrExperienceLocatorInvalid, "codex discover: project root is required")
	}
	root, err := projectpath.Canonicalize(projectRoot)
	if err != nil {
		return nil, codexLocatorError(err)
	}

	var instances []SourceInstance
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if projectpath.IsProtectedPath(path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return codexDescendDecision(root, path)
		}
		if !isRolloutName(entry.Name()) {
			return nil
		}
		rollout, err := projectpath.Canonicalize(path)
		if err != nil {
			return nil
		}
		if !projectpath.IsInsideRoot(rollout, root) {
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot, "codex discover: rollout escapes the trusted root")
		}
		instances = append(instances, SourceInstance{
			Source: domain.SourceCodex, ProjectRoot: root, RolloutPath: rollout,
			Schema: SchemaTag, Discovered: a.now(),
		})
		return nil
	})
	if err != nil {
		var validation *domain.ValidationError
		if errors.As(err, &validation) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, err
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].RolloutPath < instances[j].RolloutPath })
	return instances, nil
}

func isRolloutName(name string) bool {
	return strings.HasPrefix(name, "rollout-") && strings.EqualFold(filepath.Ext(name), ".jsonl")
}

func codexDescendDecision(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return fs.SkipDir
	}
	slash := filepath.ToSlash(rel)
	if slash == "." || slash == ".codex" || slash == sessionsRel || slash == archiveRel {
		return nil
	}
	if strings.HasPrefix(slash, sessionsRel+"/") || strings.HasPrefix(slash, archiveRel+"/") {
		if strings.Count(slash, "/")+1 > maxDiscoveryDepth {
			return fs.SkipDir
		}
		return nil
	}
	return fs.SkipDir
}

func codexLocatorError(err error) error {
	var pathErr *projectpath.Error
	if errors.As(err, &pathErr) {
		return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot, "codex discover: project root is outside the trusted root")
	}
	return domain.NewValidationError(domain.ErrExperienceLocatorInvalid, "codex discover: project root is invalid")
}
