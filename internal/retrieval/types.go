// Package retrieval implements lexical retrieval for Hito 9 (plan
// §33, docs/27). It is the recommended surface for "find me learnings
// about X" — the CLI `search` and the MCP `learning_search` tool both
// go through it.
//
// The package owns:
//
//   - The Sanitize() function: whitelists the user's query terms so
//     FTS5 cannot be tricked into SQL injection or path traversal.
//   - The Repository: a thin wrapper over the existing `learnings_fts`
//     virtual table (migration 001) that returns Candidates with a
//     raw FTS5 bm25() score.
//   - The Service: takes sanitized terms, applies score components
//     (bm25 normalized, retrieval_terms overlap, title_exact,
//     evidence_level, recency) and returns a deterministic, ordered
//     Hit list. Tiebreaker is (score DESC, fingerprint ASC, id ASC)
//     so two identical queries produce identical orderings across
//     runs.
//
// Hito 9 inherits the existing schema (001_init.sql); no migration is
// added. The score components are additive: the JSON output now
// exposes `score` and `score_components` per hit, both opt-in for
// backward compatibility with the CLI/MCP consumers.
package retrieval

import (
	"errors"
	"math"

	"agent-royo-learn/internal/domain"
)

// DefaultLimit is the default page size for Service.Search when the
// caller does not specify Limit.
const DefaultLimit = 50

// MaxLimit caps the page size to keep ranking deterministic and the
// JSON payload bounded. Callers asking for more are silently capped
// to this value.
const MaxLimit = 200

// weightSumTolerance is the maximum deviation from 1.0 that
// Weights.Validate() accepts. ±0.001 covers the floating-point round
// trip of the v1 documented weights (0.5+0.2+0.15+0.10+0.05 = 1.0).
const weightSumTolerance = 0.001

// ErrTooManyTerms is returned by Sanitize() when the input yields
// more than MaxTermsPerQuery raw tokens. The error is exported so
// Service.Search can wrap it as a typed domain error.
var ErrTooManyTerms = errors.New("retrieval: query has too many terms (max 16)")

// MaxTermsPerQuery is the hard cap on terms accepted by Sanitize.
// Exceeding it surfaces ErrTooManyTerms instead of silently
// truncating, so the operator knows they asked for too much.
const MaxTermsPerQuery = 16

// MaxTermLength is the hard cap on the length of a single term.
const MaxTermLength = 256

// Query is the input the user submits. ProjectID is required so the
// repository can scope the FTS5 query; the CLI/MCP project binding
// flows in via this field.
//
// Limit/Offset default to DefaultLimit/0 when zero. Limit is capped
// at MaxLimit.
type Query struct {
	Text      string
	Limit     int
	Offset    int
	ProjectID domain.ProjectID
}

// Options groups configuration knobs the Service honors. The default
// Service built by NewService(repo, DefaultWeights()) ignores
// Options; tests that need to override weights inject a custom
// Service with their own weights.
type Options struct {
	Limit   int
	Offset  int
	Weights Weights
}

// Weights are the score-component weights applied to each Candidate.
// The v1 budget is closed: DefaultWeights() sums to 1.0 and
// Validate() rejects any other configuration. A future v2 can relax
// this and add new components without breaking the Validate() loop.
//
// Field tags are JSON tags so the Weights can be exposed as a
// diagnostic blob from CLI/MCP without an extra marshalling step.
type Weights struct {
	BM25           float64 `json:"bm25"`
	RetrievalTerms float64 `json:"retrieval_terms"`
	TitleExact     float64 `json:"title_exact"`
	EvidenceLevel  float64 `json:"evidence_level"`
	Recency        float64 `json:"recency"`
}

// Validate returns an error when the weights do not sum to 1.0
// within weightSumTolerance. The default weights always validate.
func (w Weights) Validate() error {
	sum := w.BM25 + w.RetrievalTerms + w.TitleExact + w.EvidenceLevel + w.Recency
	if math.Abs(sum-1.0) > weightSumTolerance {
		return errors.New("retrieval: weights must sum to 1.0 ± 0.001")
	}
	return nil
}

// ScoreComponents records the per-component score that contributed
// to the final Hit.Score. The slice is structured (not a map) so the
// JSON output is stable across runs and decoders can rely on field
// order.
//
// All components are normalized to [0,1] before the weighted sum.
type ScoreComponents struct {
	BM25           float64 `json:"bm25"`
	RetrievalTerms float64 `json:"retrieval_terms"`
	TitleExact     float64 `json:"title_exact"`
	EvidenceLevel  float64 `json:"evidence_level"`
	Recency        float64 `json:"recency"`
}

// Hit is one ranked learning returned by Service.Search.
//
// Learning is the canonical domain object so callers can render it
// the same way they render any other learning.
//
// Score is the weighted sum of the components in
// DefaultWeights() order. Two runs with the same query and the same
// underlying data produce identical Score values.
//
// Components are the breakdown by component, normalized to [0,1]
// before weighting.
type Hit struct {
	Learning   *domain.Learning
	Score      float64
	Components ScoreComponents
}

// Result is the full search outcome.
//
// Hits is empty (not nil) when nothing matches. Total is the number
// of Candidates the repository returned before Limit/Offset was
// applied — useful for "X of Y" UIs without re-running the search.
// Query echoes the input so the consumer can correlate logs.
// TookMS is the wall-clock duration of the Service.Search call.
type Result struct {
	Hits   []Hit
	Total  int
	Query  string
	TookMS int64
}
