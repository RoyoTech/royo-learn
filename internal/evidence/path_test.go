package evidence

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"agent-royo-learn/internal/project"
)

func TestResolvePathRejectsMaliciousInputs(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{"parent traversal slash", "../escape.txt"},
		{"parent traversal backslash", `..\escape.txt`},
		{"embedded traversal", "safe/../file.txt"},
		{"NUL injection", "safe\x00.txt"},
		{"UNC path", `\\server\share\file.txt`},
		{"device path", `\\.\CON`},
		{"verbatim path", `\\?\C:\safe\file.txt`},
		{"Windows drive relative", `C:escape.txt`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResolvePath(root, tt.path); err == nil {
				t.Fatalf("ResolvePath(%q) succeeded, want rejection", tt.path)
			}
		})
	}

	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if _, err := ResolvePath(root, outside); err == nil {
		t.Fatal("ResolvePath accepted absolute path outside root")
	}
}

func TestResolvePathAcceptsCanonicalUnicodePathsInsideRoot(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := project.Canonicalize(root)
	if err != nil {
		t.Fatalf("canonicalize root: %v", err)
	}
	unicodePath := filepath.Join("evidencia", "café-日本語.txt")
	got, err := ResolvePath(root, unicodePath)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := filepath.Join(canonicalRoot, unicodePath)
	if got != want {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}

	abs, err := ResolvePath(root, want)
	if err != nil || abs != want {
		t.Fatalf("valid absolute path inside root = %q, %v; want %q", abs, err, want)
	}
}

func TestResolvePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := ResolvePath(root, filepath.Join("link", "new.txt")); err == nil {
		t.Fatal("ResolvePath accepted a path escaping through a symlink")
	}
}
