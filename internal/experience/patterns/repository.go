// Repository for Hito 6 slice 6.4 (migration 005).
//
// The repository owns every read and write against the
// experience_patterns and experience_pattern_members tables. It is
// the only surface that should call into the storage layer directly;
// the higher-level Service (slice 6.4) wraps it with the typed
// dismissal flow.
//
// All methods are pure with respect to the cluster/mining algorithm:
// they take ready-built values and persist them. The fingerprint is
// the cluster's identity; (project_id, fingerprint) is the unique
// key. Membership rows are unique on (pattern_id, event_id).
//
// Idempotency:
//
//   - SavePattern: re-saving the same (project_id, fingerprint)
//     updates the existing row and bumps Revision. It does NOT
//     create a duplicate. If the new pattern has a newer
//     DetectorVersion than the stored one, the stored row is
//     marked stale and a fresh row is not created (docs/24 §3 T12).
//   - AddMember: re-adding the same (pattern_id, event_id) is a
//     no-op (UNIQUE constraint).
//
// Concurrency:
//
//   - SetStatusWithReason bumps Revision and re-reads the row
//     atomically. A caller that holds the pre-update Revision will
//     fail on optimistic-lock contention.

package patterns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"github.com/google/uuid"
)

// Repository wraps the storage layer with pattern-aware persistence.
type Repository struct {
	db  *storage.DB
	raw *sql.DB
}

// NewRepository returns a Repository bound to the supplied database.
// The constructor does NOT run migrations; the caller is responsible
// for storage.Migrate during application startup.
func NewRepository(db *storage.DB) *Repository {
	return &Repository{db: db}
}

// NewRepositoryFromRaw returns a Repository bound to a raw *sql.DB.
// The CLI/MCP use this when they already have a *sql.DB in scope and
// do not need the *storage.DB wrapper.
func NewRepositoryFromRaw(raw *sql.DB) *Repository {
	return &Repository{raw: raw}
}

// SavePattern persists a pattern idempotently. The caller supplies a
// fully-populated ExperiencePattern (including a non-empty ID and
// CreatedAt); the repository sets the UpdatedAt and Revision on
// create, or advances Revision on re-save.
func (r *Repository) SavePattern(ctx context.Context, p ExperiencePattern) (*ExperiencePattern, error) {
	if err := r.validatePattern(&p); err != nil {
		return nil, err
	}
	if p.ID == "" {
		p.ID = newPatternID()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	existing, err := findPatternByFingerprintTx(ctx, tx, p.ProjectID, p.Fingerprint)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		p.Revision = 1
		if err := insertPatternTx(ctx, tx, &p); err != nil {
			return nil, err
		}
	} else if detectorVersionIsNewer(p.DetectorVersion, existing.DetectorVersion) {
		// docs/24 §3 T12: the stored row's detector_version is
		// NEWER than the candidate. Mark the stored row stale so a
		// reviewer can re-evaluate the pattern under the new
		// algorithm; the new candidate is rejected (returning the
		// stale row, not the candidate).
		if _, err := tx.ExecContext(ctx, `
			UPDATE experience_patterns
			SET status = ?, dismissal_reason = ?, updated_at = ?, revision = revision + 1
			WHERE id = ? AND revision = ?
		`, string(PatternStale), "", formatTime(time.Now().UTC()), string(existing.ID), existing.Revision); err != nil {
			return nil, fmt.Errorf("patterns: mark stale: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("patterns: commit stale: %w", err)
		}
		stale, err := r.GetByID(ctx, existing.ID)
		if err != nil {
			return nil, err
		}
		return stale, nil
	} else {
		p.ID = existing.ID
		p.CreatedAt = existing.CreatedAt
		p.Revision = existing.Revision + 1
		if err := updatePatternOnResaveTx(ctx, tx, &p, existing.Revision); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("patterns: commit: %w", err)
	}
	out := p
	return &out, nil
}

// detectorVersionIsNewer reports whether the stored detector_version
// is strictly newer than the candidate's. Both must be non-empty;
// the comparison is semver-ish ("major.minor.patch" with optional
// pre-release tags). Empty stored versions are treated as the
// oldest so a first-write always wins.
func detectorVersionIsNewer(candidate, stored string) bool {
	if candidate == "" {
		return false
	}
	if stored == "" {
		return true
	}
	// Try strict semver first; fall back to plain string compare so a
	// non-semver candidate does not silently win.
	if candidateMajor, candidateMinor, candidatePatch, ok := parseSemver(candidate); ok {
		if storedMajor, storedMinor, storedPatch, storedOK := parseSemver(stored); storedOK {
			if candidateMajor != storedMajor {
				return candidateMajor > storedMajor
			}
			if candidateMinor != storedMinor {
				return candidateMinor > storedMinor
			}
			return candidatePatch > storedPatch
		}
	}
	return candidate > stored
}

// parseSemver extracts (major, minor, patch) from a "X.Y.Z" string.
// Returns ok=false for non-semver strings so the caller can fall
// back to a plain comparison.
func parseSemver(s string) (int, int, int, bool) {
	var maj, min, pat int
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &maj); err != nil {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &min); err != nil {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &pat); err != nil {
		return 0, 0, 0, false
	}
	return maj, min, pat, true
}

// newPatternID returns a process-unique pattern ID using the same
// UUIDv7 convention as the rest of the project. UUIDv7 is
// time-ordered (timestamp embedded in the first 48 bits) so the IDs
// are also k-sortable, which makes the patterns table's PK index
// friendlier than a random UUIDv4.
func newPatternID() domain.ExperiencePatternID {
	return domain.ExperiencePatternID(uuid.Must(uuid.NewV7()).String())
}

// UpsertFromCluster is the higher-level convenience entry point the
// mining job uses. It derives a stable ID from (project_id,
// fingerprint) when one is not supplied, ensures Status is
// PatternObserved on first save and PatternQualified on the
// subsequent calls when the Qualifier says so.
func (r *Repository) UpsertFromCluster(ctx context.Context, p ExperiencePattern) error {
	if p.Status == "" {
		p.Status = PatternObserved
	}
	if _, err := r.SavePattern(ctx, p); err != nil {
		return err
	}
	return nil
}

// AddMember adds an event to the pattern's membership table. The
// call is idempotent: re-adding the same event returns the existing
// membership row unchanged.
func (r *Repository) AddMember(
	ctx context.Context,
	patternID domain.ExperiencePatternID,
	eventID domain.ExperienceEventID,
	similarityKind string,
	similarityScore float64,
	addedAt time.Time,
) (*Membership, error) {
	if patternID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: pattern id is required")
	}
	if eventID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: event id is required")
	}
	if similarityKind == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: similarity kind is required")
	}
	if similarityScore < 0 || similarityScore > 1 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: similarity score is outside [0,1]")
	}
	if addedAt.IsZero() {
		addedAt = time.Now().UTC()
	}

	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Ensure the pattern exists.
	var found int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM experience_patterns WHERE id = ?`, string(patternID)).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPatternNotFound
		}
		return nil, fmt.Errorf("patterns: lookup pattern: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO experience_pattern_members
			(pattern_id, event_id, similarity_kind, similarity_score, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, string(patternID), string(eventID), similarityKind, similarityScore, formatTime(addedAt)); err != nil {
		return nil, fmt.Errorf("patterns: insert member: %w", err)
	}

	row := tx.QueryRowContext(ctx, `
		SELECT similarity_kind, similarity_score, added_at
		FROM experience_pattern_members
		WHERE pattern_id = ? AND event_id = ?
	`, string(patternID), string(eventID))

	var m Membership
	var addedAtStr string
	if err := row.Scan(&m.SimilarityKind, &m.SimilarityScore, &addedAtStr); err != nil {
		return nil, fmt.Errorf("patterns: scan member: %w", err)
	}
	m.EventID = eventID
	parsed, perr := parseTime(addedAtStr)
	if perr != nil {
		return nil, perr
	}
	m.AddedAt = parsed

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("patterns: commit: %w", err)
	}
	return &m, nil
}

// GetByFingerprint returns the pattern with the supplied
// (project_id, fingerprint), or ErrPatternNotFound.
func (r *Repository) GetByFingerprint(ctx context.Context, projectID domain.ProjectID, fingerprint string) (*ExperiencePattern, error) {
	if projectID == "" || fingerprint == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: project_id and fingerprint are required")
	}
	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	pattern, err := findPatternByFingerprintTx(ctx, tx, projectID, fingerprint)
	if err != nil {
		return nil, err
	}
	if pattern == nil {
		return nil, ErrPatternNotFound
	}
	return pattern, nil
}

// GetByID returns the pattern with the supplied id, or
// ErrPatternNotFound.
func (r *Repository) GetByID(ctx context.Context, id domain.ExperiencePatternID) (*ExperiencePattern, error) {
	if id == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: id is required")
	}
	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	row := tx.QueryRowContext(ctx, `SELECT `+patternColumns+` FROM experience_patterns WHERE id = ?`, string(id))
	pattern, err := scanPattern(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPatternNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("patterns: get by id: %w", err)
	}
	return pattern, nil
}

// ListByStatus returns the patterns that match (project_id, status),
// in stable order (last_seen_at DESC, id ASC).
func (r *Repository) ListByStatus(ctx context.Context, projectID domain.ProjectID, status PatternStatus) ([]ExperiencePattern, error) {
	if projectID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: project_id is required")
	}
	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `SELECT `+patternColumns+`
		FROM experience_patterns
		WHERE project_id = ? AND status = ?
		ORDER BY last_seen_at DESC, id ASC`,
		string(projectID), string(status))
	if err != nil {
		return nil, fmt.Errorf("patterns: list by status: %w", err)
	}
	defer rows.Close()

	var out []ExperiencePattern
	for rows.Next() {
		p, err := scanPattern(rows)
		if err != nil {
			return nil, fmt.Errorf("patterns: scan: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ExperiencePattern{}
	}
	return out, nil
}

// SetStatus transitions the pattern to a new status and bumps
// Revision atomically. The caller supplies the current Revision as a
// CAS guard; a stale call returns ErrPatternNotFound or
// ErrPatternInsufficientSources depending on the failure mode.
func (r *Repository) SetStatus(ctx context.Context, id domain.ExperiencePatternID, status PatternStatus) (*ExperiencePattern, error) {
	return r.SetStatusWithReason(ctx, id, status, "")
}

// DismissAtomic combines the status update and the audit row into a
// single SQLite transaction so the pattern status and the audit
// evidence either both commit or both roll back. The audit row uses
// the dismissal reason + a redacted note when the reason is
// private_or_sensitive. Only the Service.Dismiss path calls this;
// raw callers should go through Dismiss instead.
func (r *Repository) DismissAtomic(
	ctx context.Context,
	id domain.ExperiencePatternID,
	reason DismissalReason,
	details DismissalDetails,
	previous *ExperiencePattern,
) error {
	if previous == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: DismissAtomic requires previous row")
	}
	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var currentRevision int
	if err := tx.QueryRowContext(ctx,
		`SELECT revision FROM experience_patterns WHERE id = ?`,
		string(id)).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPatternNotFound
		}
		return fmt.Errorf("patterns: lookup revision: %w", err)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_patterns
		SET status = ?, dismissal_reason = ?, updated_at = ?, revision = revision + 1
		WHERE id = ? AND revision = ?
	`, string(PatternDismissed), string(reason), formatTime(now), string(id), currentRevision)
	if err != nil {
		return fmt.Errorf("patterns: update status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("patterns: rows affected: %w", err)
	}
	if rows != 1 {
		return ErrPatternInsufficientSources
	}

	// Audit row in the SAME transaction so the status change and
	// the evidence commit or roll back together. The note field is
	// redacted for private_or_sensitive so sensitive text never
	// lands in the audit sink.
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         details.Actor,
		Operation:     "experience_pattern_dismissed",
		EntityType:    "experience_pattern",
		EntityID:      string(id),
		PayloadSHA256: previous.Fingerprint,
		Result:        "success",
		Details:       auditDetails(previous, reason, details),
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("patterns: commit: %w", err)
	}
	return nil
}

// SetStatusWithReason behaves like SetStatus but also stamps the
// dismissal_reason column. It is the entry point the Dismiss service
// uses so the (pattern_id, reason) idempotency check has access to
// the previously stored reason.
func (r *Repository) SetStatusWithReason(ctx context.Context, id domain.ExperiencePatternID, status PatternStatus, reason DismissalReason) (*ExperiencePattern, error) {
	if id == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: id is required")
	}
	if !isValidPatternStatus(status) {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("patterns: invalid status %q", status))
	}
	if status != PatternDismissed && reason != "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: dismissal reason requires PatternDismissed status")
	}

	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var currentRevision int
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM experience_patterns WHERE id = ?`, string(id)).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPatternNotFound
		}
		return nil, fmt.Errorf("patterns: lookup revision: %w", err)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_patterns
		SET status = ?, dismissal_reason = ?, updated_at = ?, revision = revision + 1
		WHERE id = ? AND revision = ?
	`, string(status), string(reason), formatTime(now), string(id), currentRevision)
	if err != nil {
		return nil, fmt.Errorf("patterns: update status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("patterns: rows affected: %w", err)
	}
	if rows != 1 {
		return nil, ErrPatternInsufficientSources
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("patterns: commit: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Members returns the membership rows for the pattern, sorted by
// added_at ASC then event_id ASC for stable output.
func (r *Repository) Members(ctx context.Context, id domain.ExperiencePatternID) ([]Membership, error) {
	if id == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "patterns: id is required")
	}
	tx, err := r.resolveTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
		SELECT event_id, similarity_kind, similarity_score, added_at
		FROM experience_pattern_members
		WHERE pattern_id = ?
		ORDER BY added_at ASC, event_id ASC
	`, string(id))
	if err != nil {
		return nil, fmt.Errorf("patterns: members: %w", err)
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		var m Membership
		var addedAtStr string
		if err := rows.Scan((*string)(&m.EventID), &m.SimilarityKind, &m.SimilarityScore, &addedAtStr); err != nil {
			return nil, fmt.Errorf("patterns: scan member: %w", err)
		}
		parsed, perr := parseTime(addedAtStr)
		if perr != nil {
			return nil, perr
		}
		m.AddedAt = parsed
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Membership{}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return string(out[i].EventID) < string(out[j].EventID)
		}
		return out[i].AddedAt.Before(out[j].AddedAt)
	})
	return out, nil
}

// validatePattern enforces the documented invariants before any DB
// write happens. Failing fast at the repository boundary matches the
// detector constructor convention.
func (r *Repository) validatePattern(p *ExperiencePattern) error {
	if p == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: pattern is nil")
	}
	if p.ProjectID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: project_id is required")
	}
	if len(string(p.ProjectID)) > domain.MaxExperienceIDBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "patterns: project_id exceeds the permitted byte limit")
	}
	if p.Fingerprint == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "patterns: fingerprint is required")
	}
	if len(p.Fingerprint) > domain.MaxExperienceDigestBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "patterns: fingerprint exceeds the permitted byte limit")
	}
	if !isValidPatternStatus(p.Status) {
		return domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("patterns: invalid status %q", p.Status))
	}
	if !domain.IsValidExperienceEventKindString(string(p.Kind)) {
		return domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("patterns: invalid event kind %q", p.Kind))
	}
	for field, value := range map[string]string{
		"title":            p.Title,
		"summary":          p.Summary,
		"detector_version": p.DetectorVersion,
		"input_digest":     p.InputDigest,
	} {
		limit := domain.MaxExperienceSummaryBytes
		if field == "detector_version" || field == "input_digest" {
			limit = domain.MaxExperienceMetadataBytes
		}
		if len(value) > limit {
			return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
				fmt.Sprintf("patterns: %s exceeds the permitted byte limit", field))
		}
	}
	return nil
}

func isValidPatternStatus(s PatternStatus) bool {
	switch s {
	case PatternObserved, PatternQualified, PatternDismissed, PatternPromoted, PatternStale:
		return true
	}
	return false
}

// patternColumns is the canonical SELECT list for ExperiencePattern.
// Order matches the scanPattern columns.
const patternColumns = `id, project_id, status, kind, fingerprint, title, summary,
	distinct_sessions, distinct_days, occurrence_count,
	first_seen_at, last_seen_at, proposed_learning_id,
	detector_version, input_digest, created_at, updated_at, revision,
	dismissal_reason`

func insertPatternTx(ctx context.Context, tx *sql.Tx, p *ExperiencePattern) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO experience_patterns
			(id, project_id, status, kind, fingerprint, title, summary,
			 distinct_sessions, distinct_days, occurrence_count,
			 first_seen_at, last_seen_at, proposed_learning_id,
			 detector_version, input_digest, created_at, updated_at, revision,
			 dismissal_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(p.ID), string(p.ProjectID), string(p.Status), string(p.Kind),
		p.Fingerprint, p.Title, p.Summary,
		p.DistinctSessions, p.DistinctDays, p.OccurrenceCount,
		formatTime(p.FirstSeenAt), formatTime(p.LastSeenAt),
		nullableLearningID(p.ProposedLearningID),
		p.DetectorVersion, p.InputDigest,
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt), p.Revision,
		string(p.DismissalReason))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrPatternAlreadyPromoted,
				"patterns: pattern identity already exists")
		}
		return fmt.Errorf("patterns: insert: %w", err)
	}
	return nil
}

func updatePatternOnResaveTx(ctx context.Context, tx *sql.Tx, p *ExperiencePattern, expectedRevision int) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_patterns
		SET status = ?, dismissal_reason = ?, kind = ?, title = ?, summary = ?,
		    distinct_sessions = ?, distinct_days = ?, occurrence_count = ?,
		    first_seen_at = ?, last_seen_at = ?,
		    detector_version = ?, input_digest = ?,
		    updated_at = ?, revision = ?
		WHERE project_id = ? AND fingerprint = ? AND revision = ?
	`, string(p.Status), string(p.DismissalReason), string(p.Kind), p.Title, p.Summary,
		p.DistinctSessions, p.DistinctDays, p.OccurrenceCount,
		formatTime(p.FirstSeenAt), formatTime(p.LastSeenAt),
		p.DetectorVersion, p.InputDigest,
		formatTime(p.UpdatedAt), p.Revision,
		string(p.ProjectID), p.Fingerprint, expectedRevision)
	if err != nil {
		return fmt.Errorf("patterns: update: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("patterns: rows affected: %w", err)
	}
	if rows != 1 {
		return domain.NewConflictError(domain.ErrPatternInsufficientSources,
			"patterns: pattern revision is stale")
	}
	return nil
}

func findPatternByFingerprintTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, fingerprint string) (*ExperiencePattern, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+patternColumns+`
		FROM experience_patterns WHERE project_id = ? AND fingerprint = ?`,
		string(projectID), fingerprint)
	pattern, err := scanPattern(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("patterns: find by fingerprint: %w", err)
	}
	return pattern, nil
}

// resolveTx returns a *sql.Tx that uses the underlying connection
// regardless of whether the Repository was constructed from a
// *storage.DB or a raw *sql.DB. It centralises the dispatch so the
// repository methods stay readable.
func (r *Repository) resolveTx(ctx context.Context, writable bool) (*sql.Tx, error) {
	if r.db != nil && r.db.DB != nil {
		return r.db.DB.BeginTx(ctx, nil)
	}
	if r.raw != nil {
		return r.raw.BeginTx(ctx, nil)
	}
	return nil, fmt.Errorf("patterns: no database connection")
}

// patternRow is the contract scanPattern accepts; both *sql.Row and
// *sql.Rows satisfy it.
type patternRow interface {
	Scan(dest ...any) error
}

func scanPattern(row patternRow) (*ExperiencePattern, error) {
	var (
		p                       ExperiencePattern
		proposedLearningID      sql.NullString
		firstSeenAt, lastSeenAt string
		createdAt, updatedAt    string
	)
	if err := row.Scan(
		(*string)(&p.ID), (*string)(&p.ProjectID), (*string)(&p.Status), (*string)(&p.Kind),
		&p.Fingerprint, &p.Title, &p.Summary,
		&p.DistinctSessions, &p.DistinctDays, &p.OccurrenceCount,
		&firstSeenAt, &lastSeenAt, &proposedLearningID,
		&p.DetectorVersion, &p.InputDigest,
		&createdAt, &updatedAt, &p.Revision,
		&p.DismissalReason,
	); err != nil {
		return nil, err
	}
	if proposedLearningID.Valid {
		id := domain.LearningID(proposedLearningID.String)
		p.ProposedLearningID = &id
	}
	if t, err := parseTime(firstSeenAt); err == nil {
		p.FirstSeenAt = t
	} else {
		return nil, err
	}
	if t, err := parseTime(lastSeenAt); err == nil {
		p.LastSeenAt = t
	} else {
		return nil, err
	}
	if t, err := parseTime(createdAt); err == nil {
		p.CreatedAt = t
	} else {
		return nil, err
	}
	if t, err := parseTime(updatedAt); err == nil {
		p.UpdatedAt = t
	} else {
		return nil, err
	}
	return &p, nil
}

func nullableLearningID(id *domain.LearningID) any {
	if id == nil {
		return nil
	}
	return string(*id)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed") || contains(msg, "constraint failed: UNIQUE")
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// MarshalStable produces a stable JSON encoding for log/audit output.
// The encoding is field-stable: future migrations may add fields
// and the encoding stays backward-compatible. The function is
// reserved for the few callers that need explicit byte output
// (audit pipelines, log appenders). JSON-string consumers should
// keep using the standard library directly.
func (p ExperiencePattern) MarshalStable() ([]byte, error) {
	type alias ExperiencePattern
	return json.Marshal(alias(p))
}

// SavePatternWithMembers persists pattern state and traceable memberships in one
// transaction. A dismissed pattern remains suppressed for the same detector and
// member set; only a qualified decision with additional traceable evidence may
// reopen it.
func (r *Repository) SavePatternWithMembers(ctx context.Context, p ExperiencePattern, memberIDs []domain.ExperienceEventID) (*ExperiencePattern, error) {
	if err := r.validatePattern(&p); err != nil {
		return nil, err
	}
	members := append([]domain.ExperienceEventID(nil), memberIDs...)
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
	members = compactMemberIDs(members)
	if len(members) == 0 {
		return nil, domain.NewValidationError(domain.ErrPatternInsufficientSources, "patterns: at least one traceable member is required")
	}
	if p.ID == "" {
		p.ID = newPatternID()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	tx, err := r.resolveTx(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	existing, err := findPatternByFingerprintTx(ctx, tx, p.ProjectID, p.Fingerprint)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		p.Revision = 1
		if err := insertPatternTx(ctx, tx, &p); err != nil {
			return nil, err
		}
	} else if detectorVersionIsNewer(p.DetectorVersion, existing.DetectorVersion) {
		if _, err := tx.ExecContext(ctx, `UPDATE experience_patterns
			SET status = ?, dismissal_reason = '', updated_at = ?, revision = revision + 1
			WHERE id = ? AND revision = ?`, string(PatternStale), formatTime(now), string(existing.ID), existing.Revision); err != nil {
			return nil, fmt.Errorf("patterns: mark stale: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("patterns: commit stale: %w", err)
		}
		return r.GetByID(ctx, existing.ID)
	} else {
		existingMembers, err := memberIDsTx(ctx, tx, existing.ID)
		if err != nil {
			return nil, err
		}
		hasNewEvidence := !sameMemberIDs(existingMembers, members)
		if existing.Status == PatternDismissed && (existing.DetectorVersion == p.DetectorVersion && !hasNewEvidence || p.Status != PatternQualified) {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("patterns: commit suppression read: %w", err)
			}
			return existing, nil
		}
		p.ID = existing.ID
		p.CreatedAt = existing.CreatedAt
		if existing.FirstSeenAt.Before(p.FirstSeenAt) {
			p.FirstSeenAt = existing.FirstSeenAt
		}
		p.DismissalReason = ""
		p.Revision = existing.Revision + 1
		if err := updatePatternOnResaveTx(ctx, tx, &p, existing.Revision); err != nil {
			return nil, err
		}
	}

	for _, eventID := range members {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO experience_pattern_members
			(pattern_id, event_id, similarity_kind, similarity_score, added_at)
			VALUES (?, ?, ?, ?, ?)`, string(p.ID), string(eventID), "exact_fingerprint", 1.0, formatTime(now)); err != nil {
			return nil, fmt.Errorf("patterns: insert member: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("patterns: commit: %w", err)
	}
	return &p, nil
}

func memberIDsTx(ctx context.Context, tx *sql.Tx, patternID domain.ExperiencePatternID) ([]domain.ExperienceEventID, error) {
	rows, err := tx.QueryContext(ctx, `SELECT event_id FROM experience_pattern_members WHERE pattern_id = ? ORDER BY event_id`, string(patternID))
	if err != nil {
		return nil, fmt.Errorf("patterns: list member ids: %w", err)
	}
	defer rows.Close()
	var ids []domain.ExperienceEventID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("patterns: scan member id: %w", err)
		}
		ids = append(ids, domain.ExperienceEventID(id))
	}
	return ids, rows.Err()
}

func compactMemberIDs(ids []domain.ExperienceEventID) []domain.ExperienceEventID {
	out := ids[:0]
	for _, id := range ids {
		if id == "" || len(out) > 0 && out[len(out)-1] == id {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sameMemberIDs(a, b []domain.ExperienceEventID) bool {
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
