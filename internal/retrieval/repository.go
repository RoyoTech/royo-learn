// Repository is the data-access layer for Hito 9. It owns:
//
//   - The FTS5 MATCH expression (parameterized, no string concat of
//     user input).
//   - The SELECT shape that yields a Candidate plus the raw bm25()
//     score from FTS5.
//   - The scan into Candidate: every column the Service needs to
//     rebuild a *domain.Learning is read here, so the Service can
//     stay free of *sql.Rows.
//
// The repository does NOT understand score components, ranking or
// pagination. It returns raw Candidates and lets the Service layer
// decide how to score and order them.
//
// Why a flat Candidate struct (and not *domain.Learning)?
//
//   - The repository is the boundary where SQL and Go meet. Mixing
//     the full Learning payload with a rank-influencing float (the
//     raw bm25 score) is easier with one struct that holds both.
//   - *domain.Learning has many pointer fields (ApprovedDestination,
//     ApprovedScope, IdempotencyKey) whose scan logic is centralized
//     in storage.scanLearning. Reusing it would mean reaching into
//     storage from retrieval, inverting the dependency. The flat
//     struct keeps the layering clean.

package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// Candidate is the flat struct the Repository returns. Service
// converts it into *domain.Learning + ScoreComponents.
//
// BM25Score is the raw FTS5 bm25() output. FTS5 ranks LOWER
// (more negative) as the document is more relevant, so the Service
// normalizes to [0,1] where 1 = most relevant.
type Candidate struct {
	ID                  domain.LearningID
	ProjectID           domain.ProjectID
	Status              domain.LearningStatus
	Type                domain.LearningType
	Title               string
	Context             string
	Observation         string
	ReusableLesson      string
	RetrievalTerms      []string
	EvidenceLevel       domain.EvidenceLevel
	ScopeGuess          domain.Scope
	ApprovedScope       *domain.Scope
	Confidence          domain.Confidence
	Limits              string
	ProposedDestination domain.DestinationType
	ApprovedDestination *domain.Destination
	Fingerprint         string
	NormalizedHash      string
	IdempotencyKey      *string
	Actor               domain.Actor
	Revision            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	BM25Score           float64
}

// defaultSearchSQL is the production SELECT. Tests can override
// Repository.searchSQL with a fixture-aware query.
const defaultSearchSQL = `
SELECT
	l.id, l.project_id, l.status, l.type, l.title, l.context, l.observation,
	l.reusable_lesson, l.recommended_procedure_json, l.limits_text,
	l.scope_guess, l.approved_scope, l.confidence, l.evidence_level,
	l.proposed_destination, l.approved_destination_json, l.retrieval_terms_text,
	l.fingerprint, l.normalized_hash, l.idempotency_key,
	l.actor_json, l.revision, l.created_at, l.updated_at,
	bm25(learnings_fts) AS bm25_score
FROM learnings l
JOIN learnings_fts ON learnings_fts.learning_id = l.id
WHERE learnings_fts MATCH ? AND l.project_id = ?
ORDER BY bm25_score
LIMIT ?
`

// Repository wraps the storage layer with FTS5-aware retrieval.
type Repository struct {
	db        *storage.DB
	searchSQL string
}

// NewRepository returns a Repository bound to the supplied database.
// The caller is responsible for running storage.Migrate before
// constructing the Repository.
func NewRepository(db *storage.DB) *Repository {
	return &Repository{db: db, searchSQL: defaultSearchSQL}
}

// Search returns up to `limit` Candidates whose FTS5 row matches
// the supplied terms, scoped to projectID.
//
// An empty terms slice is a no-op: the function returns (nil, nil)
// because there is nothing to search for. A terms slice that
// produces zero FTS5 hits returns ([]Candidate{}, nil) — empty
// result is not an error.
//
// limit <= 0 is treated as DefaultLimit. The Repository does NOT
// apply the project's MaxLimit cap; the Service does, after
// ranking.
func (r *Repository) Search(ctx context.Context, projectID domain.ProjectID, terms []string, limit int) ([]Candidate, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	query := FTS5Query(terms)
	if query == "" {
		return nil, nil
	}
	rows, err := r.db.DB.QueryContext(ctx, r.searchSQL, query, string(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("retrieval: search: %w", err)
	}
	defer rows.Close()

	out := make([]Candidate, 0)
	for rows.Next() {
		c, scanErr := scanCandidate(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("retrieval: scan: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("retrieval: rows: %w", err)
	}
	return out, nil
}

// scanCandidate reads one row from the defaultSearchSQL shape into a
// Candidate. It mirrors storage.scanLearning but emits the BM25
// score as well. We duplicate the parse logic intentionally: the
// retrieval package is not allowed to depend on unexported storage
// internals, and a small amount of duplication is cheaper than a
// cross-package contract.
func scanCandidate(rows *sql.Rows) (Candidate, error) {
	var (
		c                       Candidate
		createdAt, updatedAt    string
		actorJSON               string
		approvedScope           *string
		idempotencyKey          *string
		recProcJSON             string
		retrievalJSON           string
		approvedDestJSON        string
		approvedDestJSONPresent bool
	)
	err := rows.Scan(
		(*string)(&c.ID),
		(*string)(&c.ProjectID),
		(*string)(&c.Status),
		(*string)(&c.Type),
		&c.Title,
		&c.Context,
		&c.Observation,
		&c.ReusableLesson,
		&recProcJSON,
		&c.Limits,
		(*string)(&c.ScopeGuess),
		&approvedScope,
		(*string)(&c.Confidence),
		(*string)(&c.EvidenceLevel),
		(*string)(&c.ProposedDestination),
		&approvedDestJSON,
		&retrievalJSON,
		&c.Fingerprint,
		&c.NormalizedHash,
		&idempotencyKey,
		&actorJSON,
		&c.Revision,
		&createdAt,
		&updatedAt,
		&c.BM25Score,
	)
	if err != nil {
		return Candidate{}, fmt.Errorf("retrieval: scan: %w", err)
	}
	c.CreatedAt, _ = parseTime(createdAt)
	c.UpdatedAt, _ = parseTime(updatedAt)
	c.Actor = parseActor(actorJSON)
	c.IdempotencyKey = idempotencyKey
	c.RetrievalTerms = unmarshalStringSlice(retrievalJSON)

	if approvedScope != nil {
		s := domain.Scope(*approvedScope)
		c.ApprovedScope = &s
	}

	if approvedDestJSON != "" {
		approvedDestJSONPresent = true
	}
	if approvedDestJSONPresent {
		var dest domain.Destination
		if err := json.Unmarshal([]byte(approvedDestJSON), &dest); err == nil {
			c.ApprovedDestination = &dest
		}
	}

	return c, nil
}

// parseTime parses the RFC3339 timestamp format storage uses. An
// unparseable timestamp leaves the time zero-valued — the Service
// can still rank the candidate (recency decay falls back to 0).
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// parseActor parses an Actor JSON blob. Failures leave a zero Actor;
// the Service never depends on Actor for ranking, so this is safe.
func parseActor(raw string) domain.Actor {
	if raw == "" || raw == "{}" {
		return domain.Actor{}
	}
	var a domain.Actor
	_ = json.Unmarshal([]byte(raw), &a)
	return a
}

// unmarshalStringSlice parses the JSON array storage uses to persist
// retrieval_terms. Empty / "[]" returns nil.
func unmarshalStringSlice(raw string) []string {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
