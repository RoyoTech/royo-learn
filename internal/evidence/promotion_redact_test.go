// Tests for the promotion-side redaction pipeline that lives in
// internal/evidence. The promotion package never calls RedactLearning or
// evidence.Service.Prepare directly; instead it funnels the fields it
// derived from a pattern through RedactPromotionFields, then hashes the
// redacted bag with PromotionFingerprint to produce the audit-row
// "what Promotion saw" digest.
//
// These tests are the RED surface for slice 7.1. They pin the order
// obligation (redact -> hash -> persist) and the four invariants the
// promotion bridge must respect:
//
//   - RedactPromotionFields strips every known secret from every free-text
//     field of a PromotionFields bag.
//   - The redaction report names exactly the fields that changed, with no
//     false positives and no false negatives.
//   - PromotionFingerprint is deterministic over the same input and
//     order-independent over the slice fields (Recommended,
//     RetrievalTerms) so the audit row stays stable when caller order
//     varies.
//   - The fingerprint computed BEFORE redaction and AFTER redaction must
//     differ when the input contains a secret; the promotion pipeline
//     must never persist a fingerprint of unredacted content.
//
// Determinism is critical: the audit row carries the fingerprint and a
// reviewer must be able to reproduce it byte-for-byte.

package evidence

import (
	"strings"
	"testing"
)

// TestRedactPromotionFields_StripsSecretsFromEveryFreeTextField pins
// the redaction surface. Every free-text field of PromotionFields must
// be scrubbed; every slice element must be scrubbed. The test uses
// distinct secret shapes per field so a regression that only handles
// one pattern class fails fast.
func TestRedactPromotionFields_StripsSecretsFromEveryFreeTextField(t *testing.T) {
	t.Parallel()

	openaiKey := "sk-secret1234567890abcdef"
	ghToken := "ghp_abcdefghijklmnopqrstuvwxyz0123" // 32 chars after ghp_
	awsKey := "AKIAIOSFODNN7EXAMPLE"

	in := &PromotionFields{
		Title:          "detect " + openaiKey + " in title",
		Context:        "context contains " + ghToken,
		Observation:    "observation has AWS " + awsKey,
		ReusableLesson: "lesson mentions " + openaiKey,
		Limits:         "limits referencing " + ghToken,
		Recommended:    []string{"step1 with " + openaiKey, "step2 ok"},
		RetrievalTerms: []string{"term-" + awsKey, "term-clean"},
	}

	report := RedactPromotionFields(in)

	if !report.AnyRedacted {
		t.Fatalf("AnyRedacted = false, want true (input had secrets in every field)")
	}

	// Title should not still carry the openai key in cleartext.
	if strings.Contains(in.Title, openaiKey) {
		t.Fatalf("RedactPromotionFields left the openai key in Title: %q", in.Title)
	}
	if !strings.Contains(in.Title, "[REDACTED:openai_key]") {
		t.Fatalf("RedactPromotionFields did not tag Title with openai_key marker; got %q", in.Title)
	}

	if strings.Contains(in.Context, ghToken) {
		t.Fatalf("RedactPromotionFields left the github token in Context: %q", in.Context)
	}
	if strings.Contains(in.Limits, ghToken) {
		t.Fatalf("RedactPromotionFields left the github token in Limits: %q", in.Limits)
	}
	if strings.Contains(in.Observation, awsKey) {
		t.Fatalf("RedactPromotionFields left the AWS key in Observation: %q", in.Observation)
	}
	if strings.Contains(in.ReusableLesson, openaiKey) {
		t.Fatalf("RedactPromotionFields left the openai key in ReusableLesson: %q", in.ReusableLesson)
	}

	// Every slice element must be checked.
	if strings.Contains(in.Recommended[0], openaiKey) {
		t.Fatalf("RedactPromotionFields left the openai key in Recommended[0]: %q", in.Recommended[0])
	}
	if strings.Contains(in.Recommended[1], "[REDACTED") {
		t.Fatalf("RedactPromotionFields marked a clean Recommended[1] as redacted: %q", in.Recommended[1])
	}
	if strings.Contains(in.RetrievalTerms[0], awsKey) {
		t.Fatalf("RedactPromotionFields left the AWS key in RetrievalTerms[0]: %q", in.RetrievalTerms[0])
	}
	if strings.Contains(in.RetrievalTerms[1], "[REDACTED") {
		t.Fatalf("RedactPromotionFields marked a clean RetrievalTerms[1] as redacted: %q", in.RetrievalTerms[1])
	}
}

// TestRedactPromotionFields_LeavesCleanFieldsUntouched pins the
// no-op path: a PromotionFields with no secret-shaped content must
// produce AnyRedacted=false and an empty RedactedFields list.
func TestRedactPromotionFields_LeavesCleanFieldsUntouched(t *testing.T) {
	t.Parallel()

	clean := "this string has nothing secret-shaped in it"
	in := &PromotionFields{
		Title:          clean,
		Context:        clean,
		Observation:    clean,
		ReusableLesson: clean,
		Limits:         clean,
		Recommended:    []string{clean, clean},
		RetrievalTerms: []string{clean, clean, clean},
	}

	before := captureFields(in)
	report := RedactPromotionFields(in)
	after := captureFields(in)

	if report.AnyRedacted {
		t.Fatalf("AnyRedacted = true for clean input, want false")
	}
	if len(report.RedactedFields) != 0 {
		t.Fatalf("RedactedFields = %v, want empty", report.RedactedFields)
	}
	if before != after {
		t.Fatalf("RedactPromotionFields mutated a clean input. before=%q after=%q", before, after)
	}
}

// TestRedactPromotionFields_ReportsEveryChangedField pins the report
// shape. A field that changed must appear in RedactedFields exactly
// once; a clean field must not appear. The list ordering is
// deterministic (insertion order matching the documented field list)
// so reviewers can rely on it.
func TestRedactPromotionFields_ReportsEveryChangedField(t *testing.T) {
	t.Parallel()

	openaiKey := "sk-secret1234567890"
	ghToken := "ghp_abcdefghijklmnopqrstuvwxyz0123"

	in := &PromotionFields{
		Title:          "ok",
		Context:        "ok",
		Observation:    "leak " + openaiKey,
		ReusableLesson: "ok",
		Limits:         "leak " + ghToken,
		Recommended:    []string{"leak " + openaiKey, "clean"},
		RetrievalTerms: []string{"leak " + openaiKey, "leak " + openaiKey},
	}

	report := RedactPromotionFields(in)

	if !report.AnyRedacted {
		t.Fatalf("AnyRedacted = false, want true")
	}

	want := map[string]bool{
		"observation":     false,
		"limits":          false,
		"recommended":     false,
		"retrieval_terms": false,
		"title":           true, // absent from list: clean
		"context":         true, // absent from list: clean
		"reusable_lesson": true, // absent from list: clean
	}
	got := map[string]bool{}
	for _, name := range report.RedactedFields {
		got[name] = true
	}
	for name, wantAbsent := range want {
		_, present := got[name]
		if present && wantAbsent {
			t.Fatalf("RedactedFields unexpectedly contains %q (clean field); got %v", name, report.RedactedFields)
		}
		if !present && !wantAbsent {
			t.Fatalf("RedactedFields missing %q (changed field); got %v", name, report.RedactedFields)
		}
	}
}

// TestRedactPromotionFields_NilReceiver pins the nil-safety contract:
// RedactPromotionFields must reject nil and return an empty report
// rather than panic. The promotion pipeline runs inside a contexts
// where a partial bag is plausible; a nil must not crash.
func TestRedactPromotionFields_NilReceiver(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RedactPromotionFields(nil) panicked: %v", r)
		}
	}()

	report := RedactPromotionFields(nil)
	if report.AnyRedacted {
		t.Fatalf("RedactPromotionFields(nil).AnyRedacted = true, want false")
	}
	if len(report.RedactedFields) != 0 {
		t.Fatalf("RedactPromotionFields(nil).RedactedFields = %v, want empty", report.RedactedFields)
	}
}

// TestPromotionFingerprint_Deterministic pins the determinism
// contract. Two PromotionFingerprint calls over the same redacted bag
// must return the same 64-char lowercase hex digest. The "what
// Promotion saw" hash is the audit-row anchor; any drift breaks the
// reviewer reproducibility contract.
func TestPromotionFingerprint_Deterministic(t *testing.T) {
	t.Parallel()

	clean := &PromotionFields{
		Title:          "stable title",
		Context:        "stable context",
		Observation:    "stable observation",
		ReusableLesson: "stable lesson",
		Limits:         "stable limits",
		Recommended:    []string{"alpha", "beta", "gamma"},
		RetrievalTerms: []string{"x", "y", "z"},
	}

	first := PromotionFingerprint(*clean)
	if len(first) != 64 {
		t.Fatalf("PromotionFingerprint length = %d, want 64 (sha256 hex)", len(first))
	}
	for _, c := range first {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("PromotionFingerprint contains non-lowercase-hex character %q in %q", c, first)
		}
	}

	// Calling again on the same input yields the same digest.
	for i := 0; i < 3; i++ {
		again := PromotionFingerprint(*clean)
		if again != first {
			t.Fatalf("PromotionFingerprint drifted on call %d; first=%q again=%q", i, first, again)
		}
	}
}

// TestPromotionFingerprint_OrderIndependentOverSlices pins the slice
// ordering rule: callers may produce Recommended or RetrievalTerms in
// any order; the preview hash must be stable. This mirrors the order
// normalisation patterns.digestCluster applies.
func TestPromotionFingerprint_OrderIndependentOverSlices(t *testing.T) {
	t.Parallel()

	base := &PromotionFields{
		Title:          "t",
		Context:        "c",
		Observation:    "o",
		ReusableLesson: "l",
		Limits:         "b",
		Recommended:    []string{"alpha", "beta", "gamma"},
		RetrievalTerms: []string{"x", "y", "z"},
	}
	h1 := PromotionFingerprint(*base)

	reordered := &PromotionFields{
		Title:          "t",
		Context:        "c",
		Observation:    "o",
		ReusableLesson: "l",
		Limits:         "b",
		Recommended:    []string{"gamma", "alpha", "beta"},
		RetrievalTerms: []string{"z", "x", "y"},
	}
	h2 := PromotionFingerprint(*reordered)

	if h1 != h2 {
		t.Fatalf("PromotionFingerprint depends on slice order; h1=%q h2=%q", h1, h2)
	}

	// Different slice content must shift the digest.
	different := &PromotionFields{
		Title:          "t",
		Context:        "c",
		Observation:    "o",
		ReusableLesson: "l",
		Limits:         "b",
		Recommended:    []string{"alpha", "beta"},
		RetrievalTerms: []string{"x", "y", "z"},
	}
	h3 := PromotionFingerprint(*different)
	if h3 == h1 {
		t.Fatalf("PromotionFingerprint equal for genuinely different slice content; h1=%q h3=%q", h1, h3)
	}
}

// TestPromotionFingerprint_DiffersAfterRedaction is the heart of the
// "redact before hash" rule (see HANDOFF-HITO7-PROMOTION.md §3 reglas
// innegociables). The audit row carries the preview fingerprint; if
// promotion computed it BEFORE redaction, the digest would not match
// what the field bag eventually persists and reviewers could not
// reproduce the audit row from SQLite alone.
func TestPromotionFingerprint_DiffersAfterRedaction(t *testing.T) {
	t.Parallel()

	openaiKey := "sk-secret1234567890"

	in := &PromotionFields{
		Title:          "leak " + openaiKey,
		Context:        "ok",
		Observation:    "ok",
		ReusableLesson: "ok",
		Limits:         "ok",
		Recommended:    []string{"clean"},
		RetrievalTerms: []string{"clean"},
	}

	// Hash BEFORE redaction: this is the fingerprint that MUST NOT be
	// persisted. The promotion pipeline runs RedactPromotionFields
	// first and only then hashes, so callers in production never see
	// this value reach an audit row.
	hashBefore := PromotionFingerprint(*in)

	report := RedactPromotionFields(in)
	if !report.AnyRedacted {
		t.Fatalf("setup error: Title redactor did not strip the secret; in=%q", in.Title)
	}

	hashAfter := PromotionFingerprint(*in)
	if hashBefore == hashAfter {
		t.Fatalf("PromotionFingerprint unchanged across redaction; hashBefore=%q hashAfter=%q (rule violated)",
			hashBefore, hashAfter)
	}

	// Hashing the redacted bag twice is still deterministic.
	hashAfterAgain := PromotionFingerprint(*in)
	if hashAfter != hashAfterAgain {
		t.Fatalf("PromotionFingerprint not stable after redaction; first=%q second=%q", hashAfter, hashAfterAgain)
	}
}

// captureFields snapshots the visible string content of a
// PromotionFields so tests can prove RedactPromotionFields did not
// mutate a clean input. The snapshot is order-sensitive so a buggy
// implementation that shuffles slice elements is also detected.
func captureFields(in *PromotionFields) string {
	if in == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("title=")
	b.WriteString(in.Title)
	b.WriteString("|context=")
	b.WriteString(in.Context)
	b.WriteString("|observation=")
	b.WriteString(in.Observation)
	b.WriteString("|lesson=")
	b.WriteString(in.ReusableLesson)
	b.WriteString("|limits=")
	b.WriteString(in.Limits)
	b.WriteString("|recommended=")
	b.WriteString(strings.Join(in.Recommended, ","))
	b.WriteString("|terms=")
	b.WriteString(strings.Join(in.RetrievalTerms, ","))
	return b.String()
}
