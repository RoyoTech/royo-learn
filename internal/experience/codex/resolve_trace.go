package codex

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
// decides whether to surface the marker.
const TraceExcerptSuffix = "..."

// jsonlTraceLine is the minimal JSONL shape ResolveTrace needs to match a
// Codex turn. Extra fields the upstream schema may grow are accepted and
// ignored.
type jsonlTraceLine struct {
	Type    string `json:"type"`
	TurnID  string `json:"turn_id"`
	Payload struct {
		Type    string `json:"type"`
		TurnID  string `json:"turn_id"`
		Message string `json:"message"`
		Text    string `json:"text"`
	} `json:"payload"`
}

// ResolveTrace returns a bounded, redacted excerpt for the locator and
// bounds requested. Codex sessions live in JSONL, not SQLite, so the search
// is a streaming line scan filtered by turn_id.
//
// Code values produced:
//
//   - ""                            — success, excerpt is fresh and authorized.
//   - "trace_source_changed"        — SourceHash on the locator no longer
//     matches the source rollout; the caller should treat the locator as
//     stale. Per Codex design, NO excerpt is returned in this branch.
//   - "trace_source_unavailable"    — the source file is unreadable or the
//     referenced turn does not exist.
//   - "experience_locator_invalid"  — locator fields fail validation
//     before any source I/O.
//   - "experience_source_not_found" — the source file disappeared.
//   - string(domain.ErrTimeout)      — context cancellation or deadline.
func (a *Adapter) ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult {
	if err := ctx.Err(); err != nil {
		return TraceResult{Code: string(domain.ErrTimeout), Message: err.Error()}
	}
	if locator.Kind != "rollout" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "codex trace: locator kind must be rollout",
		}
	}
	if strings.TrimSpace(locator.Path) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "codex trace: locator path is required",
		}
	}
	if strings.TrimSpace(locator.TurnID) == "" {
		return TraceResult{
			Code:    string(domain.ErrExperienceLocatorInvalid),
			Message: "codex trace: locator turn id is required",
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
			Message: fmt.Sprintf("codex trace: cannot read source: %v", readErr),
		}
	}
	if locator.SourceHash != "" {
		sum := sha256.Sum256(data)
		current := hex.EncodeToString(sum[:])
		if current != locator.SourceHash {
			return TraceResult{
				SourceChanged: true,
				Code:          "trace_source_changed",
				Message:       "codex trace: source file has changed since the locator was issued",
			}
		}
	}
	content, ok := findCodexTurnContent(data, locator.TurnID)
	if !ok {
		return TraceResult{
			Code:    "trace_source_unavailable",
			Message: fmt.Sprintf("codex trace: turn %s not found in source", locator.TurnID),
		}
	}
	excerpt, redacted := redactCodexExcerpt(content, maxBytes)
	return TraceResult{Excerpt: excerpt, Redacted: redacted}
}

func findCodexTurnContent(data []byte, turnID string) (string, bool) {
	s := bufio.NewScanner(strings.NewReader(string(data)))
	s.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for s.Scan() {
		var line jsonlTraceLine
		if err := json.Unmarshal(s.Bytes(), &line); err != nil {
			continue
		}
		id := firstNonEmpty(line.TurnID, line.Payload.TurnID)
		if id != turnID {
			continue
		}
		switch line.Type {
		case "event_msg":
			if line.Payload.Type == "event_msg.user_message" && line.Payload.Message != "" {
				return line.Payload.Message, true
			}
		case "response_item":
			if line.Payload.Text != "" {
				return line.Payload.Text, true
			}
		}
	}
	return "", false
}

func redactCodexExcerpt(content string, maxBytes int) (string, bool) {
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
