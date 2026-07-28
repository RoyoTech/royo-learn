package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestDiscover_RequiresProjectRoot(t *testing.T) {
	_, err := NewAdapter().Discover(context.Background(), "  ")
	if domainCode(err) != domain.ErrExperienceLocatorInvalid {
		t.Fatalf("Discover(empty) error = %v, want %q", err, domain.ErrExperienceLocatorInvalid)
	}
}

func TestDiscover_FindsActiveAndArchivedRollouts(t *testing.T) {
	root := t.TempDir()
	active := writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "2026", "07", "27", "rollout-active.jsonl"))
	archived := writeDiscoveryFile(t, root, filepath.Join(".codex", "archived_sessions", "rollout-archived.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join(".codex", "session_index.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "2026", "07", "27", "notes.jsonl"))

	fixed := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return fixed }
	instances, err := adapter.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Discover returned %d instances, want 2", len(instances))
	}
	want := []string{archived, active}
	for i, instance := range instances {
		if instance.RolloutPath != want[i] {
			t.Fatalf("instance %d path = %q, want %q", i, instance.RolloutPath, want[i])
		}
		if instance.Source != domain.SourceCodex || instance.Schema != SchemaTag {
			t.Fatalf("instance %d source/schema = %q/%q", i, instance.Source, instance.Schema)
		}
		if instance.ProjectRoot != root || !instance.Discovered.Equal(fixed) {
			t.Fatalf("instance %d root/time = %q/%v", i, instance.ProjectRoot, instance.Discovered)
		}
	}
}

func TestDiscover_EmptyTreeIsDeterministic(t *testing.T) {
	root := t.TempDir()
	first, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("first Discover: %v", err)
	}
	second, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("second Discover: %v", err)
	}
	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("empty discoveries = %d/%d, want 0/0", len(first), len(second))
	}
}

func TestDiscover_DepthCap(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".codex", "sessions")
	for i := 0; i < maxDiscoveryDepth+1; i++ {
		rel = filepath.Join(rel, "deep")
	}
	writeDiscoveryFile(t, root, filepath.Join(rel, "rollout-too-deep.jsonl"))
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover returned %d deep instances, want 0", len(instances))
	}
}

func TestDiscover_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlinks require elevated privileges")
	}
	root, outside := t.TempDir(), t.TempDir()
	target := writeDiscoveryFile(t, outside, "rollout-outside.jsonl")
	link := filepath.Join(root, ".codex", "sessions", "2026", "07", "27", "rollout-link.jsonl")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := NewAdapter().Discover(context.Background(), root)
	if domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("Discover(symlink escape) error = %v, want %q", err, domain.ErrExperienceLocatorOutsideRoot)
	}
}

func TestDiscover_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Discover(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) || err != context.Canceled {
		t.Fatalf("Discover canceled error = %v, want literal context.Canceled", err)
	}
}

func writeDiscoveryFile(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

// TestDiscover_RejectsUncProjectRoot covers the Canonicalize failure
// (discover.go:31) and the codexLocatorError wrapping (discover.go:104-108)
// when the caller hands us a UNC/verbatim path that the projectpath package
// must reject.
func TestDiscover_RejectsUncProjectRoot(t *testing.T) {
	cases := []struct {
		name string
		root string
	}{
		{"unc share", `\\evil\share`},
		{"verbatim device", `\\?\C:\foo`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAdapter().Discover(context.Background(), tc.root)
			if domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
				t.Fatalf("Discover(%s) error = %v, want code %q", tc.root, err, domain.ErrExperienceLocatorOutsideRoot)
			}
		})
	}
}

// TestDiscover_SkipsNonCodexDirectory covers codexDescendDecision's default
// SkipDir (discover.go:101). Anything outside the .codex subtree must be
// left untouched, even if it contains a rollout-looking filename.
func TestDiscover_SkipsNonCodexDirectory(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFile(t, root, filepath.Join("notes", "rollout-note.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join("scratchpad", "rollout-scratch.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "rollout-real.jsonl"))

	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	if !strings.Contains(instances[0].RolloutPath, "rollout-real.jsonl") {
		t.Fatalf("Discovered unexpected rollout: %s", instances[0].RolloutPath)
	}
}

// TestDiscover_MultiProjectRootsOnlyOuterScanned covers the case where
// multiple `.codex/sessions` directories exist under nested project roots.
// The current behaviour intentionally walks only the outer root's sessions
// tree because intermediate project directories are not on the descent list.
func TestDiscover_MultiProjectRootsOnlyOuterScanned(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "rollout-outer.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join("proj-a", ".codex", "sessions", "rollout-nested-a.jsonl"))
	writeDiscoveryFile(t, root, filepath.Join("proj-b", "src", ".codex", "sessions", "rollout-nested-b.jsonl"))

	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (only outer)", len(instances))
	}
	if !strings.Contains(instances[0].RolloutPath, "rollout-outer.jsonl") {
		t.Fatalf("Discovered unexpected rollout: %s", instances[0].RolloutPath)
	}
}

// TestDiscover_SkipsProtectedDirectory covers the IsProtectedPath + IsDir
// branch in discover.go (lines 46-49): a protected directory nested inside
// the descent list must be left alone without descending.
func TestDiscover_SkipsProtectedDirectory(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".codex", "sessions", ".git", "objects")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A rollout file inside .git/objects must NOT be discovered.
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", ".git", "objects", "rollout-leak.jsonl"))
	// A sibling rollout must still be found.
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "rollout-real.jsonl"))

	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	if !strings.Contains(instances[0].RolloutPath, "rollout-real.jsonl") {
		t.Fatalf("Discovered unexpected rollout: %s", instances[0].RolloutPath)
	}
}

// TestDiscover_SkipsProtectedFile covers the IsProtectedPath on a
// non-directory file (discover.go:46 and line 50). A protected file inside
// the descent list must be dropped before the IsRolloutName check runs.
func TestDiscover_SkipsProtectedFile(t *testing.T) {
	root := t.TempDir()
	// credentials.jsonl is a protected filename even though it ends in .jsonl.
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "credentials.jsonl"))
	// .env is also protected.
	envPath := filepath.Join(root, ".codex", "sessions", ".env")
	if err := os.WriteFile(envPath, []byte("SECRET=x"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "rollout-real.jsonl"))

	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	if !strings.Contains(instances[0].RolloutPath, "rollout-real.jsonl") {
		t.Fatalf("Discovered unexpected rollout: %s", instances[0].RolloutPath)
	}
}

// TestDiscover_WalkErrorOnUnreadableDirectory covers the walkErr+IsDir
// branch (discover.go:40-43) by chmod-ing a directory under the walk path
// to 0 so ReadDir fails. Discover must surface a graceful outcome (empty
// result or wrapped error) without panicking.
func TestDiscover_WalkErrorOnUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 directory test relies on Unix file permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("chmod 0 has no effect when running as root")
	}
	root := t.TempDir()
	unreadable := filepath.Join(root, ".codex", "sessions", "private")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	// A sibling rollout should still be discoverable — the walk should skip
	// the unreadable subtree but continue elsewhere.
	writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "rollout-sibling.jsonl"))

	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		// Acceptable: graceful error from filepath.WalkDir is propagated.
		return
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (sibling only)", len(instances))
	}
}

// midWalkCancelContext is a context that defers cancellation until its
// Err() method has been called more than once. This deterministically
// forces Discover to bypass the outer ctx check (first call) and cancel
// inside the WalkDir callback (subsequent calls), exercising discover.go:37-39.
type midWalkCancelContext struct {
	base   context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	calls  int
}

func (c *midWalkCancelContext) Deadline() (time.Time, bool) { return c.base.Deadline() }
func (c *midWalkCancelContext) Done() <-chan struct{}       { return c.base.Done() }
func (c *midWalkCancelContext) Err() error {
	c.mu.Lock()
	c.calls++
	cancel := c.calls >= 2
	c.mu.Unlock()
	if cancel {
		c.cancel()
	}
	return c.base.Err()
}
func (c *midWalkCancelContext) Value(key any) any { return c.base.Value(key) }

// TestDiscover_ContextCanceledMidWalk covers the inner ctx.Err() check
// inside filepath.WalkDir (discover.go:37-39). The first Err() call
// (outer check) returns nil; the second (inside the walk) cancels.
func TestDiscover_ContextCanceledMidWalk(t *testing.T) {
	root := t.TempDir()
	// Multiple directories ensure the walk visits more than one callback.
	for i := 0; i < 8; i++ {
		sub := filepath.Join(root, ".codex", "sessions", "2026", "07", "27", "sub-"+string(rune('a'+i)))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		writeDiscoveryFile(t, root, filepath.Join(".codex", "sessions", "2026", "07", "27", "sub-"+string(rune('a'+i)), "rollout.jsonl"))
	}

	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &midWalkCancelContext{base: base, cancel: cancel}

	_, err := NewAdapter().Discover(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover(mid-walk cancel) error = %v, want context.Canceled", err)
	}
}
