package retrieval

// DefaultWeights returns the v1 score-component weights. The budget
// is closed (sum = 1.0):
//
//	bm25            = 0.50
//	retrieval_terms = 0.20
//	title_exact     = 0.15
//	evidence_level  = 0.10
//	recency         = 0.05
//
// Rationale per docs/27 §Score components:
//
//   - bm25 dominates because the corpus is small and the FTS5
//     ranking is well-calibrated for it.
//   - retrieval_terms is the "did the user actually mean this
//     learning?" signal: a non-empty intersection with the
//     stored terms is the strongest practical hint.
//   - title_exact is the "exact phrase" boost; a learning whose
//     title matches the query verbatim usually IS what the user
//     wants.
//   - evidence_level ranks stronger evidence above weaker
//     evidence at equal relevance.
//   - recency is the smallest weight: a 1-year-old learning is
//     not punished heavily, but a 30-day-old one wins over a
//     6-year-old one at equal relevance.
//
// A future v2 may add a new component or rebalance; the
// Validate() loop catches any sum != 1.0 before the bad weights
// reach production.
func DefaultWeights() Weights {
	return Weights{
		BM25:           0.50,
		RetrievalTerms: 0.20,
		TitleExact:     0.15,
		EvidenceLevel:  0.10,
		Recency:        0.05,
	}
}
