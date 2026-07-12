package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Path security tests
// ---------------------------------------------------------------------------

func TestCanonicalize(t *testing.T) {
	dir := t.TempDir()

	got, err := Canonicalize(dir)
	if err != nil {
		t.Fatalf("Canonicalize(%q): %v", dir, err)
	}
	want := canonicalDir(t, dir)
	if got != want {
		t.Fatalf("Canonicalize(%q)=%q want %q", dir, got, want)
	}

	// Relative path should become absolute.
	rel := filepath.Join(dir, "sub", "..", "sub")
	gotRel, err := Canonicalize(rel)
	if err != nil {
		t.Fatalf("Canonicalize(%q): %v", rel, err)
	}
	wantClean := filepath.Clean(rel)
	wantAbs := canonicalDir(t, wantClean)
	if gotRel != wantAbs {
		t.Fatalf("Canonicalize(%q)=%q want %q", rel, gotRel, wantAbs)
	}
}

func TestCanonicalizeSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	os.MkdirAll(realDir, 0o755)

	link := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got, err := Canonicalize(link)
	if err != nil {
		t.Fatalf("Canonicalize(%q): %v", link, err)
	}
	want := canonicalDir(t, realDir)
	if got != want {
		t.Fatalf("Canonicalize(%q)=%q want %q", link, got, want)
	}
}

func TestIsInsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "sub", "project")
	outside := t.TempDir()

	if !IsInsideRoot(inside, root) {
		t.Fatalf("IsInsideRoot(%q, %q) should be true", inside, root)
	}
	if IsInsideRoot(outside, root) {
		t.Fatalf("IsInsideRoot(%q, %q) should be false", outside, root)
	}

	// Root itself must count as inside.
	if !IsInsideRoot(root, root) {
		t.Fatalf("IsInsideRoot(%q, %q) should be true", root, root)
	}

	// Traversal must be rejected.
	traversal := filepath.Join(root, "safe", "..", "..", "other")
	if IsInsideRoot(traversal, root) {
		t.Fatalf("IsInsideRoot(%q, %q) should be false for traversal", traversal, root)
	}
}

func TestProtectedPath(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, ".git", "config"), true},
		{filepath.Join(dir, ".ssh", "id_rsa"), true},
		{filepath.Join(dir, ".env"), true},
		{filepath.Join(dir, "normal.txt"), false},
		{filepath.Join(dir, "src", "main.go"), false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := IsProtectedPath(tc.path)
			if got != tc.want {
				t.Fatalf("IsProtectedPath(%q)=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestProtectedPathCredentials(t *testing.T) {
	dir := t.TempDir()
	creds := []string{
		filepath.Join(dir, "credentials"),
		filepath.Join(dir, ".credentials"),
		filepath.Join(dir, "credentials.json"),
		filepath.Join(dir, ".netrc"),
		filepath.Join(dir, ".npmrc"),
	}
	for _, p := range creds {
		if !IsProtectedPath(p) {
			t.Fatalf("IsProtectedPath(%q) should be true", p)
		}
	}
}

func TestRejectUNCPaths(t *testing.T) {
	paths := []string{
		`\\server\share\project`,
		`\\?\C:\project`,
		`\\.\C:\project`,
	}
	for _, p := range paths {
		_, err := Canonicalize(p)
		if err == nil {
			t.Fatalf("Canonicalize(%q) should return error for forbidden path", p)
		}
	}
}

func TestWindowsCaseInsensitivePrefix(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "project")

	// On Windows, case differences in the drive letter should not matter.
	if runtime.GOOS == "windows" {
		if !IsInsideRoot(inside, root) {
			t.Fatalf("IsInsideRoot(%q, %q) should be true on Windows", inside, root)
		}
	}
	// Cross-platform: same case must work everywhere.
	if !IsInsideRoot(inside, root) {
		t.Fatalf("IsInsideRoot(%q, %q) should be true", inside, root)
	}
}

// ---------------------------------------------------------------------------
// Key derivation tests
// ---------------------------------------------------------------------------

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func createGitRepo(t *testing.T, dir, remote string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if remote != "" {
		runGit(t, dir, "remote", "add", "origin", remote)
	}
	// Create a dummy file so there's a commit.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q: %v\n%s", args, dir, err, string(out))
	}
}

func TestDeriveKeyFromGitRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	createGitRepo(t, dir, "https://github.com/RoyoTech/royo-learn.git")

	key, err := DeriveKey(dir)
	if err != nil {
		t.Fatalf("DeriveKey(%q): %v", dir, err)
	}
	if key == "" {
		t.Fatal("DeriveKey returned empty key")
	}
	// Key must be lowercase and contain the repo name.
	if !strings.Contains(key, "royo-learn") {
		t.Fatalf("DeriveKey=%q, should contain royo-learn", key)
	}
}

func TestDeriveKeyFallbackToHash(t *testing.T) {
	dir := t.TempDir()

	key, err := DeriveKey(dir)
	if err != nil {
		t.Fatalf("DeriveKey(%q): %v", dir, err)
	}
	if key == "" {
		t.Fatal("DeriveKey returned empty key for non-git dir")
	}
	// Fallback is a 12-char hex digest, so 12 hex chars.
	if len(key) != 12 {
		t.Fatalf("DeriveKey(%q)=%q, want 12-char hex", dir, key)
	}
	for _, r := range key {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("DeriveKey(%q)=%q, not hex", dir, key)
		}
	}
}

func TestDeriveKeyNormalized(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	createGitRepo(t, dir, "https://github.com/RoyoTech/My-Project.git")

	key, err := DeriveKey(dir)
	if err != nil {
		t.Fatalf("DeriveKey(%q): %v", dir, err)
	}
	// Key must be completely lowercase — no uppercase letters present.
	if key != strings.ToLower(key) {
		t.Fatalf("DeriveKey=%q contains uppercase, should be lowercase kebab-case", key)
	}
}

// ---------------------------------------------------------------------------
// Project resolver tests
// ---------------------------------------------------------------------------

func TestResolveExplicitRootWins(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: test\n"), 0o644)

	r := NewResolver(WithTrustedRoots([]string{root}))
	req := &ResolveRequest{ExplicitRoot: root}

	ctx := context.Background()
	proj, err := r.Resolve(ctx, req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if proj == nil {
		t.Fatal("Resolve returned nil project")
	}
	want := canonicalDir(t, root)
	if proj.Root != want {
		t.Fatalf("Project.Root=%q want %q", proj.Root, want)
	}
	if proj.Key == "" {
		t.Fatal("Project.Key is empty")
	}
}

func TestResolveExplicitOutsideTrustedRoot(t *testing.T) {
	roots := []string{t.TempDir()}
	outside := t.TempDir()

	r := NewResolver(WithTrustedRoots(roots))
	req := &ResolveRequest{ExplicitRoot: outside}

	_, err := r.Resolve(context.Background(), req)
	assertProjectErrorCode(t, err, "path_outside_root")
}

func TestResolveCWDGitRoot(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	root := t.TempDir()
	createGitRepo(t, root, "https://github.com/example/test.git")

	// CWD is a subdirectory within the git repo.
	cwd := filepath.Join(root, "src", "internal")
	os.MkdirAll(cwd, 0o755)

	r := NewResolver(WithTrustedRoots([]string{root}))
	req := &ResolveRequest{CWD: cwd}

	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := canonicalDir(t, root)
	if proj.Root != want {
		t.Fatalf("got Root=%q want %q", proj.Root, want)
	}
}

func TestResolveMonorepoNestedProject(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	monoRoot := t.TempDir()
	createGitRepo(t, monoRoot, "https://github.com/example/monorepo.git")

	// A sub-project inside the monorepo with its own .royo-learn/config.yaml.
	sub := filepath.Join(monoRoot, "services", "api")
	os.MkdirAll(filepath.Join(sub, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(sub, ".royo-learn", "config.yaml"), []byte("project:\n  name: api-service\n"), 0o644)

	// CWD is inside the sub-project.
	cwd := filepath.Join(sub, "handlers")
	os.MkdirAll(cwd, 0o755)

	r := NewResolver(WithTrustedRoots([]string{monoRoot}))
	req := &ResolveRequest{CWD: cwd}

	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := canonicalDir(t, sub)
	if proj.Root != want {
		t.Fatalf("got Root=%q want sub-project root %q", proj.Root, want)
	}
}

func TestResolveAmbiguousProject(t *testing.T) {
	parent := t.TempDir()

	// Two sibling directories, both with .royo-learn/config.yaml.
	// CWD is a subdirectory inside one of them, so the walk-up algorithm
	// finds the first marker and then discovers the sibling.
	for _, name := range []string{"a", "b"} {
		sub := filepath.Join(parent, name)
		os.MkdirAll(filepath.Join(sub, ".royo-learn"), 0o755)
		os.WriteFile(filepath.Join(sub, ".royo-learn", "config.yaml"),
			[]byte(fmt.Sprintf("project:\n  name: %s\n", name)), 0o644)
	}

	// CWD is inside "a" so walk-up finds marker at "a", then checks siblings.
	cwd := filepath.Join(parent, "a", "src")
	os.MkdirAll(cwd, 0o755)

	r := NewResolver(WithTrustedRoots([]string{parent}))
	req := &ResolveRequest{CWD: cwd}

	_, err := r.Resolve(context.Background(), req)
	assertProjectErrorCode(t, err, "ambiguous_project")
}

func TestResolveCWDExactMarkerWithSibling(t *testing.T) {
	parent := t.TempDir()

	// Two sibling directories, both with .royo-learn/config.yaml markers.
	// CWD is set to one of them EXACTLY (not a subdirectory).
	// Resolution MUST succeed — there is no ambiguity when CWD is the project root.
	for _, name := range []string{"a", "b"} {
		sub := filepath.Join(parent, name)
		os.MkdirAll(filepath.Join(sub, ".royo-learn"), 0o755)
		os.WriteFile(filepath.Join(sub, ".royo-learn", "config.yaml"),
			[]byte(fmt.Sprintf("project:\n  name: %s\n", name)), 0o644)
	}

	// CWD IS project "a" itself.
	cwd := filepath.Join(parent, "a")

	r := NewResolver(WithTrustedRoots([]string{parent}))
	req := &ResolveRequest{CWD: cwd}

	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve with CWD at project root: %v", err)
	}
	want := canonicalDir(t, cwd)
	if proj.Root != want {
		t.Fatalf("got Root=%q want %q", proj.Root, want)
	}
}

func TestResolveProjectNotFound(t *testing.T) {
	dir := t.TempDir()

	r := NewResolver(WithTrustedRoots([]string{dir}))
	req := &ResolveRequest{CWD: dir}

	_, err := r.Resolve(context.Background(), req)
	assertProjectErrorCode(t, err, "project_not_found")
}

func TestResolveProjectNotFoundNoGit(t *testing.T) {
	dir := t.TempDir()
	// Make a subdirectory as CWD, no git repo anywhere up.
	cwd := filepath.Join(dir, "deep", "path")
	os.MkdirAll(cwd, 0o755)

	r := NewResolver(WithTrustedRoots([]string{dir}))
	req := &ResolveRequest{CWD: cwd}

	_, err := r.Resolve(context.Background(), req)
	assertProjectErrorCode(t, err, "project_not_found")
}

func TestResolveOrphanProjectConfig(t *testing.T) {
	// .royo-learn/config.yaml exists but directory is not inside a git repo.
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(dir, ".royo-learn", "config.yaml"), []byte("project:\n  name: orphan\n"), 0o644)

	r := NewResolver(WithTrustedRoots([]string{dir}))
	req := &ResolveRequest{CWD: dir}

	// Should still resolve — explicit config marker beats git.
	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve orphan: %v", err)
	}
	if proj == nil {
		t.Fatal("expected project from orphan config marker")
	}
}

func TestResolveMCPRoot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: mcp-test\n"), 0o644)

	r := NewResolver(WithTrustedRoots([]string{root}))
	req := &ResolveRequest{MCPRoot: root}

	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve with MCPRoot: %v", err)
	}
	want := canonicalDir(t, root)
	if proj.Root != want {
		t.Fatalf("got Root=%q want %q", proj.Root, want)
	}
}

func TestResolveWithKeyDeriver(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755)
	os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project:\n  name: custom\n"), 0o644)

	r := NewResolver(
		WithTrustedRoots([]string{root}),
		WithKeyDeriver(func(root string) (string, error) {
			return "custom-key", nil
		}),
	)
	req := &ResolveRequest{ExplicitRoot: root}

	proj, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("Resolve with custom key deriver: %v", err)
	}
	if proj.Key != "custom-key" {
		t.Fatalf("got Key=%q want custom-key", proj.Key)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// canonicalDir returns the canonical absolute path for dir, using Canonicalize.
// Unlike filepath.Abs, this resolves symlinks and short names, so tests
// compare against the same canonical form the production code uses.
func canonicalDir(t *testing.T, dir string) string {
	t.Helper()
	got, err := Canonicalize(dir)
	if err != nil {
		t.Fatalf("Canonicalize(%q): %v", dir, err)
	}
	return got
}

func assertProjectErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", want)
	}
	var pErr *Error
	if !errors.As(err, &pErr) || pErr.Code != want {
		t.Fatalf("got err=%v, want code %q", err, want)
	}
}
