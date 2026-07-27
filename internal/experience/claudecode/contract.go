package claudecode

import (
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

// SchemaTag is the stable schema identifier for this adapter. Bumping it is a
// breaking change for any consumer that gates on schema version.
const SchemaTag = "claude-code/jsonl-v1"

// SourceInstance is one located Claude Code session store that an adapter can
// scan. Discover returns zero or more of these per call.
type SourceInstance struct {
	// Source is always domain.SourceClaudeCode for this adapter.
	Source domain.ExperienceSource
	// ProjectRoot is the canonical, symlink-resolved absolute path of the
	// stored project root the caller has authorized.
	ProjectRoot string
	// JSONLPath is the canonical absolute path of the Claude Code JSONL
	// session file. The adapter only opens it in read-only mode.
	JSONLPath string
	// Schema identifies the upstream schema version observed during discovery.
	Schema string
	// Discovered records when Discover located this instance.
	Discovered time.Time
}

// ScanRequest carries the inputs required to scan one SourceInstance for new
// experience. Since and Cursor are optional; when both are nil the scan reads
// the entire store.
type ScanRequest struct {
	ProjectRoot string
	Instance    SourceInstance
	Since       *time.Time
	Cursor      map[string]any
}

// ScanResult is the outcome of one Scan call. Envelopes are redacted only for
// transport fields (paths, IDs); full redaction and fingerprinting happen in
// the core service after this method returns.
type ScanResult struct {
	Instance  SourceInstance
	Envelopes []experience.ExperienceEnvelope
	// NextCursor is the opaque checkpoint the caller should persist and pass
	// back on the next scan. Nil when the scan exhausted the source.
	NextCursor map[string]any
	// Status is one of: "ok", "degraded", "error".
	Status string
	// Code is a stable error code when Status is not "ok".
	Code string
	// Message is a human-readable summary, always redacted and bounded.
	Message string
	// Degraded reports whether the source was partially readable.
	Degraded bool
	// SkippedIncomplete counts turns the adapter dropped because no
	// stop_reason and no subsequent user turn closed the turn.
	SkippedIncomplete int
	// SkippedMalformed counts JSONL lines that failed to parse.
	SkippedMalformed int
	// SkippedSystem counts lines whose type is "system".
	SkippedSystem int
	// SkippedUnknown counts lines whose type is not user/assistant/system.
	SkippedUnknown int
	// ScannedAt records the wall-clock time the scan finished.
	ScannedAt time.Time
}

// TraceBounds parameterizes a trace excerpt request. MaxBytes is a hard cap on
// the returned excerpt; Offset and Length are advisory hints and may be
// ignored when the locator does not support byte addressing.
type TraceBounds struct {
	MaxBytes int
	Offset   int64
	Length   int64
}

// TraceResult is the outcome of one ResolveTrace call. The Excerpt is always
// redacted and bounded by TraceBounds.MaxBytes. Full transcript content is
// never returned without an explicit excerpt flag, and the adapter itself
// does not currently emit full transcripts (see "El adaptador NO puede",
// docs/22-ADAPTER-CONTRACT.md §2).
type TraceResult struct {
	// Excerpt is the bounded, redacted slice of transcript text.
	Excerpt string
	// Redacted reports whether any secret pattern was replaced.
	Redacted bool
	// SourceChanged reports whether the source file has been mutated since
	// the referenced turn was ingested. When true, the caller should treat
	// the excerpt as advisory only.
	SourceChanged bool
	// Code is a stable trace result code. Empty when the excerpt is fresh and
	// authorized.
	Code string
	// Message is a human-readable summary, always redacted and bounded.
	Message string
}

// HealthResult is the outcome of one Health call. Status is "ok" only when
// Readable and SchemaOK are both true. The adapter never writes to the source
// file during Health.
type HealthResult struct {
	Status    string // "ok" | "degraded" | "error"
	JSONLPath string
	Readable  bool
	SchemaOK  bool
	Message   string
	Code      string
	CheckedAt time.Time
}
