package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// fixtureTree builds a directory tree from the description and returns its
// root. Files with content "FILE" are created as regular files; entries with
// content "DIR" are created as directories; entries with content "SYMLINK"
// followed by a "<-target" suffix are created as symlinks pointing at target.
func fixtureTree(t *testing.T, description map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, kind := range description {
		full := filepath.Join(root, rel)
		switch {
		case kind == "DIR":
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
		case kind == "FILE":
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatalf("mkdir parent %s: %v", full, err)
			}
			if err := os.WriteFile(full, []byte("fixture"), 0o600); err != nil {
				t.Fatalf("write %s: %v", full, err)
			}
		default:
			if target, ok := splitSymlinkSpec(kind); ok {
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("mkdir parent %s: %v", full, err)
				}
				if err := os.Symlink(target, full); err != nil {
					t.Fatalf("symlink %s -> %s: %v", full, target, err)
				}
			} else {
				t.Fatalf("unknown fixture kind %q for %s", kind, rel)
			}
		}
	}
	return root
}

func splitSymlinkSpec(kind string) (string, bool) {
	const prefix = "SYMLINK<-"
	if len(kind) > len(prefix) && kind[:len(prefix)] == prefix {
		return kind[len(prefix):], true
	}
	return "", false
}

func sortedDBPaths(instances []SourceInstance) []string {
	paths := make([]string, 0, len(instances))
	for _, inst := range instances {
		paths = append(paths, filepath.ToSlash(inst.DBPath))
	}
	sort.Strings(paths)
	return paths
}

// TestDiscover_EmptyProjectRoot rejects a missing root with a typed error.
// Discover must never silently invent a project root.
func TestDiscover_EmptyProjectRoot(t *testing.T) {
	if _, err := NewAdapter().Discover(context.Background(), ""); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("Discover(empty) error = %v, want invalid_argument", err)
	}
}

// TestDiscover_NonexistentRootReturnsEmpty verifies that a project root that
// simply does not exist is reported as "no instances" rather than an error.
// The caller is the one responsible for verifying the project exists; the
// adapter does not pretend to know.
func TestDiscover_NonexistentRootReturnsEmpty(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "definitely-missing-project-root")
	instances, err := NewAdapter().Discover(context.Background(), missing)
	if err != nil {
		t.Fatalf("Discover(missing) error = %v, want nil", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover(missing) returned %d instances, want 0", len(instances))
	}
}

// TestDiscover_NoInstancesEmptyTree verifies the empty-project-tree case.
func TestDiscover_NoInstancesEmptyTree(t *testing.T) {
	instances, err := NewAdapter().Discover(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Discover(empty) error = %v, want nil", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover(empty) returned %d instances, want 0", len(instances))
	}
}

// TestDiscover_FindsOpenCodeDBAtRoot verifies the canonical happy path: the
// adapter looks for files literally named "opencode.db" inside the project
// tree.
func TestDiscover_FindsOpenCodeDBAtRoot(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		"opencode.db": "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedDBPaths(instances); len(got) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (%v)", len(got), got)
	}
	// On Windows, t.TempDir() and canonicalise-derived paths can differ in
	// representation (8.3 short names like RUNNER~1 vs long names like
	// runneradmin). Compare by case-folded base + cleaned parent so the
	// assert is stable across both representations.
	gotPath := filepath.Clean(instances[0].DBPath)
	wantPath := filepath.Clean(filepath.Join(root, "opencode.db"))
	if strings.EqualFold(filepath.Base(gotPath), filepath.Base(wantPath)) == false ||
		!strings.EqualFold(filepath.Dir(gotPath), filepath.Dir(wantPath)) {
		t.Fatalf("Discover DBPath = %q, want %q", gotPath, wantPath)
	}
	if instances[0].Source != domain.SourceOpenCode {
		t.Fatalf("Discover Source = %q, want %q", instances[0].Source, domain.SourceOpenCode)
	}
	if instances[0].Schema != SchemaTag {
		t.Fatalf("Discover Schema = %q, want %q", instances[0].Schema, SchemaTag)
	}
	if instances[0].ProjectRoot == "" {
		t.Fatal("Discover ProjectRoot is empty, want canonical project root")
	}
	if instances[0].Discovered.IsZero() {
		t.Fatal("Discover Discovered is zero, want a non-zero timestamp")
	}
}

// TestDiscover_FindsNestedOpenCodeDB verifies the discovery walk recurses
// into subdirectories of the project root.
func TestDiscover_FindsNestedOpenCodeDB(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join("nested", "deeper", "opencode.db"): "FILE",
		filepath.Join("sibling", "other", "opencode.db"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedDBPaths(instances); len(got) != 2 {
		t.Fatalf("Discover returned %d instances, want 2 (%v)", len(got), got)
	}
}

// TestDiscover_SkipsNonOpenCodeDB verifies the discovery only reports files
// with the exact "opencode.db" name. Other *.db files are ignored at this
// stage; Health decides whether they are valid OpenCode stores.
func TestDiscover_SkipsNonOpenCodeDB(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		"other.db":                        "FILE",
		filepath.Join("a", "x.db"):        "FILE",
		filepath.Join("a", "opencode.db"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedDBPaths(instances); len(got) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (%v)", len(got), got)
	}
}

// TestDiscover_RejectsSymlinkEscape verifies that an opencode.db reached via
// a symlink whose target lies outside the project root is not reported as an
// instance. The threat-model rule (docs/24-EXPERIENCE-THREAT-MODEL.md §3 T4)
// requires the adapter to never widen its discovery surface.
func TestDiscover_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink escape coverage is exercised on POSIX; Windows symlinks require admin")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "opencode.db"), []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	root := fixtureTree(t, map[string]string{
		"link-to-outside.db": "SYMLINK<-" + filepath.Join(outside, "opencode.db"),
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover returned %d instances, want 0 (symlink escape must be skipped)", len(instances))
	}
}

// TestDiscover_AcceptsInternalSymlink verifies that a symlink whose target
// remains inside the project root is accepted. The project root is the trust
// boundary, not the symlink itself.
func TestDiscover_AcceptsInternalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink coverage exercised on POSIX; Windows symlinks require admin")
	}
	root := fixtureTree(t, map[string]string{
		filepath.Join("real", "opencode.db"): "FILE",
		"opencode.db":                        "SYMLINK<-" + filepath.Join("real", "opencode.db"),
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) == 0 {
		t.Fatal("Discover returned 0 instances, want at least 1 for an internal symlink")
	}
}

// TestDiscover_ContextCanceled verifies Discover respects context
// cancellation. The contract requires every adapter method to bail out when
// the caller's context is already cancelled.
func TestDiscover_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewAdapter().Discover(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover with cancelled context error = %v, want context.Canceled", err)
	}
}

// TestDiscover_InstanceFieldsAreAbsolute verifies that the returned
// SourceInstance carries absolute canonical paths, regardless of how the
// caller spelled the input. This is a precondition for Health/Scan which
// rely on absolute paths.
func TestDiscover_InstanceFieldsAreAbsolute(t *testing.T) {
	root := fixtureTree(t, map[string]string{"opencode.db": "FILE"})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	for _, field := range []string{instances[0].ProjectRoot, instances[0].DBPath} {
		if !filepath.IsAbs(field) {
			t.Fatalf("Discover returned non-absolute path %q; absolute is required", field)
		}
	}
}

// TestDiscover_DiscoveredIsUTC verifies the Discovered timestamp is normalized
// to UTC so downstream code never has to deal with local-time skew.
func TestDiscover_DiscoveredIsUTC(t *testing.T) {
	adapter := NewAdapter()
	adapter.Now = func() time.Time {
		loc, err := time.LoadLocation("America/Buenos_Aires")
		if err != nil {
			return time.Now()
		}
		return time.Date(2026, 7, 24, 12, 0, 0, 0, loc)
	}
	root := fixtureTree(t, map[string]string{"opencode.db": "FILE"})
	instances, err := adapter.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	if instances[0].Discovered.Location() != time.UTC {
		t.Fatalf("Discover Discovered location = %v, want UTC", instances[0].Discovered.Location())
	}
}
