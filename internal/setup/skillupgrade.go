package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Skill upgrade action statuses.
const (
	// UpgradeUpToDate means the installed copy already equals the source.
	UpgradeUpToDate = "up_to_date"
	// UpgradeClean means the installed copy is untouched and can be upgraded
	// (backed up, then replaced). In --dry-run it is only reported.
	UpgradeClean = "upgrade"
	// UpgradeConflict means the user modified the installed copy; it is NOT
	// overwritten. A candidate is written alongside and a conflict recorded.
	UpgradeConflict = "conflict"
	// UpgradeUnmanaged means the installed skill has no royo-learn manifest;
	// it is never touched.
	UpgradeUnmanaged = "unmanaged"
	// UpgradeAbsent means the source skill is not installed at the destination.
	UpgradeAbsent = "absent"
	// UpgradeError means the action failed.
	UpgradeError = "error"
)

// SkillUpgradeAction is the outcome for a single skill.
type SkillUpgradeAction struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	Backup      string `json:"backup,omitempty"`
	Candidate   string `json:"candidate,omitempty"`
	Diff        string `json:"diff,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// SkillUpgradeResult aggregates the outcome of an upgrade pass.
type SkillUpgradeResult struct {
	Applied bool                 `json:"applied"`
	Actions []SkillUpgradeAction `json:"actions"`
	Errors  []string             `json:"errors,omitempty"`
}

type conflictRecord struct {
	Name             string `json:"name"`
	DetectedAt       string `json:"detected_at"`
	InstalledSHA256  string `json:"installed_sha256"`
	SourceSHA256     string `json:"source_sha256"`
	CandidateVersion string `json:"candidate_version"`
	CandidatePath    string `json:"candidate_path"`
	Diff             string `json:"diff"`
}

// UpgradeSkills evaluates every source skill against the installed copies under
// dstDir and, when apply is true, safely upgrades those that are untouched
// while preserving user-modified copies byte-for-byte.
//
// Policy (from the recovery plan, Recorrido F):
//   - installed hash == manifest installed_sha256 (untouched) → back up,
//     update, record the new version.
//   - installed hash != manifest (user-modified) → do NOT overwrite; create a
//     candidate alongside, show a diff, record a conflict.
//   - no royo-learn manifest (managed_by != royo-learn) → do not touch it.
//
// apply == false is a pure report (dry-run) that writes nothing.
func UpgradeSkills(srcDir, dstDir string, apply bool) (*SkillUpgradeResult, error) {
	result := &SkillUpgradeResult{Applied: apply}

	if _, err := os.Stat(srcDir); err != nil {
		return result, nil // no source skills to upgrade from
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("setup: cannot read skills dir %q: %w", srcDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		srcSkillDir := filepath.Join(srcDir, name)
		if !hasSkillMarker(srcSkillDir) {
			continue
		}
		action := upgradeOne(srcSkillDir, dstDir, name, apply)
		if action.Status == UpgradeError {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", name, action.Detail))
		}
		result.Actions = append(result.Actions, action)
	}
	return result, nil
}

func upgradeOne(srcSkillDir, dstDir, name string, apply bool) SkillUpgradeAction {
	a := SkillUpgradeAction{Name: name}
	dstSkillDir := filepath.Join(dstDir, name)

	srcHash, err := HashSkillDir(srcSkillDir)
	if err != nil {
		a.Status = UpgradeError
		a.Detail = err.Error()
		return a
	}
	a.ToVersion = SkillVersion(srcSkillDir)

	// Not installed → nothing to upgrade (install handles first-time install).
	if !hasSkillMarker(dstSkillDir) {
		a.Status = UpgradeAbsent
		a.Detail = "skill is not installed at the destination"
		return a
	}

	man, err := ReadSkillManifest(dstDir, name)
	if err != nil || man == nil || man.ManagedBy != ManagedByRoyoLearn {
		a.Status = UpgradeUnmanaged
		a.Detail = "no royo-learn manifest; the installed skill is not managed by royo-learn"
		return a
	}
	a.FromVersion = man.Version

	instHash, err := HashSkillDir(dstSkillDir)
	if err != nil {
		a.Status = UpgradeError
		a.Detail = err.Error()
		return a
	}

	// Already current.
	if instHash == srcHash {
		a.Status = UpgradeUpToDate
		return a
	}

	// Untouched since we last wrote it → safe to upgrade.
	if instHash == man.InstalledSHA256 {
		a.Status = UpgradeClean
		if apply {
			backup, err := performUpgrade(srcSkillDir, dstSkillDir, dstDir, name, a.ToVersion, srcHash)
			a.Backup = backup
			if err != nil {
				a.Status = UpgradeError
				a.Detail = err.Error()
			}
		}
		return a
	}

	// User modified the installed copy → never overwrite it.
	a.Status = UpgradeConflict
	a.Diff = skillDiff(dstSkillDir, srcSkillDir)
	a.Detail = "installed skill was modified by the user; not overwritten"
	if apply {
		candidate, err := writeCandidate(srcSkillDir, dstDir, name, a.ToVersion, instHash, srcHash, a.Diff)
		a.Candidate = candidate
		if err != nil {
			a.Status = UpgradeError
			a.Detail = err.Error()
		}
	}
	return a
}

// performUpgrade replaces the installed skill with the source, atomically and
// recoverably: it stages the new version off to the side, backs up the current
// copy before any overwrite, then swaps. If the swap fails after the original
// was removed, it restores the backup so no half-written skill remains.
// Returns the backup path (a recovery handle) whenever one was created.
func performUpgrade(srcSkillDir, dstSkillDir, dstDir, name, version, srcHash string) (string, error) {
	ts := timestamp()

	// 1. Build the new version off to the side. A failure here never touches
	//    the installed copy.
	staging := filepath.Join(stagingDir(dstDir), name+"-"+ts)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", fmt.Errorf("stage skill %q: %w", name, err)
	}
	defer os.RemoveAll(staging)
	if err := copyDir(srcSkillDir, staging); err != nil {
		return "", fmt.Errorf("stage skill %q: %w", name, err)
	}

	// 2. Back up the current installed copy BEFORE any overwrite.
	if err := os.MkdirAll(backupsDir(dstDir), 0o755); err != nil {
		return "", fmt.Errorf("backup skill %q: %w", name, err)
	}
	backup := filepath.Join(backupsDir(dstDir), name+"-"+ts)
	if err := copyDir(dstSkillDir, backup); err != nil {
		return "", fmt.Errorf("backup skill %q: %w", name, err)
	}

	// 3. Swap: remove the original, then materialize the staged version.
	if err := os.RemoveAll(dstSkillDir); err != nil {
		return backup, fmt.Errorf("swap skill %q: %w", name, err)
	}
	if err := copyDir(staging, dstSkillDir); err != nil {
		// Roll back: restore the exact backup.
		_ = os.RemoveAll(dstSkillDir)
		if rbErr := copyDir(backup, dstSkillDir); rbErr != nil {
			return backup, fmt.Errorf("upgrade of %q failed and rollback failed (%v); restore manually from %s: %w", name, rbErr, backup, err)
		}
		return backup, fmt.Errorf("upgrade of %q failed, rolled back from %s: %w", name, backup, err)
	}

	// 4. Record the new manifest: installed now equals source.
	m := SkillManifest{
		Name:            name,
		Version:         version,
		SourceSHA256:    srcHash,
		InstalledSHA256: srcHash,
		ManagedBy:       ManagedByRoyoLearn,
	}
	if err := WriteSkillManifest(dstDir, m); err != nil {
		return backup, fmt.Errorf("upgrade of %q wrote files but the manifest failed: %w", name, err)
	}
	return backup, nil
}

// writeCandidate copies the source skill into a candidate location (never over
// the user's installed copy) and records a conflict for later review.
func writeCandidate(srcSkillDir, dstDir, name, version, instHash, srcHash, diff string) (string, error) {
	candidate := filepath.Join(candidatesDir(dstDir), name)
	if err := os.RemoveAll(candidate); err != nil {
		return "", fmt.Errorf("prepare candidate for %q: %w", name, err)
	}
	if err := copyDir(srcSkillDir, candidate); err != nil {
		return "", fmt.Errorf("write candidate for %q: %w", name, err)
	}

	if err := os.MkdirAll(conflictsDir(dstDir), 0o755); err != nil {
		return candidate, fmt.Errorf("record conflict for %q: %w", name, err)
	}
	rec := conflictRecord{
		Name:             name,
		DetectedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		InstalledSHA256:  instHash,
		SourceSHA256:     srcHash,
		CandidateVersion: version,
		CandidatePath:    candidate,
		Diff:             diff,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return candidate, fmt.Errorf("record conflict for %q: %w", name, err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(filepath.Join(conflictsDir(dstDir), name+".json"), data, 0o644); err != nil {
		return candidate, fmt.Errorf("record conflict for %q: %w", name, err)
	}
	return candidate, nil
}

// skillDiff returns a compact line diff of the two SKILL.md files.
func skillDiff(installedDir, sourceDir string) string {
	inst, _ := os.ReadFile(filepath.Join(installedDir, "SKILL.md"))
	src, _ := os.ReadFile(filepath.Join(sourceDir, "SKILL.md"))
	return lineDiff("SKILL.md", inst, src)
}

// lineDiff produces a minimal, deterministic line-by-line diff. It is intended
// for human review of an upgrade conflict, not for patching.
func lineDiff(path string, current, proposed []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- installed/%s\n", path)
	fmt.Fprintf(&b, "+++ candidate/%s\n", path)

	cur := strings.Split(string(current), "\n")
	prop := strings.Split(string(proposed), "\n")
	max := len(cur)
	if len(prop) > max {
		max = len(prop)
	}
	for i := 0; i < max; i++ {
		var c, p string
		if i < len(cur) {
			c = cur[i]
		}
		if i < len(prop) {
			p = prop[i]
		}
		if c == p {
			fmt.Fprintf(&b, " %s\n", c)
			continue
		}
		if i < len(cur) {
			fmt.Fprintf(&b, "-%s\n", c)
		}
		if i < len(prop) {
			fmt.Fprintf(&b, "+%s\n", p)
		}
	}
	return b.String()
}
