package buildinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// One version, one source (plan §Tramo 5: "sin versiones escritas a mano en
// múltiples archivos").
//
// The product version of record is buildinfo.Version, injected at build time
// from the Git tag. Nothing else may declare it. A semver copied into a second
// file is a second source of truth, and the two drift the first time only one
// of them is updated — which is how a binary ends up reporting a version it is
// not.
// ---------------------------------------------------------------------------

// TestVersionSource_NoHardcodedVersionInSource proves no source file bakes in a
// version: an ordinary build with no ldflags reports the neutral placeholder.
// If someone hardcodes a semver in the var block, this fails.
func TestVersionSource_NoHardcodedVersionInSource(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q in a build with no ldflags, want \"dev\": "+
			"the version is baked into the source instead of injected from the Git tag", Version)
	}
	if Commit != "unknown" || BuildDate != "unknown" {
		t.Fatalf("Commit = %q, BuildDate = %q in a build with no ldflags, want \"unknown\": "+
			"build metadata is baked into the source instead of injected", Commit, BuildDate)
	}
}

// TestVersionSource_ReleaseInjectsBuildinfo proves the release build actually
// feeds that single source. Without these ldflags the released binary would
// silently report "dev".
func TestVersionSource_ReleaseInjectsBuildinfo(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}
	config := string(raw)

	for _, want := range []string{
		"-X agent-royo-learn/internal/buildinfo.Version=",
		"-X agent-royo-learn/internal/buildinfo.Commit=",
		"-X agent-royo-learn/internal/buildinfo.BuildDate=",
	} {
		if !strings.Contains(config, want) {
			t.Errorf(".goreleaser.yml does not inject %q; a released binary would report the placeholder", want)
		}
	}
}

// semverAssignment matches a Go string assignment of a bare semver to any
// identifier that ends in "version" — `Version = "0.1.9"`, but also
// `productVersion := "0.1.9"`, which is the same second source wearing a
// longer name.
var semverAssignment = regexp.MustCompile(`(?i)\b[a-z_][a-z_0-9]*version\s*:?=\s*"v?\d+\.\d+\.\d+"`)

// TestVersionSource_NoSecondVersionDeclaration scans production Go code for a
// second declaration of the version. Deprecation notices that merely MENTION a
// version in a sentence ("removed in v0.2.0") are not declarations and are left
// alone — what this forbids is another variable claiming to BE the version.
func TestVersionSource_NoSecondVersionDeclaration(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", ".codegraph", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if m := semverAssignment.FindString(trimmed); m != "" {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s declares a second version of record: %s\n"+
					"The version comes from the Git tag through buildinfo.Version; "+
					"a copy here is a second source of truth that will drift.", rel, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
