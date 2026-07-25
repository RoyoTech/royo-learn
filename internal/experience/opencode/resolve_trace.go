package opencode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
)

// defaultTraceMaxBytes bounds the trace excerpt when the caller does not
// pin a value. 1 KiB is far below the 1 MiB response ceiling mandated by
// the experience threat model (docs/24-EXPERIENCE-THREAT-MODEL.md §4) and
// is enough for a useful preview without disclosing an entire turn.
const defaultTraceMaxBytes = 1024

// TraceExcerptSuffix marks an excerpt that was truncated to honour the
// bounds. The suffix itself is part of the bounded output; the caller
// decides whether to surface the marker.
const TraceExcerptSuffix = "..."

// ResolveTrace returns a bounded, redacted excerpt for the locator and
// bounds requested. The adapter never returns full transcript content
// without an explicit excerpt flag (docs/22-ADAPTER-CONTRACT.md §2 —
// "El adaptador NO puede"); the only output is an excerpt capped at
// bounds.MaxBytes and run through evidence.Redact.
//
// Code values produced:
//
//   - ""                       — success, excerpt is fresh and authorized.
//   - "trace_source_changed"   — SourceHash on the locator no longer
//     matches the source DB; the caller should treat the excerpt as
//     advisory only.
//   - "trace_source_unavailable" — the source DB is unreadable or the
//     referenced turn does not exist anymore.
//   - "experience_locator_invalid" — locator fields fail validation
//     before any source I/O.
//   - "experience_source_not_found" — the source file disappeared.
//   - "experience_source_schema_unsupported" — schema mismatch.
//   - "timeout"                 — context cancellation or deadline.
func (a *Adapter) ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult {
	if err := ctx.Err(); err != nil {
		return TraceResult{
			Code:    string(domain.ErrTimeout),
			Message: err.Error(),
		}
	}
	if locator.Kind != "sqlite" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "opencode trace: locator kind must be sqlite",
		}
	}
	if strings.TrimSpace(locator.Path) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "opencode trace: locator path is required",
		}
	}
	if strings.TrimSpace(locator.TurnID) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "opencode trace: locator turn id is required",
		}
	}

	maxBytes := bounds.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTraceMaxBytes
	}

	currentHash, hashErr := hashFileContents(locator.Path)
	if hashErr != nil {
		if errors.Is(hashErr, sql.ErrNoRows) {
			// Defensive: hashFileContents never returns ErrNoRows; kept for
			// clarity when the helper evolves.
		}
		return TraceResult{
			Code:    string(domain.ErrExperienceSourceNotFound),
			Message: fmt.Sprintf("opencode trace: cannot hash source: %v", hashErr),
		}
	}
	if locator.SourceHash != "" && currentHash != locator.SourceHash {
		return TraceResult{
			SourceChanged: true,
			Code:          "trace_source_changed",
			Message:       "opencode trace: source database has changed since the locator was issued",
		}
	}

	db, openErr := sql.Open("sqlite", "file:"+locator.Path+"?mode=ro")
	if openErr != nil {
		return TraceResult{
			Code:    string(domain.ErrExperienceSourceNotFound),
			Message: fmt.Sprintf("opencode trace: cannot open source read-only: %v", openErr),
		}
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return TraceResult{
				Code:    string(domain.ErrTimeout),
				Message: err.Error(),
			}
		}
		return TraceResult{
			Code:    string(domain.ErrExperienceSourceNotFound),
			Message: fmt.Sprintf("opencode trace: cannot ping source: %v", err),
		}
	}

	if !verifyOpenCodeSchema(ctx, db) {
		return TraceResult{
			Code:    string(domain.ErrExperienceSchemaUnsupported),
			Message: "opencode trace: source database does not match the expected OpenCode schema",
		}
	}

	var content sql.NullString
	queryErr := db.QueryRowContext(ctx, "SELECT content FROM messages WHERE id = ?", locator.TurnID).Scan(&content)
	if queryErr != nil {
		if errors.Is(queryErr, sql.ErrNoRows) {
			return TraceResult{
				Code:    "trace_source_unavailable",
				Message: fmt.Sprintf("opencode trace: turn %s not found in source", locator.TurnID),
			}
		}
		if errors.Is(queryErr, context.Canceled) || errors.Is(queryErr, context.DeadlineExceeded) {
			return TraceResult{
				Code:    string(domain.ErrTimeout),
				Message: queryErr.Error(),
			}
		}
		return TraceResult{
			Code:    "trace_source_unavailable",
			Message: fmt.Sprintf("opencode trace: cannot query source: %v", queryErr),
		}
	}

	excerpt, redacted := redactExcerpt(content.String, maxBytes)
	return TraceResult{
		Excerpt:  excerpt,
		Redacted: redacted,
	}
}

// redactExcerpt runs the content through evidence.Redact and then trims
// the result to maxBytes. The truncation appends TraceExcerptSuffix when
// content was trimmed; the suffix itself counts against the cap.
func redactExcerpt(content string, maxBytes int) (string, bool) {
	redacted := evidence.Redact([]byte(content), nil)
	changed := string(redacted) != content
	out := string(redacted)
	if len(out) <= maxBytes {
		return out, changed
	}
	limit := maxBytes - len(TraceExcerptSuffix)
	if limit < 0 {
		limit = 0
	}
	if limit > len(out) {
		limit = len(out)
	}
	out = out[:limit] + TraceExcerptSuffix
	return out, changed
}
