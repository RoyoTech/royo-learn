package claudecode

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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
// decides whether to surface the marker. It mirrors the opencode adapter
// so callers across the two adapters see the same shape.
const TraceExcerptSuffix = "..."

// jsonlTraceLine is the minimal JSONL shape ResolveTrace needs to match a
// turn. It is intentionally narrow: any extra fields the upstream schema
// may grow (parentUuid, version, cwd, …) are accepted and ignored via the
// embedded RawMessage on message.content.
type jsonlTraceLine struct {
	UUID    string `json:"uuid"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// ResolveTrace returns a bounded, redacted excerpt for the locator and
// bounds requested. Claude Code sessions live in JSONL, not SQLite, so
// the search is a streaming line scan filtered by uuid. The adapter never
// returns full transcript content (docs/22-ADAPTER-CONTRACT.md §2 —
// "El adaptador NO puede"); the only output is an excerpt capped at
// bounds.MaxBytes and run through evidence.Redact.
//
// Code values produced:
//
//   - ""                            — success, excerpt is fresh and authorized.
//   - "trace_source_changed"        — SourceHash on the locator no longer
//     matches the source JSONL; the caller should treat the excerpt as
//     advisory only (still returned).
//   - "trace_source_unavailable"    — the source JSONL is unreadable or
//     the referenced turn does not exist.
//   - "experience_locator_invalid"  — locator fields fail validation
//     before any source I/O.
//   - "experience_source_not_found" — the source file disappeared.
//   - string(domain.ErrTimeout)      — context cancellation or deadline.
func (a *Adapter) ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult {
	if err := ctx.Err(); err != nil {
		return TraceResult{
			Code:    string(domain.ErrTimeout),
			Message: err.Error(),
		}
	}
	if locator.Kind != "jsonl" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "claudecode trace: locator kind must be jsonl",
		}
	}
	if strings.TrimSpace(locator.Path) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "claudecode trace: locator path is required",
		}
	}
	if strings.TrimSpace(locator.TurnID) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "claudecode trace: locator turn id is required",
		}
	}

	maxBytes := bounds.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTraceMaxBytes
	}

	data, readErr := os.ReadFile(locator.Path)
	if readErr != nil {
		return TraceResult{
			Code:    string(domain.ErrExperienceSourceNotFound),
			Message: fmt.Sprintf("claudecode trace: cannot read source: %v", readErr),
		}
	}

	currentHash := sha256.Sum256(data)
	currentHashHex := hex.EncodeToString(currentHash[:])
	if locator.SourceHash != "" && currentHashHex != locator.SourceHash {
		// Source is advisory: still return the bounded excerpt while flagging
		// the divergence so the caller can downgrade the result.
		content, ok := findTurnContent(data, locator.TurnID)
		if !ok {
			return TraceResult{
				SourceChanged: true,
				Code:          "trace_source_changed",
				Message:       "claudecode trace: source file has changed since the locator was issued",
			}
		}
		excerpt, redacted := redactExcerpt(content, maxBytes)
		return TraceResult{
			Excerpt:       excerpt,
			Redacted:      redacted,
			SourceChanged: true,
			Code:          "trace_source_changed",
			Message:       "claudecode trace: source file has changed since the locator was issued",
		}
	}

	content, ok := findTurnContent(data, locator.TurnID)
	if !ok {
		return TraceResult{
			Code:    "trace_source_unavailable",
			Message: fmt.Sprintf("claudecode trace: turn %s not found in source", locator.TurnID),
		}
	}

	excerpt, redacted := redactExcerpt(content, maxBytes)
	return TraceResult{
		Excerpt:  excerpt,
		Redacted: redacted,
	}
}

// findTurnContent streams the JSONL bytes looking for the line whose uuid
// matches turnID. It returns the textual content extracted from the
// matching turn: a single string field when message.content is a string,
// or the concatenation of text blocks when it is a list. Thinking blocks
// are dropped per AGENTS.md regla 9.
func findTurnContent(data []byte, turnID string) (string, bool) {
	s := bufio.NewScanner(strings.NewReader(string(data)))
	s.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for s.Scan() {
		raw := s.Bytes()
		if len(raw) == 0 {
			continue
		}
		var line jsonlTraceLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}
		if line.UUID != turnID {
			continue
		}
		return extractLineContent(line), true
	}
	return "", false
}

// extractLineContent reduces a turn's message.content to its readable
// text. A string content becomes that string; a list of blocks becomes the
// concatenation of the text blocks (thinking blocks omitted).
func extractLineContent(line jsonlTraceLine) string {
	if len(line.Message.Content) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(line.Message.Content, &asString); err == nil {
		return asString
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(line.Message.Content, &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == "text" {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// redactExcerpt runs the content through evidence.Redact and then trims
// the result to maxBytes. Truncation appends TraceExcerptSuffix when the
// trimmed content still exceeds the cap; the suffix itself counts.
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
