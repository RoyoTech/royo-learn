package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
