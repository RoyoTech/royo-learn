// Service is the orchestration layer for Hito 9. It owns:
//
//   - Sanitization: ErrTooManyTerms is wrapped as a typed
//     domain.ErrInvalidArgument so the CLI/MCP can map it to the
//     documented exit code 2.
//   - Limit/Offset handling: defaults applied, MaxLimit cap enforced.
//   - Score-component computation: per the v1 weights, all components
//     normalized to [0,1] before the weighted sum.
//   - Deterministic ordering: sort.SliceStable by score DESC, then
//     fingerprint ASC, then id ASC. The same query against the same
//     database state produces the same order across runs.
//
// The Service does NOT write to the database. It is a read-only
// orchestrator on top of the Repository.

package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
)

// recencyWindowDays is the linear-decay horizon for the recency
// component. A learning older than this gets recency = 0; a
// learning fresher than recencyFreshDays gets recency = 1.0.
const (
	recencyWindowDays = 365.0
	recencyFreshDays  = 7.0
)

// ErrNotImplemented is returned by the SearchWithEngram stub so a
// caller that wires the future Engram integration does not silently
// fall back to a degraded path. The plan keeps Engram integration
// for Hito 10.
var ErrNotImplemented = errors.New("retrieval: SearchWithEngram not implemented yet")

// overfetchMultiplier scales the limit passed to the Repository so
// the Service has a pool to score before applying Limit/Offset. 4x
// is enough to handle the "many candidates, only the top survive"
// case without being so large that a noisy query degrades perf.
const overfetchMultiplier = 4

// Service is the high-level entry point for retrieval.
type Service struct {
	repo    *Repository
	weights Weights
	now     func() time.Time
}

// NewService returns a Service bound to repo with the supplied
// weights. The clock defaults to time.Now; tests can override with
// SetNow.
func NewService(repo *Repository, weights Weights) *Service {
	return &Service{
		repo:    repo,
		weights: weights,
		now:     time.Now,
	}
}

// SetNow replaces the clock used to compute recency. Tests inject a
// fixed clock; production leaves the default time.Now.
func (s *Service) SetNow(now func() time.Time) {
	s.now = now
}

// SearchWithEngram is a placeholder for the future Engram-aware
// retrieval. The current implementation returns ErrNotImplemented;
// the signature is stable so a future PR can plug the real
// implementation in without breaking callers.
//
// engramFn is reserved for the Hito 10 integration; today it is
// ignored.
func (s *Service) SearchWithEngram(ctx context.Context, q Query, engramFn func(string) []domain.Learning) (Result, error) {
	return Result{}, ErrNotImplemented
}

// Search runs the full retrieval pipeline and returns a ranked,
// deterministic Result. The Result.Hits slice is non-nil but may be
// empty when no candidates match.
//
// Errors:
//   - ErrTooManyTerms from Sanitize is wrapped as
//     domain.NewValidationError(domain.ErrInvalidArgument, ...) so
//     the CLI/MCP can surface it with exit code 2.
//   - Repository errors are wrapped with the "retrieval:" prefix
//     and propagated unchanged.
func (s *Service) Search(ctx context.Context, q Query) (Result, error) {
	start := s.now()

	terms, err := Sanitize(q.Text)
	if err != nil {
		if errors.Is(err, ErrTooManyTerms) {
			return Result{}, domain.NewValidationError(
				domain.ErrInvalidArgument,
				fmt.Sprintf("retrieval: query has too many terms (max %d)", MaxTermsPerQuery),
			)
		}
		return Result{}, fmt.Errorf("retrieval: sanitize: %w", err)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	// Overfetch so we have a pool to score before applying the
	// user's Limit/Offset. The Repository does not need to know
	// about the user's page size.
	fetch := limit * overfetchMultiplier
	if fetch <= 0 {
		fetch = limit
	}
	candidates, err := s.repo.Search(ctx, q.ProjectID, terms, fetch)
	if err != nil {
		return Result{}, err
	}
	if len(candidates) == 0 {
		return Result{
			Hits:   []Hit{},
			Total:  0,
			Query:  q.Text,
			TookMS: sinceMS(start, s.now()),
		}, nil
	}

	scored := s.scoreAll(candidates, terms, q.Text)
	sortHitsDeterministic(scored)

	hits := make([]Hit, 0, limit)
	if offset >= len(scored) {
		return Result{
			Hits:   []Hit{},
			Total:  len(scored),
			Query:  q.Text,
			TookMS: sinceMS(start, s.now()),
		}, nil
	}
	end := offset + limit
	if end > len(scored) {
		end = len(scored)
	}
	for _, sh := range scored[offset:end] {
		hits = append(hits, sh.hit)
	}

	return Result{
		Hits:   hits,
		Total:  len(scored),
		Query:  q.Text,
		TookMS: sinceMS(start, s.now()),
	}, nil
}

// sinceMS returns the duration between start and now in
// milliseconds. It is tolerant to a non-monotonic clock.
func sinceMS(start, now time.Time) int64 {
	d := now.Sub(start)
	if d < 0 {
		d = 0
	}
	return d.Milliseconds()
}

// scoredHit bundles the Hit with the fields the sorter needs. We
// keep the Hit in the struct so we don't recompute the Learning
// pointer later.
type scoredHit struct {
	hit         Hit
	fingerprint string
	id          string
	score       float64
}

// scoreAll computes the score components for every candidate and
// returns them in input order. The sorter is then free to reorder.
//
// The score components per docs/27 §Score components:
//
//	bm25            = 1 / (1 + |bm25_raw| / max_abs_bm25)
//	retrieval_terms = 1.0 if normalize(query_terms) ∩
//	                  normalize(retrieval_terms) is non-empty, else 0
//	title_exact     = 1.0 if EqualFold(Trim(title), Trim(query)), else 0
//	evidence_level  = {strong:1.0, moderate:0.7, weak:0.4,
//	                   insufficient:0.1, default:0.5}
//	recency         = 1 - min(1, days_since / 365), capped at 1.0
//	                  for any learning < 7 days old.
//
// The final score is the dot product of weights and components.
func (s *Service) scoreAll(candidates []Candidate, terms []string, queryText string) []scoredHit {
	// First pass: normalize BM25 across the candidate pool.
	maxAbs := 0.0
	for i := range candidates {
		abs := math.Abs(candidates[i].BM25Score)
		if abs > maxAbs {
			maxAbs = abs
		}
	}
	norm := func(raw float64) float64 {
		if maxAbs == 0 {
			return 0.0
		}
		return 1.0 / (1.0 + math.Abs(raw)/maxAbs)
	}

	normalizedTerms := patterns.NormalizeRetrievalTerms(terms)

	out := make([]scoredHit, 0, len(candidates))
	for _, c := range candidates {
		components := ScoreComponents{
			BM25:           norm(c.BM25Score),
			RetrievalTerms: retrievalTermsComponent(normalizedTerms, c.RetrievalTerms),
			TitleExact:     titleExactComponent(c.Title, queryText),
			EvidenceLevel:  evidenceLevelComponent(c.EvidenceLevel),
			Recency:        recencyComponent(c.CreatedAt, s.now()),
		}
		score := s.weights.BM25*components.BM25 +
			s.weights.RetrievalTerms*components.RetrievalTerms +
			s.weights.TitleExact*components.TitleExact +
			s.weights.EvidenceLevel*components.EvidenceLevel +
			s.weights.Recency*components.Recency

		hit := Hit{
			Learning:   candidateToLearning(c),
			Score:      score,
			Components: components,
		}
		out = append(out, scoredHit{
			hit:         hit,
			fingerprint: c.Fingerprint,
			id:          string(c.ID),
			score:       score,
		})
	}
	return out
}

// retrievalTermsComponent returns 1.0 when the intersection between
// the normalized query terms and the normalized candidate terms is
// non-empty, else 0.
//
// Both sides are normalized with the same canonical form the
// fingerprint uses (lowercase, trimmed, sorted, deduped, stripped
// of UUIDs/paths/etc.) so two equivalent inputs produce the same
// component value.
func retrievalTermsComponent(normalizedQuery []string, candidateTerms []string) float64 {
	if len(normalizedQuery) == 0 || len(candidateTerms) == 0 {
		return 0.0
	}
	normalizedCand := patterns.NormalizeRetrievalTerms(candidateTerms)
	if len(normalizedCand) == 0 {
		return 0.0
	}
	candSet := make(map[string]struct{}, len(normalizedCand))
	for _, t := range normalizedCand {
		candSet[t] = struct{}{}
	}
	for _, t := range normalizedQuery {
		if _, ok := candSet[t]; ok {
			return 1.0
		}
	}
	return 0.0
}

// titleExactComponent returns 1.0 when the candidate title matches
// the query verbatim (case-insensitive, trimmed), else 0. We use
// EqualFold to handle the common "Title Case" vs "title case"
// mismatch without pulling in a Unicode case-folding library.
func titleExactComponent(title, queryText string) float64 {
	if title == "" || queryText == "" {
		return 0.0
	}
	if strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(queryText)) {
		return 1.0
	}
	return 0.0
}

// evidenceLevelComponent maps the domain enum to the [0,1] score.
// Unknown levels fall back to 0.5 (neutral) so a future domain
// value does not silently get a 0 score.
func evidenceLevelComponent(level domain.EvidenceLevel) float64 {
	switch level {
	case domain.EvidenceStrong:
		return 1.0
	case domain.EvidenceModerate:
		return 0.7
	case domain.EvidenceWeak:
		return 0.4
	case domain.EvidenceInsufficient:
		return 0.1
	default:
		return 0.5
	}
}

// recencyComponent returns 1.0 for a learning created within the
// fresh window, then decays linearly to 0.0 over recencyWindowDays.
func recencyComponent(createdAt, now time.Time) float64 {
	if createdAt.IsZero() {
		return 0.0
	}
	days := now.Sub(createdAt).Hours() / 24.0
	if days < recencyFreshDays {
		return 1.0
	}
	decay := 1.0 - math.Min(1.0, days/recencyWindowDays)
	if decay < 0 {
		decay = 0
	}
	return decay
}

// sortHitsDeterministic orders scoredHits by (score DESC,
// fingerprint ASC, id ASC). We use sort.SliceStable so equal-score
// hits stay in their pre-sort order (a tie on all three keys is a
// stable tie).
func sortHitsDeterministic(hits []scoredHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].fingerprint != hits[j].fingerprint {
			return hits[i].fingerprint < hits[j].fingerprint
		}
		return hits[i].id < hits[j].id
	})
}

// candidateToLearning promotes a Candidate to a *domain.Learning.
// It is a copy, not a move, because the caller may need to keep
// the Candidate around (e.g. for diagnostics).
func candidateToLearning(c Candidate) *domain.Learning {
	l := &domain.Learning{
		ID:                  c.ID,
		ProjectID:           c.ProjectID,
		Status:              c.Status,
		Type:                c.Type,
		Title:               c.Title,
		Context:             c.Context,
		Observation:         c.Observation,
		ReusableLesson:      c.ReusableLesson,
		RetrievalTerms:      c.RetrievalTerms,
		EvidenceLevel:       c.EvidenceLevel,
		ScopeGuess:          c.ScopeGuess,
		ApprovedScope:       c.ApprovedScope,
		Confidence:          c.Confidence,
		Limits:              c.Limits,
		ProposedDestination: c.ProposedDestination,
		ApprovedDestination: c.ApprovedDestination,
		Fingerprint:         c.Fingerprint,
		NormalizedHash:      c.NormalizedHash,
		IdempotencyKey:      c.IdempotencyKey,
		Actor:               c.Actor,
		Revision:            c.Revision,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
	return l
}
