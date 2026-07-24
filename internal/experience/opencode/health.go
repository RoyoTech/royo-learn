package opencode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// requiredTables is the minimum schema set Health verifies against. Real
// OpenCode stores may carry additional tables; the adapter only gates on
// these two.
var requiredTables = []string{"sessions", "messages"}

// Health performs a read-only check on the candidate database at
// instance.DBPath. It opens the file in SQLite's "mode=ro" URL mode, runs
// a single sqlite_master query, and closes the handle before returning.
// The source database is never written to: the mtime test in
// TestHealth_NoSourceSideEffects would otherwise fail.
//
// Status mapping:
//
//   - "ok"        — readable and schema matches.
//   - "degraded"  — readable but schema is missing, file is missing,
//     path is a directory, or the file is not a SQLite database.
//   - "error"     — bad input (wrong source, empty path) or the caller's
//     context was cancelled.
//
// "degraded" is never a fatal error: callers continue ingestion of any
// other instance and report the missing store at the end of the run.
func (a *Adapter) Health(ctx context.Context, instance SourceInstance) HealthResult {
	now := a.now()
	if err := ctx.Err(); err != nil {
		return HealthResult{
			Status:    "error",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrTimeout),
			Message:   err.Error(),
			CheckedAt: now,
		}
	}
	if instance.Source != domain.SourceOpenCode {
		return HealthResult{
			Status:    "error",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrInvalidArgument),
			Message:   "opencode health: instance source is not opencode",
			CheckedAt: now,
		}
	}
	if strings.TrimSpace(instance.DBPath) == "" {
		return HealthResult{
			Status:    "error",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrInvalidArgument),
			Message:   "opencode health: instance DBPath is required",
			CheckedAt: now,
		}
	}

	info, err := os.Stat(instance.DBPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HealthResult{
				Status:    "degraded",
				DBPath:    instance.DBPath,
				Code:      string(domain.ErrExperienceSourceNotFound),
				Message:   "opencode health: source database file does not exist",
				CheckedAt: now,
			}
		}
		return HealthResult{
			Status:    "degraded",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode health: cannot stat source: %v", err),
			CheckedAt: now,
		}
	}
	if info.IsDir() {
		return HealthResult{
			Status:    "degraded",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   "opencode health: source path is a directory, not a database file",
			CheckedAt: now,
		}
	}

	if err := ctx.Err(); err != nil {
		return HealthResult{
			Status:    "error",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrTimeout),
			Message:   err.Error(),
			CheckedAt: now,
		}
	}

	db, openErr := sql.Open("sqlite", "file:"+instance.DBPath+"?mode=ro")
	if openErr != nil {
		return HealthResult{
			Status:    "degraded",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode health: cannot open source read-only: %v", openErr),
			CheckedAt: now,
		}
	}
	// Read-only handles cannot leak writes; closing explicitly is still the
	// safe default and matches the rest of the codebase.
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return HealthResult{
				Status:    "error",
				DBPath:    instance.DBPath,
				Code:      string(domain.ErrTimeout),
				Message:   err.Error(),
				CheckedAt: now,
			}
		}
		return HealthResult{
			Status:    "degraded",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode health: cannot ping source: %v", err),
			CheckedAt: now,
		}
	}

	if !verifyOpenCodeSchema(ctx, db) {
		return HealthResult{
			Status:    "degraded",
			DBPath:    instance.DBPath,
			Readable:  true,
			SchemaOK:  false,
			Code:      string(domain.ErrExperienceSchemaUnsupported),
			Message:   "opencode health: source database does not match the expected OpenCode schema",
			CheckedAt: now,
		}
	}

	return HealthResult{
		Status:    "ok",
		DBPath:    instance.DBPath,
		Readable:  true,
		SchemaOK:  true,
		CheckedAt: now,
	}
}

// verifyOpenCodeSchema queries sqlite_master for the required tables and
// returns true iff every required name is present. It runs a single
// SELECT and walks the rows; no writes occur.
func verifyOpenCodeSchema(ctx context.Context, db *sql.DB) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return false
	}
	defer rows.Close()
	present := make(map[string]struct{}, len(requiredTables))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false
		}
		present[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	for _, required := range requiredTables {
		if _, ok := present[required]; !ok {
			return false
		}
	}
	return true
}

// time alias used internally; kept private so package callers do not
// gain a time-time helper they should not depend on.
var _ = time.Time{}
