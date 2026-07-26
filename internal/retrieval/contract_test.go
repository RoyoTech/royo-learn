// Contract tests for Hito 9 retrieval package.
//
// These tests pin the public API surface documented in
// docs/27-RETRIEVAL.md. They verify that:
//
//   - DefaultWeights() sums to 1.0 within a documented tolerance.
//   - DefaultLimit / MaxLimit are exported and reasonable.
//   - The Query, Hit, Result, ScoreComponents and Weights types expose
//     the documented fields (struct shape contract).
//   - Sanitize() handles empty input, FTS5 keywords, path traversal
//     fragments, control characters, oversize tokens and over-quota
//     term counts deterministically.
//
// They are RED before the package exists (compile failure), GREEN
// after types.go + sanitize.go are implemented. They do NOT depend on
// SQLite — pure in-memory checks only.

package retrieval_test

import (
	"math"
	"strings"
	"testing"

	"agent-royo-learn/internal/retrieval"
)

// TestContract_DefaultWeights_SumsToOne verifies the v1 weight budget
// is closed (no missing component, no overflowing component).
func TestContract_DefaultWeights_SumsToOne(t *testing.T) {
	t.Parallel()

	w := retrieval.DefaultWeights()
	sum := w.BM25 + w.RetrievalTerms + w.TitleExact + w.EvidenceLevel + w.Recency
	if math.Abs(sum-1.0) > 0.001 {
		t.Fatalf("DefaultWeights() sum = %f, want 1.0 ± 0.001 (got %+v)", sum, w)
	}
}

// TestContract_DefaultWeights_Validate verifies the closed-budget
// validator returns nil on the v1 weights.
func TestContract_DefaultWeights_Validate(t *testing.T) {
	t.Parallel()

	w := retrieval.DefaultWeights()
	if err := w.Validate(); err != nil {
		t.Fatalf("DefaultWeights().Validate() = %v, want nil", err)
	}
}

// TestContract_DefaultWeights_RejectsBadBudget verifies the
// validator catches a weights set that does not sum to 1.0.
func TestContract_DefaultWeights_RejectsBadBudget(t *testing.T) {
	t.Parallel()

	bad := retrieval.DefaultWeights()
	bad.BM25 = 0.9 // breaks the sum=1.0 invariant
	if err := bad.Validate(); err == nil {
		t.Fatalf("Validate(bad budget) = nil, want error")
	}
}

// TestContract_LimitConstants verifies the documented default and
// maximum are exported and reasonable.
func TestContract_LimitConstants(t *testing.T) {
	t.Parallel()

	if retrieval.DefaultLimit <= 0 {
		t.Fatalf("DefaultLimit = %d, want > 0", retrieval.DefaultLimit)
	}
	if retrieval.MaxLimit < retrieval.DefaultLimit {
		t.Fatalf("MaxLimit (%d) < DefaultLimit (%d)", retrieval.MaxLimit, retrieval.DefaultLimit)
	}
	if retrieval.MaxLimit > 1000 {
		t.Fatalf("MaxLimit = %d, want <= 1000", retrieval.MaxLimit)
	}
}

// TestContract_TypesHaveDocumentedFields verifies the public structs
// expose the fields documented in docs/27. A future refactor that
// removes or renames a field must fail loudly here.
func TestContract_TypesHaveDocumentedFields(t *testing.T) {
	t.Parallel()

	q := retrieval.Query{
		Text:      "hello",
		Limit:     10,
		Offset:    5,
		ProjectID: "proj-1",
	}
	if q.Text != "hello" || q.Limit != 10 || q.Offset != 5 || q.ProjectID != "proj-1" {
		t.Fatalf("Query field round-trip failed: %+v", q)
	}

	sc := retrieval.ScoreComponents{
		BM25:           0.5,
		RetrievalTerms: 0.2,
		TitleExact:     0.15,
		EvidenceLevel:  0.1,
		Recency:        0.05,
	}
	if sc.BM25 != 0.5 {
		t.Fatalf("ScoreComponents.BM25 round-trip failed")
	}

	// Result struct: Hits, Total, Query, TookMS.
	r := retrieval.Result{
		Hits:   nil,
		Total:  7,
		Query:  "q",
		TookMS: 42,
	}
	if r.Total != 7 || r.Query != "q" || r.TookMS != 42 {
		t.Fatalf("Result field round-trip failed: %+v", r)
	}

	w := retrieval.Weights{
		BM25:           0.5,
		RetrievalTerms: 0.2,
		TitleExact:     0.15,
		EvidenceLevel:  0.1,
		Recency:        0.05,
	}
	if w.EvidenceLevel != 0.1 {
		t.Fatalf("Weights.EvidenceLevel round-trip failed")
	}
}

// TestContract_Sanitize_Empty verifies that an empty query returns
// an empty slice with nil error (no search terms, not a validation
// failure).
func TestContract_Sanitize_Empty(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("")
	if err != nil {
		t.Fatalf("Sanitize(\"\") error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("Sanitize(\"\") = %v, want []", got)
	}
}

// TestContract_Sanitize_BasicTerms verifies the common case.
func TestContract_Sanitize_BasicTerms(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("hello world")
	if err != nil {
		t.Fatalf("Sanitize(\"hello world\") error = %v", err)
	}
	want := []string{"hello", "world"}
	if !equalSlice(got, want) {
		t.Fatalf("Sanitize = %v, want %v", got, want)
	}
}

// TestContract_Sanitize_PreservesFTS5Keywords verifies the v1 fix:
// AND/OR/NOT/NEAR are no longer dropped from the search query.
func TestContract_Sanitize_PreservesFTS5Keywords(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("AND operator")
	if err != nil {
		t.Fatalf("Sanitize error = %v", err)
	}
	want := []string{"AND", "operator"}
	if !equalSlice(got, want) {
		t.Fatalf("Sanitize = %v, want %v", got, want)
	}
}

// TestContract_Sanitize_FiltersPathTraversal verifies ".." is filtered
// out and that the surviving terms are searchable.
func TestContract_Sanitize_FiltersPathTraversal(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("../etc/passwd")
	if err != nil {
		t.Fatalf("Sanitize error = %v", err)
	}
	for _, term := range got {
		if term == ".." {
			t.Fatalf("Sanitize leaked path traversal segment: %v", got)
		}
	}
	// "etc" and "passwd" are valid alphanumerics → kept.
	want := []string{"etc", "passwd"}
	if !equalSlice(got, want) {
		t.Fatalf("Sanitize = %v, want %v", got, want)
	}
}

// TestContract_Sanitize_StripsLeadingSlash verifies path-like tokens
// are filtered before reaching FTS5.
func TestContract_Sanitize_StripsLeadingSlash(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("/etc/passwd safe")
	if err != nil {
		t.Fatalf("Sanitize error = %v", err)
	}
	for _, term := range got {
		if strings.HasPrefix(term, "/") {
			t.Fatalf("Sanitize leaked leading-slash term: %v", got)
		}
	}
}

// TestContract_Sanitize_HandlesControlChars verifies NUL and other
// control characters either split the term or are filtered without
// raising a fatal error.
func TestContract_Sanitize_HandlesControlChars(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("foo\x00bar")
	if err != nil {
		t.Fatalf("Sanitize(\"foo\\x00bar\") error = %v, want nil", err)
	}
	// The implementation may return [] or ["foo"] or ["foo", "bar"]
	// depending on how it splits. The contract requires no error.
	for _, term := range got {
		if strings.ContainsRune(term, 0) {
			t.Fatalf("Sanitize leaked control character in %q", term)
		}
	}
}

// TestContract_Sanitize_RejectsOversizeTerms verifies a 300-character
// term is discarded (or surfaced as a typed error) without panicking.
// The choice between silent discard and ErrTooManyTerms is documented
// in docs/27 §Sanitization.
func TestContract_Sanitize_RejectsOversizeTerms(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 300)
	got, err := retrieval.Sanitize(long)
	if err != nil && err != retrieval.ErrTooManyTerms {
		t.Fatalf("Sanitize(long) error = %v, want nil or ErrTooManyTerms", err)
	}
	for _, term := range got {
		if len(term) > 256 {
			t.Fatalf("Sanitize kept oversize term (len=%d)", len(term))
		}
	}
}

// TestContract_Sanitize_RejectsTooManyTerms verifies the 16-term cap.
func TestContract_Sanitize_RejectsTooManyTerms(t *testing.T) {
	t.Parallel()

	query := strings.Repeat("a ", 20) // 20 tokens
	_, err := retrieval.Sanitize(strings.TrimSpace(query))
	if err != retrieval.ErrTooManyTerms {
		t.Fatalf("Sanitize(20 tokens) error = %v, want ErrTooManyTerms", err)
	}
}

// TestContract_Sanitize_DedupePreservingOrder verifies duplicate
// terms collapse to the first occurrence's position.
func TestContract_Sanitize_DedupePreservingOrder(t *testing.T) {
	t.Parallel()

	got, err := retrieval.Sanitize("alpha beta alpha gamma beta")
	if err != nil {
		t.Fatalf("Sanitize error = %v", err)
	}
	want := []string{"alpha", "beta", "gamma"}
	if !equalSlice(got, want) {
		t.Fatalf("Sanitize = %v, want %v", got, want)
	}
}

// --- helpers ---

func equalSlice(a, b []string) bool {
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
