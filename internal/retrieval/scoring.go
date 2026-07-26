// Score component helpers exposed as part of the public API.
//
// The Service uses these internally; exposing them lets diagnostic
// tools (and the MCP "explain my score" feature planned for Hito 10)
// reuse the same arithmetic without re-implementing it. Each
// function takes only the inputs it needs; they are pure and
// trivially testable.

package retrieval

import (
	"math"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
)

// RetrievalTermsComponent returns 1.0 when the normalized query
// terms intersect the normalized candidate retrieval terms, else 0.
// Exported for the coverage test and for any future "explain my
// score" diagnostic. Both sides are normalized through
// patterns.NormalizeRetrievalTerms so a caller does not have to
// pre-clean the input.
func RetrievalTermsComponent(queryTerms, candidateTerms []string) float64 {
	normalizedQuery := patterns.NormalizeRetrievalTerms(queryTerms)
	return retrievalTermsComponent(normalizedQuery, candidateTerms)
}

// TitleExactComponent returns 1.0 when title matches the query
// verbatim (case-insensitive, trimmed), else 0.
func TitleExactComponent(title, queryText string) float64 {
	return titleExactComponent(title, queryText)
}

// EvidenceLevelComponent maps the domain enum to a [0,1] score.
func EvidenceLevelComponent(level domain.EvidenceLevel) float64 {
	return evidenceLevelComponent(level)
}

// RecencyComponent returns the linear-decay score for a learning
// created at `createdAt` relative to `now`.
func RecencyComponent(createdAt, now time.Time) float64 {
	return recencyComponent(createdAt, now)
}

// NormalizeBM25 maps a raw FTS5 bm25() value to [0,1] using the
// maxAbs normalization the Service uses internally. Exposed so a
// caller can preview the normalized score without running a full
// Service.Search.
//
// bm25() returns negative numbers (more negative = more relevant).
// We normalize with 1 / (1 + |raw| / maxAbs) so the best candidate
// in the pool scores 1.0 and every other candidate scores in (0, 1).
func NormalizeBM25(raw, maxAbs float64) float64 {
	if maxAbs == 0 {
		return 0.0
	}
	return 1.0 / (1.0 + math.Abs(raw)/maxAbs)
}

// IntersectRetrievalTerms returns the intersection of two
// retrieval-term lists, normalized through
// patterns.NormalizeRetrievalTerms. Exposed because the same
// computation is useful outside the Service (e.g. a "why was this
// ranked first?" UI).
func IntersectRetrievalTerms(a, b []string) []string {
	na := patterns.NormalizeRetrievalTerms(a)
	nb := patterns.NormalizeRetrievalTerms(b)
	if len(na) == 0 || len(nb) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(nb))
	for _, t := range nb {
		set[t] = struct{}{}
	}
	out := make([]string, 0, len(na))
	for _, t := range na {
		if _, ok := set[t]; ok {
			out = append(out, t)
		}
	}
	return out
}

// EqualFoldTrim is the canonical "do these match after trimming and
// case-folding?" check the title_exact component uses. Exposed so
// callers do not silently drift from the Service's definition.
func EqualFoldTrim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
