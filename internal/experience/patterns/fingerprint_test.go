// Fingerprint and retrieval-term normalization tests for Hito 6
// slice 6.1. Same TDD discipline as slice 6.0: tests first, then the
// minimal surface to make them green.
//
// The contract verifies the four invariants every pattern fingerprint
// must satisfy (docs/23-PATTERN-MINING.md §3):
//
//   - Order-independent: same set of inputs in any order produces the
//     same fingerprint. This mirrors the existing EventFingerprint
//     guarantee in internal/experience/detectors/persist.go.
//   - Volatile-value-stripping: timestamps, UUIDs, ports and absolute
//     user paths are not part of the hash input. The fingerprint is
//     computed over canonical problem tokens, the canonical tool name,
//     the canonical kind, the canonical result and the canonical
//     retrieval terms — nothing else.
//   - Deterministic: same canonical input → same fingerprint across
//     runs, processes and operating systems.
//   - Distinct: inputs that differ in any of the canonical fields
//     produce distinct fingerprints.
//
// Retrieval-term normalization verifies docs/23 §3 too:
//
//   - Lowercase, trimmed, non-empty terms.
//   - Sorted alphabetically so the fingerprint is order-independent.
//   - Duplicates removed after normalization.
//   - Volatile values (UUIDs, ports, hashes, absolute paths) stripped
//     so two sessions of the same pattern produce identical term sets.

package patterns

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

// TestNormalizeRetrievalTerms_Canonical verifies the normalizer
// output is sorted, deduplicated, lowercase and trimmed.
func TestNormalizeRetrievalTerms_Canonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "lowercase trimmed sorted deduplicated",
			in:   []string{"Foo", "  foo ", "BAR", "bar", "baz"},
			want: []string{"bar", "baz", "foo"},
		},
		{
			name: "stable order regardless of input order",
			in:   []string{"zeta", "alpha", "mango", "banana"},
			want: []string{"alpha", "banana", "mango", "zeta"},
		},
		{
			name: "empty inputs collapse to empty slice",
			in:   nil,
			want: []string{},
		},
		{
			name: "whitespace-only tokens are dropped",
			in:   []string{"   ", "\t", "ok", ""},
			want: []string{"ok"},
		},
		{
			name: "unicode terms normalize case-insensitively where possible",
			in:   []string{"Café", "CAFÉ", "cafe"},
			want: []string{"cafe", "café"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeRetrievalTerms(tc.in)
			if !sliceEqual(got, tc.want) {
				t.Fatalf("NormalizeRetrievalTerms(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeRetrievalTerms_StripsVolatile verifies that UUIDs,
// ports, hashes and absolute user paths are removed from the
// retrieval terms before they reach the fingerprint. This is the
// docs/23 §3 "eliminate" rule.
func TestNormalizeRetrievalTerms_StripsVolatile(t *testing.T) {
	t.Parallel()

	volatile := []string{
		"run-7c9f3a1b-7e3a-4f4a-8a6e-7c9f3a1b8a6e", // UUID
		"port:8080", // port
		"sha-9c3a1f8e7c9f3a1b8a6e7c9f3a1b8a6e7c9f3a1b",       // hash
		"/home/alice/projects/agent-royo-learn-codex-spec",   // absolute path
		"C:\\Users\\alice\\AppData\\Local\\Temp\\fixture.db", // Windows absolute path
		"safe", // kept
	}
	got := NormalizeRetrievalTerms(volatile)
	want := []string{"safe"}
	if !sliceEqual(got, want) {
		t.Fatalf("NormalizeRetrievalTerms(strip volatile) = %v, want %v", got, want)
	}
}

// TestNormalizeRetrievalTerms_NoSecretLeak ensures that the
// normalizer never accidentally retains redacted secrets or private
// paths in the term set. The threat model treats retrieval terms as
// observable output (docs/24 §6).
func TestNormalizeRetrievalTerms_NoSecretLeak(t *testing.T) {
	t.Parallel()

	leaks := []string{
		"[REDACTED:known]",
		"password=super-secret",
		"<private>super-secret</private>",
		"secret\nvalue",
	}
	got := NormalizeRetrievalTerms(leaks)
	for _, term := range got {
		if strings.Contains(term, "secret") || strings.Contains(term, "REDACTED") || strings.Contains(term, "<private>") {
			t.Fatalf("normalizer leaked sensitive content into term %q", term)
		}
	}
}

// TestPatternFingerprint_Deterministic checks the basic determinism
// rule: same input → same output, no matter the run.
func TestPatternFingerprint_Deterministic(t *testing.T) {
	t.Parallel()

	in := PatternInput{
		Kind:           "retry",
		ProblemTokens:  []string{"fingerprint", "failed", "within", "minutes"},
		Tool:           "git fetch",
		Result:         "fail",
		RetrievalTerms: []string{"retry", "git", "fetch"},
	}

	first := PatternFingerprint(in)
	second := PatternFingerprint(in)
	if first != second {
		t.Fatalf("PatternFingerprint is not deterministic: %s vs %s", first, second)
	}
	if !isHex64(first) {
		t.Fatalf("PatternFingerprint = %q, want 64-char hex", first)
	}
}

// TestPatternFingerprint_OrderIndependent verifies that the order of
// slices and maps in the input does not change the output. This is
// the same guarantee as EventFingerprint (slice 5.4) and is required
// for clustering idempotency.
func TestPatternFingerprint_OrderIndependent(t *testing.T) {
	t.Parallel()

	a := PatternInput{
		Kind:           "retry",
		ProblemTokens:  []string{"alpha", "beta", "gamma"},
		Tool:           "tool",
		Result:         "fail",
		RetrievalTerms: []string{"alpha", "beta"},
	}
	b := PatternInput{
		Kind:           "retry",
		ProblemTokens:  []string{"gamma", "alpha", "beta"},
		Tool:           "tool",
		Result:         "fail",
		RetrievalTerms: []string{"beta", "alpha"},
	}

	if PatternFingerprint(a) != PatternFingerprint(b) {
		t.Fatalf("PatternFingerprint is order-dependent: %s != %s", PatternFingerprint(a), PatternFingerprint(b))
	}
}

// TestPatternFingerprint_VolatileExcluded ensures that adding a
// volatile field (timestamp, UUID, session ID, hash, absolute path)
// does NOT change the fingerprint. This is the docs/23 §3
// "eliminate" rule at the function level.
func TestPatternFingerprint_VolatileExcluded(t *testing.T) {
	t.Parallel()

	canonical := PatternInput{
		Kind:           "retry",
		ProblemTokens:  []string{"alpha"},
		Tool:           "tool",
		Result:         "fail",
		RetrievalTerms: []string{"alpha"},
	}
	withVolatile := canonical
	withVolatile.Timestamps = []string{"2026-07-25T12:00:00Z", "2026-07-25T12:00:01Z"}
	withVolatile.UUIDs = []string{"7c9f3a1b-7e3a-4f4a-8a6e-7c9f3a1b8a6e"}
	withVolatile.AbsolutePaths = []string{"/home/alice/project", "C:\\Users\\bob\\project"}
	withVolatile.SessionIDs = []string{"session-1234567890"}
	withVolatile.Hashes = []string{"9c3a1f8e7c9f3a1b8a6e7c9f3a1b8a6e7c9f3a1b"}

	if PatternFingerprint(canonical) != PatternFingerprint(withVolatile) {
		t.Fatalf("PatternFingerprint leaked volatile fields: %s != %s",
			PatternFingerprint(canonical), PatternFingerprint(withVolatile))
	}
}

// TestPatternFingerprint_Distinct guarantees that differing any
// canonical field produces a different fingerprint. Two patterns must
// NEVER collide when their canonical inputs differ.
func TestPatternFingerprint_Distinct(t *testing.T) {
	t.Parallel()

	base := PatternInput{
		Kind:           "retry",
		ProblemTokens:  []string{"alpha"},
		Tool:           "tool",
		Result:         "fail",
		RetrievalTerms: []string{"alpha"},
	}

	mutations := []struct {
		name   string
		mutate func(*PatternInput)
	}{
		{"kind", func(p *PatternInput) { p.Kind = "command_failure" }},
		{"tool", func(p *PatternInput) { p.Tool = "other-tool" }},
		{"result", func(p *PatternInput) { p.Result = "success" }},
		{"problem_tokens", func(p *PatternInput) { p.ProblemTokens = []string{"beta"} }},
		{"retrieval_terms", func(p *PatternInput) { p.RetrievalTerms = []string{"beta"} }},
	}
	for _, m := range mutations {
		m := m
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()
			candidate := base
			m.mutate(&candidate)
			if PatternFingerprint(base) == PatternFingerprint(candidate) {
				t.Fatalf("PatternFingerprint collided after mutating %s", m.name)
			}
		})
	}
}

// TestPatternFingerprint_PrefixFree verifies that the fingerprint
// length is exactly 64 lowercase hex characters, the standard sha256
// representation. No truncation, no prefix collision risk.
func TestPatternFingerprint_PrefixFree(t *testing.T) {
	t.Parallel()

	in := PatternInput{Kind: "retry"}
	got := PatternFingerprint(in)
	if !isHex64(got) {
		t.Fatalf("PatternFingerprint = %q, want 64-char lowercase hex", got)
	}
}

// --- helpers ---

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	matched, err := regexp.MatchString("^[0-9a-f]{64}$", s)
	if err != nil {
		return false
	}
	return matched
}

// Compile-time guard for the sha256/hex imports we will use once
// the production surface lands. Keeps the test file compiling when
// the implementation is not yet wired up (RED phase).
var _ = sha256.New
var _ = hex.EncodeToString
