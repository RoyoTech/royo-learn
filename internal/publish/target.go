package publish

import (
	"os"
	"path/filepath"
	"strings"

	"agent-royo-learn/internal/domain"
)

// TargetContext holds additional context for skill target resolution.
type TargetContext struct {
	ProjectKey     string
	NeedAgentsHook bool
	Area           string // resolved skill area (explicit or derived)
}

// ResolveTarget determines where a learning would be published based on its
// curation destination and scope. It returns the target root, relative path,
// operation type, and whether the target file currently exists.
// For DestSkill, it optionally uses TargetContext to resolve all related targets
// (child skill, index, agents hook). When ctx is nil, falls back to single-target
// for backward compatibility.
func ResolveTarget(projectRoot string, curation *domain.Curation, ctx *TargetContext) ([]TargetResolution, error) {
	if curation == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "curation is nil for target resolution")
	}

	dest := curation.Destination
	if dest == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "curation has no destination set")
	}

	var targets []TargetResolution

	switch dest.Type {
	case domain.DestSkill:
		if ctx != nil && ctx.ProjectKey != "" {
			// Multi-target resolution for skills.
			skillTargets, err := ResolveSkillPublishTargets(projectRoot, dest, ctx.ProjectKey, ctx.NeedAgentsHook)
			if err != nil {
				return nil, err
			}
			targets = skillTargets.Flatten()
		} else {
			targets = append(targets, resolveSkillTarget(projectRoot, dest))
		}
	case domain.DestAgentsRule:
		targets = append(targets, resolveAgentsTarget(projectRoot, dest))
	case domain.DestProject:
		targets = append(targets, resolveProjectTarget(projectRoot, dest))
	case domain.DestShared:
		targets = append(targets, resolveSharedTarget(projectRoot, dest))
	default:
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"unknown destination type: "+string(dest.Type))
	}

	// Validate that all targets are within managed boundaries.
	for _, t := range targets {
		if err := validateTargetPath(projectRoot, t); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

// resolveSkillTarget resolves a skill file target.
func resolveSkillTarget(projectRoot string, dest *domain.Destination) TargetResolution {
	skillPath := filepath.Join(projectRoot, dest.Root, dest.Path)
	_, err := os.Stat(skillPath)
	exists := err == nil

	op := domain.OpCreate
	if exists {
		op = domain.OpReplaceManagedBlock
	}

	return TargetResolution{
		Root:      dest.Root,
		Path:      dest.Path,
		Operation: op,
		Exists:    exists,
		IsManaged: true,
	}
}

// resolveAgentsTarget resolves an AGENTS.md target.
func resolveAgentsTarget(projectRoot string, dest *domain.Destination) TargetResolution {
	agentsPath := filepath.Join(projectRoot, dest.Root, dest.Path)
	_, err := os.Stat(agentsPath)
	exists := err == nil

	op := domain.OpCreate
	if exists {
		op = domain.OpReplaceManagedBlock
	}

	return TargetResolution{
		Root:      dest.Root,
		Path:      dest.Path,
		Operation: op,
		Exists:    exists,
		IsManaged: true,
	}
}

// resolveProjectTarget resolves a project-scoped target.
func resolveProjectTarget(projectRoot string, dest *domain.Destination) TargetResolution {
	fullPath := filepath.Join(projectRoot, dest.Root, dest.Path)
	_, err := os.Stat(fullPath)
	exists := err == nil

	return TargetResolution{
		Root:      dest.Root,
		Path:      dest.Path,
		Operation: domain.OpReplace,
		Exists:    exists,
		IsManaged: false,
	}
}

// resolveSharedTarget resolves a shared library target.
func resolveSharedTarget(projectRoot string, dest *domain.Destination) TargetResolution {
	fullPath := filepath.Join(projectRoot, dest.Root, dest.Path)
	_, err := os.Stat(fullPath)
	exists := err == nil

	return TargetResolution{
		Root:      dest.Root,
		Path:      dest.Path,
		Operation: domain.OpReplaceManagedBlock,
		Exists:    exists,
		IsManaged: true,
	}
}

// validateTargetPath ensures the resolved target is within managed boundaries.
func validateTargetPath(projectRoot string, t TargetResolution) error {
	absRoot := filepath.Join(projectRoot, t.Root)
	absPath := filepath.Join(absRoot, t.Path)

	// Check that the target path is within the project root.
	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return domain.NewValidationError(domain.ErrPathOutsideRoot,
			"cannot resolve relative path for target: "+absPath)
	}
	if strings.HasPrefix(rel, "..") {
		return domain.NewValidationError(domain.ErrPathOutsideRoot,
			"target path escapes project root: "+absPath)
	}

	// Check that the target is within the specified root.
	relToTargetRoot, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return domain.NewValidationError(domain.ErrPathOutsideRoot,
			"cannot resolve relative path for target: "+absPath)
	}
	if strings.HasPrefix(relToTargetRoot, "..") {
		return domain.NewValidationError(domain.ErrPathOutsideRoot,
			"target path escapes destination root: "+absPath)
	}

	return nil
}
