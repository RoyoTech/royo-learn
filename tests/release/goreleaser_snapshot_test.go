package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGoreleaserSnapshot_SBOMEmission runs `goreleaser release --snapshot --clean`
// and asserts that the dist/ directory contains at least one *.spdx.json SBOM
// file alongside the archive artifacts.
//
// The test uses t.TempDir() for the Go build cache and t.Cleanup() to remove
// any dist/ directory goreleaser creates in the repo root, so no snapshot
// artifacts are left on the CI runner.
//
// If the goreleaser binary is not on PATH the test is skipped — it must NOT
// fail CI when goreleaser is absent.
func TestGoreleaserSnapshot_SBOMEmission(t *testing.T) {
	if _, err := exec.LookPath("goreleaser"); err != nil {
		t.Skip("goreleaser binary not on PATH; skipping SBOM snapshot test")
	}

	// t.TempDir() is auto-removed by the testing framework. We use it for the
	// Go build cache so goreleaser's `go build` invocations do not pollute the
	// developer's GOCACHE.
	tmpDir := t.TempDir()

	root := repoRoot(t)

	// Safety net: goreleaser writes to ./dist relative to the project root.
	// --clean removes a pre-existing dist/ first, and t.Cleanup removes whatever
	// goreleaser produced so the working tree stays clean.
	distDir := filepath.Join(root, "dist")
	t.Cleanup(func() {
		_ = os.RemoveAll(distDir)
	})

	cmd := exec.Command("goreleaser", "release", "--snapshot", "--clean")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOCACHE="+tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("goreleaser release --snapshot --clean failed: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(distDir, "*.spdx.json"))
	if err != nil {
		t.Fatalf("glob for *.spdx.json in %s: %v", distDir, err)
	}
	if len(matches) == 0 {
		t.Errorf("expected at least one *.spdx.json SBOM file in %s, found none", distDir)
	}
}

// repoRoot walks up from the current working directory until it finds a
// directory containing go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod walking up from test directory")
		}
		dir = parent
	}
}
