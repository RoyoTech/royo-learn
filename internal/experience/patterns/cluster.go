// Pure v1 clustering algorithm for Hito 6 slice 6.2.
//
// Clustering is the second stage of the pattern-mining pipeline
// (docs/23-PATTERN-MINING.md §4). The v1 algorithm is deliberately
// simple:
//
//   1. Partition by (kind, fingerprint): two events with the same
//      canonical fingerprint land in the same partition; two events
//      with different kinds or different fingerprints cannot merge.
//   2. Within each partition, split into chunks of at most
//      MaxClusterMembers (default 100). The chunk index is the
//      cluster's stable split suffix.
//   3. Cross-partition Jaccard fallback: partitions with different
//      fingerprints but overlapping retrieval terms above the
//      configured threshold (default 0.5, reversible) are merged into
//      a single cluster. The same-kind constraint is preserved; the
//      cap is preserved (a candidate merge that would exceed the cap
//      is rejected).
//
// The algorithm is pure (no I/O, no DB, no clock) and deterministic
// across candidate reorderings because:
//   - partition key is `(kind, fingerprint)`, hash-independent;
//   - chunk index is the position-in-partition, hash-independent;
//   - cross-partition Jaccard comparison uses sorted, deduplicated
//     retrieval terms (NormalizeRetrievalTerms sorts).
//
// It does NOT use embeddings or any vector-database API (AGENTS.md
// rule 9, docs/23-PATTERN-MINING.md §4).

package patterns

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// PatternCandidate is the in-memory event the clusterer groups. It
// carries the minimum metadata the algorithm needs (id, fingerprint,
// retrieval terms, session id, occurred-at) without touching the
// SQLite row.
type PatternCandidate struct {
	EventID        domain.ExperienceEventID
	ProjectID      domain.ProjectID
	Kind           domain.ExperienceEventKind
	Fingerprint    string
	Problem        string
	Tool           string
	Result         string
	RetrievalTerms []string
	SessionID      string
	OccurredAt     time.Time
}

// ClusterRecord is the output of Group. It is the in-memory
// representation the Qualifier consumes (slice 6.3). It mirrors the
// Cluster interface declared in patterns.go.
type ClusterRecord struct {
	Fingerprint        string
	Kind               domain.ExperienceEventKind
	Members            []domain.ExperienceEventID
	RetrievalTerms     []string
	Sessions           map[string]struct{}
	Days               map[string]struct{}
	DistinctSessions   int
	DistinctDays       int
	OccurrenceCount    int
	FirstSeenAt        time.Time
	LastSeenAt         time.Time
	SourceFingerprints []string
	// SuccessfulOutcomes is the count of "success"/"corrected" member
	// events. The Qualifier uses it for criterion B. Zero means "not
	// yet tallied" — the orchestrator is responsible for filling it
	// before calling Qualify.
	SuccessfulOutcomes int
	// RepeatedCorrection is true when an explicit repeated correction
	// (e.g. user said "no, do X" twice) was recorded for this pattern.
	// It satisfies criterion B without counting "success" outcomes.
	RepeatedCorrection bool
	// HasContradiction is true when a posterior contradiction has been
	// recorded for this cluster (criterion C).
	HasContradiction bool
	// CoveredByLearningID is non-empty when an existing Learning
	// already covers this cluster (criterion E). The value is the
	// LearningID.
	CoveredByLearningID string
}

// Group partitions candidates by (kind, fingerprint), splits each
// partition into chunks of at most MaxClusterMembers, and then merges
// partitions across different fingerprints when the Jaccard overlap
// of retrieval terms meets the configured threshold.
//
// The output is sorted by fingerprint so callers can iterate
// predictably. Clusters are never nil; an empty input returns an
// empty slice.
//
// Invalid configurations (negative Jaccard, non-positive cap) fail
// closed by returning an empty slice; callers validate up-front via
// Config.Validate when they want a typed error.
func Group(candidates []PatternCandidate, cfg Config) []ClusterRecord {
	if len(candidates) == 0 {
		return []ClusterRecord{}
	}
	if err := cfg.Validate(); err != nil {
		return []ClusterRecord{}
	}

	ordered := append([]PatternCandidate(nil), candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		if ordered[i].Fingerprint != ordered[j].Fingerprint {
			return ordered[i].Fingerprint < ordered[j].Fingerprint
		}
		if ordered[i].EventID != ordered[j].EventID {
			return ordered[i].EventID < ordered[j].EventID
		}
		if ordered[i].SessionID != ordered[j].SessionID {
			return ordered[i].SessionID < ordered[j].SessionID
		}
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})

	// Phase 1: partition by (kind, fingerprint) and split each
	// partition into chunks of at most MaxClusterMembers. The chunk
	// index is deterministic because partition iteration is over
	// the input order.
	type partition struct {
		key        string
		fp         string
		kind       domain.ExperienceEventKind
		candidates []PatternCandidate
	}
	var partitions []partition
	partitionIndex := make(map[string]int)
	for _, c := range ordered {
		key := string(c.Kind) + "\x1f" + c.Fingerprint
		idx, ok := partitionIndex[key]
		if !ok {
			idx = len(partitions)
			partitionIndex[key] = idx
			partitions = append(partitions, partition{key: key, fp: c.Fingerprint, kind: c.Kind})
		}
		partitions[idx].candidates = append(partitions[idx].candidates, c)
	}

	// Phase 2: split each partition into chunks of at most
	// MaxClusterMembers. Each chunk becomes one ClusterRecord. The
	// clusters carry their normalized retrieval terms and the metrics
	// derived from the chunk's members.
	var out []ClusterRecord
	for _, p := range partitions {
		for chunkStart := 0; chunkStart < len(p.candidates); chunkStart += cfg.MaxClusterMembers {
			chunkEnd := chunkStart + cfg.MaxClusterMembers
			if chunkEnd > len(p.candidates) {
				chunkEnd = len(p.candidates)
			}
			chunk := p.candidates[chunkStart:chunkEnd]
			out = append(out, buildClusterRecord(p.fp, p.kind, chunk))
		}
	}

	// Phase 3: cross-partition Jaccard fallback. For each pair of
	// distinct partitions (different fingerprint), if the same-kind
	// constraint holds and the Jaccard overlap meets the threshold
	// and the merged size would not exceed the cap, merge them. We
	// iterate until stable.
	merged := true
	for merged {
		merged = false
		for i := 0; i < len(out); i++ {
			if merged {
				break
			}
			for j := i + 1; j < len(out); j++ {
				if i >= len(out) || j >= len(out) {
					break
				}
				if shouldMergeClusters(out[i], out[j], cfg) {
					out[i] = mergeClusterRecords(out[i], out[j])
					out = append(out[:j], out[j+1:]...)
					merged = true
					break
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out
}

// buildClusterRecord constructs a single ClusterRecord from one
// partition chunk. The retrieval terms are normalized (sort, dedup,
// strip volatile content) so the fingerprint and Jaccard signals are
// computed over the canonical form.
func buildClusterRecord(fp string, kind domain.ExperienceEventKind, chunk []PatternCandidate) ClusterRecord {
	cr := ClusterRecord{
		Fingerprint:        fp,
		Kind:               kind,
		Members:            make([]domain.ExperienceEventID, 0, len(chunk)),
		RetrievalTerms:     nil,
		Sessions:           map[string]struct{}{},
		Days:               map[string]struct{}{},
		SourceFingerprints: make([]string, 0, len(chunk)),
	}
	termUnion := map[string]struct{}{}
	for _, c := range chunk {
		cr.Members = append(cr.Members, c.EventID)
		cr.SourceFingerprints = append(cr.SourceFingerprints, c.Fingerprint)
		switch strings.ToLower(strings.TrimSpace(c.Result)) {
		case "success", "successful", "corrected":
			cr.SuccessfulOutcomes++
		}
		if cr.FirstSeenAt.IsZero() || c.OccurredAt.Before(cr.FirstSeenAt) {
			cr.FirstSeenAt = c.OccurredAt
		}
		if c.OccurredAt.After(cr.LastSeenAt) {
			cr.LastSeenAt = c.OccurredAt
		}
		if c.SessionID != "" {
			cr.Sessions[c.SessionID] = struct{}{}
		}
		cr.Days[c.OccurredAt.UTC().Format("2006-01-02")] = struct{}{}
		for _, t := range NormalizeRetrievalTerms(c.RetrievalTerms) {
			termUnion[t] = struct{}{}
		}
	}
	terms := make([]string, 0, len(termUnion))
	for t := range termUnion {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	cr.RetrievalTerms = terms
	cr.DistinctSessions = len(cr.Sessions)
	cr.DistinctDays = len(cr.Days)
	sort.Slice(cr.Members, func(i, j int) bool { return cr.Members[i] < cr.Members[j] })
	sort.Strings(cr.SourceFingerprints)
	cr.OccurrenceCount = len(cr.Members)
	return cr
}

// shouldMergeClusters reports whether two clusters from different
// partitions (different fingerprint) should merge under the
// Jaccard fallback. The same-kind constraint, the conservative
// threshold, and the MaxClusterMembers cap are all preserved.
func shouldMergeClusters(a, b ClusterRecord, cfg Config) bool {
	if a.Fingerprint == b.Fingerprint {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	score := jaccard(a.RetrievalTerms, b.RetrievalTerms)
	if score < cfg.MinRetrievalJaccard {
		return false
	}
	if a.OccurrenceCount+b.OccurrenceCount > cfg.MaxClusterMembers {
		return false
	}
	return true
}

// mergeClusterRecords returns the union of two clusters under the
// canonical identity (fingerprint + kind) of the left operand. The
// retrieval terms, sessions, days and member list are merged.
func mergeClusterRecords(a, b ClusterRecord) ClusterRecord {
	out := a
	for _, m := range b.Members {
		out.Members = append(out.Members, m)
	}
	for s := range b.Sessions {
		out.Sessions[s] = struct{}{}
	}
	for d := range b.Days {
		out.Days[d] = struct{}{}
	}
	if b.FirstSeenAt.Before(out.FirstSeenAt) {
		out.FirstSeenAt = b.FirstSeenAt
	}
	if b.LastSeenAt.After(out.LastSeenAt) {
		out.LastSeenAt = b.LastSeenAt
	}
	termUnion := map[string]struct{}{}
	for _, t := range out.RetrievalTerms {
		termUnion[t] = struct{}{}
	}
	for _, t := range b.RetrievalTerms {
		termUnion[t] = struct{}{}
	}
	terms := make([]string, 0, len(termUnion))
	for t := range termUnion {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	out.RetrievalTerms = terms
	out.SourceFingerprints = append(out.SourceFingerprints, b.SourceFingerprints...)
	sort.Slice(out.Members, func(i, j int) bool { return out.Members[i] < out.Members[j] })
	sort.Strings(out.SourceFingerprints)
	out.DistinctSessions = len(out.Sessions)
	out.DistinctDays = len(out.Days)
	out.SuccessfulOutcomes += b.SuccessfulOutcomes
	out.OccurrenceCount = len(out.Members)
	return out
}

// jaccard returns |A ∩ B| / |A ∪ B| for two slices of normalized
// terms. Both inputs must already be sorted and deduplicated. Empty
// inputs return 0 so the conservative threshold is never bypassed.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// splitKey produces a deterministic cluster key by appending the
// partition index to the (kind, fingerprint) partition key. The
// partition index is the position in the partitions slice, which is
// stable for any given input order.
func splitKey(pKey string, chunkIndex int) string {
	return pKey + "#chunk-" + strconv.Itoa(chunkIndex)
}
