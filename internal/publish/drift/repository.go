// Repository persists and reads publication_drift_state rows. The
// recordDriftSQL constant is the single source of truth for the upsert
// statement; tests that need a custom path can pass a DriftRow with the
// RunID set to a deterministic value.

package drift

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// recordDriftSQL is the per-row upsert statement. The conflict target is
// the composite PRIMARY KEY (publication_id, target_path); every
// column except the primary key is overwritten on conflict.
const recordDriftSQL = `
INSERT INTO publication_drift_state
    (publication_id, source, target_path, expected_hash, actual_hash, status, checked_at, run_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(publication_id, target_path) DO UPDATE SET
    source         = excluded.source,
    expected_hash  = excluded.expected_hash,
    actual_hash    = excluded.actual_hash,
    status         = excluded.status,
    checked_at     = excluded.checked_at,
    run_id         = excluded.run_id`

// DriftRow is the row shape persisted in publication_drift_state. The
// Status field carries one of the four enum values (StatusOK,
// StatusDrifted, StatusTargetMissing, StatusTargetUnreadable). The
// CheckedAt timestamp is stored as RFC3339 UTC.
type DriftRow struct {
	PublicationID string
	Source        string
	TargetPath    string
	ExpectedHash  string
	ActualHash    string
	Status        Status
	CheckedAt     time.Time
	RunID         string
}

// Repository is the per-package handle for the publication_drift_state
// table. It owns a *sql.DB handle and the optional nowFn clock used by
// callers that want deterministic timestamps in tests.
type Repository struct {
	db   *sql.DB
	nowF func() time.Time
}

// NewRepository returns a Repository backed by db. The nowFn clock is
// optional; pass nil to use time.Now. The Repository does not own db;
// the caller manages its lifecycle.
func NewRepository(db *sql.DB, nowFn func() time.Time) *Repository {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &Repository{db: db, nowF: nowFn}
}

// RecordDrift upserts one row into publication_drift_state keyed by
// (publication_id, target_path). The method opens a short-lived tx per
// call (no project-level lock; drift detection is read-only on the
// target and writes only to publication_drift_state).
func (r *Repository) RecordDrift(ctx context.Context, row DriftRow) error {
	if row.CheckedAt.IsZero() {
		row.CheckedAt = r.nowF()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("drift.RecordDrift: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, recordDriftSQL,
		row.PublicationID,
		row.Source,
		row.TargetPath,
		row.ExpectedHash,
		row.ActualHash,
		string(row.Status),
		row.CheckedAt.UTC().Format(time.RFC3339),
		row.RunID,
	); err != nil {
		return fmt.Errorf("drift.RecordDrift: exec: %w", err)
	}
	return tx.Commit()
}

// ListFilter narrows a ListDrift query. Both fields are optional; an
// empty value disables the corresponding filter.
type ListFilter struct {
	Source string
	RunID  string
}

// ListDrift returns rows from publication_drift_state matching the
// filters, ordered by (checked_at DESC, publication_id ASC,
// target_path ASC) so callers can paginate deterministically.
func (r *Repository) ListDrift(ctx context.Context, f ListFilter) ([]DriftRow, error) {
	q := `SELECT publication_id, source, target_path, expected_hash, actual_hash, status, checked_at, run_id
          FROM publication_drift_state
          WHERE (? = '' OR source = ?)
            AND (? = '' OR run_id  = ?)
          ORDER BY checked_at DESC, publication_id ASC, target_path ASC`
	rows, err := r.db.QueryContext(ctx, q, f.Source, f.Source, f.RunID, f.RunID)
	if err != nil {
		return nil, fmt.Errorf("drift.ListDrift: query: %w", err)
	}
	defer rows.Close()

	var out []DriftRow
	for rows.Next() {
		var (
			row       DriftRow
			statusStr string
			checkedAt string
		)
		if err := rows.Scan(
			&row.PublicationID,
			&row.Source,
			&row.TargetPath,
			&row.ExpectedHash,
			&row.ActualHash,
			&statusStr,
			&checkedAt,
			&row.RunID,
		); err != nil {
			return nil, fmt.Errorf("drift.ListDrift: scan: %w", err)
		}
		row.Status = Status(statusStr)
		if t, parseErr := time.Parse(time.RFC3339, checkedAt); parseErr == nil {
			row.CheckedAt = t
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CountByStatus returns the number of rows per status for the supplied
// filter. Useful for the operator dashboard ("how many drifted in the
// last 24 h?"). Returned map keys are the four enum values; missing
// keys are reported as zero by the caller.
func (r *Repository) CountByStatus(ctx context.Context, f ListFilter) (map[Status]int, error) {
	q := `SELECT status, COUNT(*) FROM publication_drift_state
          WHERE (? = '' OR source = ?)
            AND (? = '' OR run_id  = ?)
          GROUP BY status`
	rows, err := r.db.QueryContext(ctx, q, f.Source, f.Source, f.RunID, f.RunID)
	if err != nil {
		return nil, fmt.Errorf("drift.CountByStatus: query: %w", err)
	}
	defer rows.Close()

	out := map[Status]int{
		StatusOK:               0,
		StatusDrifted:          0,
		StatusTargetMissing:    0,
		StatusTargetUnreadable: 0,
	}
	for rows.Next() {
		var (
			statusStr string
			n         int
		)
		if err := rows.Scan(&statusStr, &n); err != nil {
			return nil, fmt.Errorf("drift.CountByStatus: scan: %w", err)
		}
		out[Status(statusStr)] = n
	}
	return out, rows.Err()
}
