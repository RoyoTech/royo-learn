package claudecode

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/project"
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

func sortedJSONLPaths(instances []SourceInstance) []string {
	paths := make([]string, 0, len(instances))
	for _, inst := range instances {
		paths = append(paths, filepath.ToSlash(inst.JSONLPath))
	}
	sort.Strings(paths)
	return paths
}

// encodedSlug mirrors the upstream Claude Code project slug encoding. The
// adapter does not decode the slug itself, but tests build the encoded
// directory the same way upstream does so the layout is realistic.
func encodedSlug(projectRoot string) string {
	abs := filepath.Clean(projectRoot)
	abs = strings.ReplaceAll(abs, ":", "")
	return url.PathEscape(abs)
}

// TestDiscover_EmptyProjectRoot rejects a missing root with a typed error.
func TestDiscover_EmptyProjectRoot(t *testing.T) {
	if _, err := NewAdapter().Discover(context.Background(), ""); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("Discover(empty) error = %v, want invalid_argument", err)
	}
}

// TestDiscover_NoInstancesEmptyTree verifies the empty-project-tree case
// (empty root -> empty result, no error per acceptance).
func TestDiscover_NoInstancesEmptyTree(t *testing.T) {
	instances, err := NewAdapter().Discover(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Discover(empty) error = %v, want nil", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover(empty) returned %d instances, want 0", len(instances))
	}
}

// TestDiscover_NonexistentRootReturnsEmpty verifies that a project root that
// simply does not exist is reported as "no instances" rather than an error.
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

// TestDiscover_FindsJSONLUnderEncodedDir verifies the canonical happy path:
// the adapter walks <projectRoot>/.claude/projects/<encoded>/<uuid>.jsonl.
func TestDiscover_FindsJSONLUnderEncodedDir(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", encodedSlug("root"), "11111111-1111-1111-1111-111111111111.jsonl"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedJSONLPaths(instances); len(got) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (%v)", len(got), got)
	}
	wantPath, wantErr := project.Canonicalize(
		filepath.Join(root, ".claude", "projects", encodedSlug("root"),
			"11111111-1111-1111-1111-111111111111.jsonl"))
	if wantErr != nil {
		t.Fatalf("canonicalise expected JSONLPath: %v", wantErr)
	}
	if instances[0].JSONLPath != wantPath {
		t.Fatalf("Discover JSONLPath = %q, want %q", instances[0].JSONLPath, wantPath)
	}
	if instances[0].Source != domain.SourceClaudeCode {
		t.Fatalf("Discover Source = %q, want %q", instances[0].Source, domain.SourceClaudeCode)
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

// TestDiscover_AcceptsAnyEncodedDirUnderRoot verifies that the adapter does
// NOT decode the slug: any <encoded> directory directly under projectRoot is
// honored. This pins the design decision that the slug is opaque.
func TestDiscover_AcceptsAnyEncodedDirUnderRoot(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", "alpha", "22222222-2222-2222-2222-222222222222.jsonl"): "FILE",
		filepath.Join(".claude", "projects", "bravo", "33333333-3333-3333-3333-333333333333.jsonl"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedJSONLPaths(instances); len(got) != 2 {
		t.Fatalf("Discover returned %d instances, want 2 (%v)", len(got), got)
	}
}

// TestDiscover_DepthCap verifies that nested directories beyond
// maxDiscoveryDepth (8) are not descended into.
func TestDiscover_DepthCap(t *testing.T) {
	rel := filepath.Join(".claude", "projects", encodedSlug("root"),
		"44444444-4444-4444-4444-444444444444.jsonl")
	// Prefix 8 deep subdirs, then the .claude tree. The walk should bail out
	// before reaching the .claude/projects subdir.
	deep := rel
	for i := 0; i < 9; i++ {
		deep = filepath.Join("d", deep)
	}
	root := fixtureTree(t, map[string]string{deep: "FILE"})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover with deep nested JSONL returned %d instances, want 0 (depth cap)", len(instances))
	}
}

// TestDiscover_SkipsNonJSONLFiles verifies the discovery only reports files
// with the .jsonl extension under the .claude/projects/<encoded> tree.
// Other files (notes, README, .md) are ignored at this stage.
func TestDiscover_SkipsNonJSONLFiles(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", encodedSlug("root"), "notes.md"):                                   "FILE",
		filepath.Join(".claude", "projects", encodedSlug("root"), "README"):                                     "FILE",
		filepath.Join(".claude", "projects", encodedSlug("root"), "55555555-5555-5555-5555-555555555555.jsonl"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := sortedJSONLPaths(instances); len(got) != 1 {
		t.Fatalf("Discover returned %d instances, want 1 (%v)", len(got), got)
	}
}

// TestDiscover_RejectsSymlinkEscape verifies that a JSONL reached via a
// symlink whose target lies outside the project root is rejected with a
// typed error.
func TestDiscover_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink escape coverage is exercised on POSIX; Windows symlinks require admin")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "66666666-6666-6666-6666-666666666666.jsonl"),
		[]byte("fixture"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", "alpha",
			"link-to-outside.jsonl"): "SYMLINK<-" + filepath.Join(outside, "66666666-6666-6666-6666-666666666666.jsonl"),
	})
	_, err := NewAdapter().Discover(context.Background(), root)
	if err == nil {
		t.Fatalf("Discover with symlink escape returned no error, want experience_locator_outside_root")
	}
	if domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("Discover with symlink escape error = %v, want experience_locator_outside_root", err)
	}
}

// TestDiscover_RejectsSessionFileOutsideRoot verifies that a JSONL file
// located outside the trust boundary (under the .claude/projects tree but
// escaped via IsInsideRoot check) is not surfaced, per docs/24 T4.
func TestDiscover_RejectsSessionFileOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink escape coverage is exercised on POSIX")
	}
	outside := t.TempDir()
	slug := encodedSlug("outside")
	outsideJSONL := filepath.Join(outside, ".claude", "projects", slug,
		"77777777-7777-7777-7777-777777777777.jsonl")
	if err := os.MkdirAll(filepath.Dir(outsideJSONL), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(outsideJSONL, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	root := fixtureTree(t, map[string]string{
		// A symlink under root that points OUTSIDE root — canonicalised path
		// falls outside the trust boundary and must be rejected.
		filepath.Join(".claude", "projects", "alpha",
			"link-outside.jsonl"): "SYMLINK<-" + outsideJSONL,
	})
	_, err := NewAdapter().Discover(context.Background(), root)
	if err == nil {
		t.Fatalf("Discover with out-of-root JSONL returned no error, want experience_locator_outside_root")
	}
	if domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("Discover outside root error = %v, want experience_locator_outside_root", err)
	}
}

// TestDiscover_ContextCanceled verifies Discover respects context
// cancellation. The contract requires the returned error to be exactly
// context.Canceled (not wrapped).
func TestDiscover_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Discover(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover with cancelled context error = %v, want context.Canceled", err)
	}
	if err != context.Canceled {
		t.Fatalf("Discover with cancelled context err = %v, want literal context.Canceled (not wrapped)", err)
	}
}

// TestDiscover_InstanceFieldsAreAbsolute verifies that the returned
// SourceInstance carries absolute canonical paths.
func TestDiscover_InstanceFieldsAreAbsolute(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", "alpha", "88888888-8888-8888-8888-888888888888.jsonl"): "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover returned %d instances, want 1", len(instances))
	}
	for _, field := range []string{instances[0].ProjectRoot, instances[0].JSONLPath} {
		if !filepath.IsAbs(field) {
			t.Fatalf("Discover returned non-absolute path %q; absolute is required", field)
		}
	}
}

// TestDiscover_DeterministicSort verifies that two scans over the same
// project layout return instances in the same (sorted) order.
func TestDiscover_DeterministicSort(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", "alpha", "99999999-9999-9999-9999-999999999999.jsonl"):   "FILE",
		filepath.Join(".claude", "projects", "bravo", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl"):   "FILE",
		filepath.Join(".claude", "projects", "charlie", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.jsonl"): "FILE",
	})
	first, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover(first): %v", err)
	}
	second, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover(second): %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("discoveries returned different counts: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].JSONLPath != second[i].JSONLPath {
			t.Fatalf("discoveries differ at index %d: %q vs %q", i, first[i].JSONLPath, second[i].JSONLPath)
		}
	}
	// Sorted by JSONLPath.
	for i := 1; i < len(first); i++ {
		if first[i-1].JSONLPath > first[i].JSONLPath {
			t.Fatalf("Discover output not sorted at index %d: %q > %q", i, first[i-1].JSONLPath, first[i].JSONLPath)
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
	root := fixtureTree(t, map[string]string{
		filepath.Join(".claude", "projects", "alpha", "cccccccc-cccc-cccc-cccc-cccccccccccc.jsonl"): "FILE",
	})
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
