// Pattern Service for Hito 6 slice 6.4.
//
// The Service wraps the Repository with the typed dismissal flow and
// the read-side projections the CLI and MCP expose. It is the only
// surface the higher layers call into: promotion remains out of scope
// here and lives in Hito 7 via capture.Service.
//
// Idempotence:
//
//   - Dismiss: dismissing the same pattern with the same reason
//     returns nil and writes no extra audit row. Dismissing with a
//     different reason is rejected (caller should clarify the
//     previous dismissal first).
//   - List/Get: pure reads; no side effects.
//
// Errors:
//
//   - All typed errors surface as the package-level sentinels
//     (ErrPatternNotFound, ErrPatternNotQualified, etc.). Callers can
//     compare with errors.Is or with the canonical domain codes.

package patterns

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// Service is the orchestrator-facing surface for the patterns
// package. It owns the typed dismissal flow, the read-side
// projections and the audit trail.
type Service struct {
	repo *Repository
	db   *storage.DB
}

// NewService returns a Service bound to the supplied database.
func NewService(db *storage.DB) *Service {
	if db == nil {
		return &Service{repo: nil}
	}
	return &Service{repo: NewRepository(db), db: db}
}

// NewServiceFromRaw returns a Service bound to a raw *sql.DB. Used
// by CLI/MCP integration when the storage wrapper is not in scope.
func NewServiceFromRaw(raw *sql.DB) *Service {
	if raw == nil {
		return &Service{repo: nil}
	}
	return &Service{repo: NewRepositoryFromRaw(raw)}
}

// NewServiceWithRepository lets advanced callers inject a Repository
// directly. It is the constructor the slice 6.4 acceptance test
// uses.
func NewServiceWithRepository(repo *Repository) *Service {
	return &Service{repo: repo}
}

// DBForTest returns the *storage.DB the Service is bound to. It is
// reserved for the test fixtures that need to seed companion tables
// (experience_events, audit_events) before the membership FK
// constraint fires. Production code should never call this method.
func (s *Service) DBForTest() *storage.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Dismiss marks a pattern as Dismissed with a typed reason. The
// call is idempotent: dismissing the same pattern with the same
// reason returns nil and writes no extra audit row. A pattern that
// has already been promoted returns ErrPatternAlreadyPromoted and is
// NOT touched.
//
// The status update and the structured audit row commit atomically
// in a single SQLite transaction. If either step fails the whole
// transaction rolls back; the caller never sees a half-dismissed
// pattern. The audit row redacts the reviewer note when the reason
// is private_or_sensitive so sensitive text never lands in the audit
// trail (docs/24-EXPERIENCE-THREAT-MODEL.md §6).
func (s *Service) Dismiss(ctx context.Context, patternID domain.ExperiencePatternID, reason DismissalReason, details DismissalDetails) error {
	if s == nil || s.repo == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: service is not initialised")
	}
	if patternID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: pattern id is required")
	}
	if !isValidDismissalReason(reason) {
		return domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("patterns: invalid dismissal reason %q", reason))
	}
	if len(details.Note) > MaxDismissalNoteBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"patterns: dismissal note exceeds the permitted byte limit")
	}

	current, err := s.repo.GetByID(ctx, patternID)
	if err != nil {
		return err
	}
	if current.Status == PatternPromoted {
		return ErrPatternAlreadyPromoted
	}
	if current.Status == PatternDismissed && current.DismissalReason == reason {
		return nil
	}
	if current.Status == PatternDismissed && current.DismissalReason != reason {
		return domain.NewConflictError(domain.ErrPatternInsufficientSources,
			fmt.Sprintf("patterns: pattern was previously dismissed with reason %q; clarify before changing",
				current.DismissalReason))
	}

	if err := s.repo.DismissAtomic(ctx, patternID, reason, details, current); err != nil {
		return err
	}
	return nil
}

// List returns the patterns that match the filter in stable order.
func (s *Service) List(ctx context.Context, filter ListerFilter) ([]ExperiencePattern, error) {
	if filter.Status == "" {
		filter.Status = PatternObserved
	}
	if !isValidPatternStatus(filter.Status) {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("patterns: invalid status %q", filter.Status))
	}
	if filter.Project == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: project id is required")
	}
	out, err := s.repo.ListByStatus(ctx, filter.Project, filter.Status)
	if err != nil {
		return nil, err
	}
	if filter.Kind != "" {
		filtered := out[:0]
		for _, p := range out {
			if p.Kind == filter.Kind {
				filtered = append(filtered, p)
			}
		}
		out = filtered
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// Get returns a single pattern by id.
func (s *Service) Get(ctx context.Context, id domain.ExperiencePatternID) (*ExperiencePattern, error) {
	return s.repo.GetByID(ctx, id)
}

// IngestCluster is the slice 6.4 entry point the orchestrator uses:
// given a ClusterRecord produced by the clusterer + a qualifier
// decision, it persists the pattern, records the membership and
// returns the resulting ExperiencePattern.
func (s *Service) IngestCluster(ctx context.Context, projectID domain.ProjectID, cluster ClusterRecord, decision QualificationDecision) (*ExperiencePattern, error) {
	if projectID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: project id is required")
	}
	if cluster.OccurrenceCount == 0 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: empty cluster")
	}

	status := PatternObserved
	if decision.Status == PatternQualified {
		status = PatternQualified
	}

	pattern := ExperiencePattern{
		ProjectID:        projectID,
		Status:           status,
		Kind:             cluster.Kind,
		Fingerprint:      cluster.Fingerprint,
		Title:            "pattern " + safePrefix(cluster.Fingerprint, 16),
		Summary:          clusterSummary(cluster),
		DistinctSessions: cluster.DistinctSessions,
		DistinctDays:     cluster.DistinctDays,
		OccurrenceCount:  cluster.OccurrenceCount,
		FirstSeenAt:      cluster.FirstSeenAt,
		LastSeenAt:       cluster.LastSeenAt,
		DetectorVersion:  "v1",
		InputDigest:      digestCluster(cluster),
	}
	if pattern.FirstSeenAt.IsZero() {
		pattern.FirstSeenAt = time.Now().UTC()
	}
	if pattern.LastSeenAt.IsZero() {
		pattern.LastSeenAt = pattern.FirstSeenAt
	}

	return s.repo.SavePatternWithMembers(ctx, pattern, cluster.Members)
}

// --- helpers ---

func isValidDismissalReason(r DismissalReason) bool {
	switch r {
	case DismissalOneOff, DismissalNotReusable, DismissalAlreadyCovered,
		DismissalContradicted, DismissalInsufficientEvidence,
		DismissalPrivateOrSensitive, DismissalFalseCluster:
		return true
	}
	return false
}

// safePrefix returns the first n bytes of s, or the full string if
// it is shorter. Byte-based so the title can never overflow the
// configured limit regardless of UTF-8 multibyte characters.
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// clusterSummary produces the human-readable summary stored on the
// pattern. It is bounded so the row cannot grow without limit.
func clusterSummary(c ClusterRecord) string {
	out := fmt.Sprintf("observed=%d distinct_sessions=%d distinct_days=%d",
		c.OccurrenceCount, c.DistinctSessions, c.DistinctDays)
	if len(out) > MaxSummaryBytes {
		out = out[:MaxSummaryBytes]
	}
	return out
}

// MaxSummaryBytes bounds the stored summary text.
const MaxSummaryBytes = 256

// digestCluster produces a stable sha256 hex digest over the
// canonical bytes of the cluster's identity. The fingerprint is the
// cluster's canonical identity; the digest is the per-write content
// hash that invalidates stale patterns when the algorithm version
// changes. Both are derived from the same canonical form so they
// stay aligned.
func digestCluster(c ClusterRecord) string {
	retrievalTerms := append([]string(nil), c.RetrievalTerms...)
	sort.Strings(retrievalTerms)
	sourceFingerprints := append([]string(nil), c.SourceFingerprints...)
	sort.Strings(sourceFingerprints)
	members := append([]domain.ExperienceEventID(nil), c.Members...)
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
	payload := struct {
		Fingerprint        string                     `json:"fingerprint"`
		Kind               string                     `json:"kind"`
		OccurrenceCount    int                        `json:"occurrence_count"`
		DistinctSessions   int                        `json:"distinct_sessions"`
		DistinctDays       int                        `json:"distinct_days"`
		RetrievalTerms     []string                   `json:"retrieval_terms"`
		Members            []domain.ExperienceEventID `json:"members"`
		SourceFingerprints []string                   `json:"source_fingerprints"`
	}{
		Fingerprint:        c.Fingerprint,
		Kind:               string(c.Kind),
		OccurrenceCount:    c.OccurrenceCount,
		DistinctSessions:   c.DistinctSessions,
		DistinctDays:       c.DistinctDays,
		RetrievalTerms:     retrievalTerms,
		SourceFingerprints: sourceFingerprints,
		Members:            members,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// json.Marshal of a fixed struct shape cannot fail at runtime.
		// We surface a deterministic fallback so the caller still gets
		// a 64-char hex digest even on the impossible path.
		encoded = []byte(strconv.Itoa(c.OccurrenceCount) + ":" + c.Fingerprint)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// redactedNote returns the value of the note that the audit row
// stores. For DismissalPrivateOrSensitive the note is dropped so the
// sensitive text never lands in the audit sink (docs/24 §6); for any
// other reason the bounded note is stored verbatim.
func redactedNote(reason DismissalReason, note string) string {
	if reason == DismissalPrivateOrSensitive {
		return ""
	}
	return note
}

// suppressionMarker is the deterministic token written to the audit
// row when a private_or_sensitive dismissal redacts the reviewer
// note. It is the only value an operator can grep for to confirm
// suppression actually happened.
const suppressionMarker = "[note redacted: private_or_sensitive]"

// suppressionNote returns the marker used in audit summaries when
// the note was redacted. Exposed for the test surface.
func suppressionNote() string { return suppressionMarker }

// auditDetails builds the structured details map the dismissal
// audit row stores. The note key is either the reviewer note (any
// reason except private_or_sensitive) or the deterministic
// suppression marker (private_or_sensitive).
func auditDetails(previous *ExperiencePattern, reason DismissalReason, details DismissalDetails) map[string]any {
	noteValue := redactedNote(reason, details.Note)
	if reason == DismissalPrivateOrSensitive {
		noteValue = suppressionMarker
	}
	return map[string]any{
		"project_id":   string(previous.ProjectID),
		"reason":       string(reason),
		"note":         noteValue,
		"prior_status": string(previous.Status),
	}
}
