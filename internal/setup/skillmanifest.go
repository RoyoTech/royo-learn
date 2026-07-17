package setup

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ManagedByRoyoLearn is the marker written into every skill manifest that
// royo-learn installs. The upgrade path refuses to touch any installed skill
// whose manifest is missing or carries a different marker.
const ManagedByRoyoLearn = "royo-learn"

// royoLearnMgmtDir is the hidden directory, created next to the installed
// skills, where royo-learn keeps its manifests, backups, staging areas,
// candidates, and conflict records. It never contains a SKILL.md, so agents
// (and InstallSkills, which iterates the source tree) never treat it as a
// skill.
const royoLearnMgmtDir = ".royo-learn"

// SkillManifest records what royo-learn installed for a single skill so that a
// later binary can decide, safely, whether the installed copy may be upgraded.
type SkillManifest struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	SourceSHA256    string `json:"source_sha256"`
	InstalledSHA256 string `json:"installed_sha256"`
	ManagedBy       string `json:"managed_by"`
}

func mgmtDir(dstDir string) string       { return filepath.Join(dstDir, royoLearnMgmtDir) }
func manifestsDir(dstDir string) string  { return filepath.Join(mgmtDir(dstDir), "manifests") }
func backupsDir(dstDir string) string    { return filepath.Join(mgmtDir(dstDir), "backups") }
func stagingDir(dstDir string) string    { return filepath.Join(mgmtDir(dstDir), "staging") }
func candidatesDir(dstDir string) string { return filepath.Join(mgmtDir(dstDir), "candidates") }
func conflictsDir(dstDir string) string  { return filepath.Join(mgmtDir(dstDir), "conflicts") }

func manifestPath(dstDir, name string) string {
	return filepath.Join(manifestsDir(dstDir), name+".json")
}

// WriteSkillManifest persists the manifest for a skill under the management
// directory next to the installed skills.
func WriteSkillManifest(dstDir string, m SkillManifest) error {
	if err := os.MkdirAll(manifestsDir(dstDir), 0o755); err != nil {
		return fmt.Errorf("setup: cannot create manifests dir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("setup: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(manifestPath(dstDir, m.Name), data, 0o644); err != nil {
		return fmt.Errorf("setup: write manifest: %w", err)
	}
	return nil
}

// ReadSkillManifest loads the manifest for a skill. A missing manifest returns
// os.ErrNotExist so callers can distinguish "unmanaged" from a real failure.
func ReadSkillManifest(dstDir, name string) (*SkillManifest, error) {
	data, err := os.ReadFile(manifestPath(dstDir, name))
	if err != nil {
		return nil, err
	}
	var m SkillManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("setup: manifest %q is corrupt: %w", name, err)
	}
	return &m, nil
}

// HashSkillDir computes a deterministic content hash over the skill files in
// dir. Only files that InstallSkills would copy (isSkillFile) are hashed, so a
// hash taken over the source and a hash taken over a freshly-installed copy are
// identical. Relative paths are normalized to forward slashes for
// cross-platform stability.
func HashSkillDir(dir string) (string, error) {
	type fileHash struct{ rel, hash string }
	var files []fileHash

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !isSkillFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, fileHash{
			rel:  filepath.ToSlash(rel),
			hash: fmt.Sprintf("%x", sum),
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("setup: hash skill dir %q: %w", dir, err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\n", f.rel, f.hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

var skillVersionRe = regexp.MustCompile(`(?m)^\s*version:\s*"?([^"\r\n]+?)"?\s*$`)

// SkillVersion extracts the declared version from a skill's SKILL.md
// frontmatter. It returns "unknown" when no version line is present.
func SkillVersion(skillDir string) string {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "unknown"
	}
	if m := skillVersionRe.FindSubmatch(data); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return "unknown"
}

// hasSkillMarker reports whether dir looks like an installed skill (has a
// SKILL.md).
func hasSkillMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil
}

func timestamp() string { return time.Now().UTC().Format("20060102150405.000000000") }
