package codex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

const maxJSONLLineBytes = 1 << 20

type codexEvent struct {
	Timestamp json.RawMessage `json:"timestamp"`
	Type      string          `json:"type"`
	TurnID    string          `json:"turn_id"`
	Payload   struct {
		CodexSessionID string          `json:"codex_session_id"`
		SessionID      string          `json:"session_id"`
		ID             string          `json:"id"`
		TurnID         string          `json:"turn_id"`
		Type           string          `json:"type"`
		Message        string          `json:"message"`
		Role           string          `json:"role"`
		CallID         string          `json:"call_id"`
		Name           string          `json:"name"`
		Arguments      json.RawMessage `json:"arguments"`
		Output         string          `json:"output"`
		Text           string          `json:"text"`
	} `json:"payload"`
}

// Scan reads one rollout JSONL and emits one neutral envelope per complete
// turn anchored by event_msg.task_started / task_complete or turn_context.
// Reasoning events are dropped before the envelope is built, and
// function_call_output never reaches envelope fields verbatim.
func (a *Adapter) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	if req.Instance.Source != domain.SourceCodex {
		return ScanResult{}, domain.NewValidationError(domain.ErrInvalidArgument, "codex scan: instance source is not codex")
	}
	data, err := os.ReadFile(req.Instance.RolloutPath)
	if err != nil {
		return ScanResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	sum := sha256.Sum256(data)
	sourceHash := hex.EncodeToString(sum[:])
	result := ScanResult{Instance: req.Instance, Status: "ok", ScannedAt: a.now()}

	cursorSession, cursorTurn, hasCursor := cursorCheckpoint(req.Cursor)

	var lines []codexEvent
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, err
		}
		var ev codexEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			result.SkippedMalformed++
			continue
		}
		lines = append(lines, ev)
	}
	if err := scanner.Err(); err != nil {
		return ScanResult{}, err
	}

	var envelopes []experience.ExperienceEnvelope
	sessionID := ""
	turns := map[string]*codexTurn{}
	var turnOrder []string
	for _, ev := range lines {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, err
		}
		switch ev.Type {
		case "session_meta":
			if sessionID == "" {
				sessionID = firstNonEmpty(ev.Payload.CodexSessionID, ev.Payload.SessionID, ev.Payload.ID)
			}
		case "turn_context":
			id := firstNonEmpty(ev.TurnID, ev.Payload.TurnID)
			if id == "" {
				continue
			}
			turns[id] = newCodexTurn(id)
			if _, ok := indexOfTurn(turnOrder, id); !ok {
				turnOrder = append(turnOrder, id)
			}
		case "event_msg":
			id := firstNonEmpty(ev.TurnID, ev.Payload.TurnID)
			switch strings.TrimPrefix(ev.Payload.Type, "event_msg.") {
			case "task_started":
				if _, ok := turns[id]; !ok {
					turns[id] = newCodexTurn(id)
					if _, exists := indexOfTurn(turnOrder, id); !exists {
						turnOrder = append(turnOrder, id)
					}
				}
				turns[id].StartedAt = mergeTime(turns[id].StartedAt, parseTimestamp(ev.Timestamp))
				turns[id].StarterSeen = true
			case "user_message":
				anchor := firstNonEmpty(id, lastTurnID(turnOrder, turns))
				if anchor == "" {
					continue
				}
				if _, ok := turns[anchor]; !ok {
					turns[anchor] = newCodexTurn(anchor)
					turnOrder = append(turnOrder, anchor)
				}
				turns[anchor].UserText = mergeText(turns[anchor].UserText, ev.Payload.Message)
				turns[anchor].StarterSeen = true
			case "task_complete":
				anchor := firstNonEmpty(id, lastTurnID(turnOrder, turns))
				if anchor == "" {
					continue
				}
				if _, ok := turns[anchor]; !ok {
					continue
				}
				turns[anchor].AssistantText = mergeText(turns[anchor].AssistantText, ev.Payload.Message)
				turns[anchor].FinishReason = strings.TrimPrefix(ev.Payload.Type, "event_msg.")
				turns[anchor].Complete = true
				turns[anchor].OccurredAt = parseTimestamp(ev.Timestamp)
			}
		case "response_item":
			id := firstNonEmpty(ev.TurnID, ev.Payload.TurnID)
			if id == "" {
				id = lastTurnID(turnOrder, turns)
			}
			if id == "" {
				continue
			}
			if _, ok := turns[id]; !ok {
				turns[id] = newCodexTurn(id)
				turnOrder = append(turnOrder, id)
			}
			turn := turns[id]
			switch ev.Payload.Type {
			case "function_call":
				call := experience.SafeToolCall{Name: ev.Payload.Name, Outcome: ev.Payload.CallID}
				if len(ev.Payload.Arguments) > 0 {
					var args map[string]any
					if err := json.Unmarshal(ev.Payload.Arguments, &args); err == nil {
						call.Arguments = args
					} else {
						call.Arguments = map[string]any{"raw": string(ev.Payload.Arguments)}
					}
				}
				turn.ToolCalls = append(turn.ToolCalls, call)
				turn.FunctionOutputs[ev.Payload.CallID] = ""
			case "function_call_output":
				turn.FunctionOutputs[ev.Payload.CallID] = ev.Payload.Output
			}
		}
	}
	if sessionID == "" {
		for _, ev := range lines {
			if ev.Type == "session_meta" {
				sessionID = firstNonEmpty(ev.Payload.CodexSessionID, ev.Payload.SessionID, ev.Payload.ID)
				if sessionID != "" {
					break
				}
			}
		}
	}
	for _, id := range turnOrder {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, err
		}
		turn := turns[id]
		if !turn.StarterSeen || !turn.Complete {
			result.SkippedIncomplete++
			continue
		}
		if sessionID == "" {
			result.SkippedIncomplete++
			continue
		}
		if hasCursor && cursorAtOrBefore(sessionID, id, cursorSession, cursorTurn) {
			continue
		}
		envelope := makeCodexEnvelope(req, sessionID, id, turn, sourceHash)
		envelopes = append(envelopes, envelope)
	}
	sort.Slice(envelopes, func(i, j int) bool {
		a, b := envelopes[i], envelopes[j]
		if a.Session.ExternalID != b.Session.ExternalID {
			return a.Session.ExternalID < b.Session.ExternalID
		}
		return a.Turn.ExternalID < b.Turn.ExternalID
	})
	result.Envelopes = envelopes
	if len(envelopes) > 0 {
		last := envelopes[len(envelopes)-1]
		result.NextCursor = map[string]any{
			"last_session_id": last.Session.ExternalID,
			"last_turn_uuid":  last.Turn.ExternalID,
		}
	}
	return result, nil
}

type codexTurn struct {
	ID              string
	UserText        string
	AssistantText   string
	ToolCalls       []experience.SafeToolCall
	FunctionOutputs map[string]string
	StartedAt       time.Time
	OccurredAt      time.Time
	FinishReason    string
	Complete        bool
	StarterSeen     bool
}

func newCodexTurn(id string) *codexTurn {
	return &codexTurn{ID: id, FunctionOutputs: map[string]string{}}
}

func mergeText(prev, next string) string {
	if prev == "" {
		return next
	}
	if next == "" {
		return prev
	}
	return prev + "\n" + next
}

func mergeTime(prev, next time.Time) time.Time {
	if next.IsZero() {
		return prev
	}
	if prev.IsZero() {
		return next
	}
	if next.Before(prev) {
		return next
	}
	return prev
}

func lastTurnID(order []string, turns map[string]*codexTurn) string {
	for i := len(order) - 1; i >= 0; i-- {
		if _, ok := turns[order[i]]; ok {
			return order[i]
		}
	}
	return ""
}

func indexOfTurn(order []string, id string) (int, bool) {
	for i, value := range order {
		if value == id {
			return i, true
		}
	}
	return 0, false
}

func makeCodexEnvelope(req ScanRequest, sessionID, turnID string, turn *codexTurn, sourceHash string) experience.ExperienceEnvelope {
	var envelope experience.ExperienceEnvelope
	envelope.SchemaVersion = experience.ExperienceEnvelopeSchemaVersion
	envelope.Source = domain.SourceCodex
	envelope.ProjectRoot = req.ProjectRoot
	envelope.Session.ExternalID = sessionID
	envelope.Session.Locator = domain.TranscriptLocator{
		Kind: "rollout", Path: req.Instance.RolloutPath,
		SessionID: sessionID, TurnID: turnID, SourceHash: sourceHash,
	}
	envelope.Session.UpdatedAt = turn.OccurredAt
	if !turn.StartedAt.IsZero() {
		started := turn.StartedAt
		envelope.Session.StartedAt = &started
	}
	envelope.Turn.ExternalID = turnID
	envelope.Turn.Complete = turn.Complete
	envelope.Turn.FinishReason = turn.FinishReason
	envelope.Turn.OccurredAt = turn.OccurredAt
	envelope.Turn.UserText = turn.UserText
	envelope.Turn.AssistantText = turn.AssistantText
	envelope.Turn.SourceRevision = sourceHash
	envelope.Actor = domain.Actor{Kind: "agent", Name: "codex"}

	if len(turn.ToolCalls) > 0 {
		prepared := make([]experience.SafeToolCall, 0, len(turn.ToolCalls))
		for _, call := range turn.ToolCalls {
			if output, ok := turn.FunctionOutputs[call.Outcome]; ok {
				call.OutputHash = digestCodexOutput(output)
				call.OutputHint = boundedOmissionHint(output)
			}
			prepared = append(prepared, call)
		}
		envelope.Turn.ToolCalls = prepared
	}
	return envelope
}

func digestCodexOutput(output string) string {
	if output == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(output))
	return hex.EncodeToString(sum[:])
}

func boundedOmissionHint(output string) string {
	const hintCap = 96
	if len(output) <= hintCap {
		return "[omitted]"
	}
	return "[omitted " + itoa(len(output)) + " bytes]"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func parseTimestamp(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
