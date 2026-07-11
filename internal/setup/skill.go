package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SkillInstallResult reports the outcome of a skill installation.
type SkillInstallResult struct {
	Installed int
	Skipped   int
	Errors    []string
}

// InstallSkills copies project skills from the source directory to the
// target agent skills directory. Each subdirectory under srcDir containing
// a SKILL.md is treated as a skill. Existing skills are skipped (never
// overwritten).
func InstallSkills(srcDir, dstDir string) (*SkillInstallResult, error) {
	result := &SkillInstallResult{}

	if _, err := os.Stat(srcDir); err != nil {
		return result, nil // no skills to install
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("setup: cannot read skills dir %q: %w", srcDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillDir := filepath.Join(srcDir, skillName)
		dstSkillDir := filepath.Join(dstDir, skillName)

		// Verify source has SKILL.md.
		srcSkill := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(srcSkill); err != nil {
			continue // not a valid skill dir
		}

		// Skip if destination already exists.
		dstSkill := filepath.Join(dstSkillDir, "SKILL.md")
		if _, err := os.Stat(dstSkill); err == nil {
			result.Skipped++
			continue
		}

		// Copy the entire skill directory.
		if err := copyDir(skillDir, dstSkillDir); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("failed to install %q: %v", skillName, err))
			continue
		}
		result.Installed++
	}

	return result, nil
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		// Skip non-SKILL and non-tool files for safety.
		name := entry.Name()
		if !isSkillFile(name) {
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

// isSkillFile returns true for files that are part of a skill.
func isSkillFile(name string) bool {
	return strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".json") ||
		strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") ||
		strings.HasSuffix(name, ".txt") || name == "LICENSE"
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		os.Remove(dst)
		return err
	}

	// Copy file mode.
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}
