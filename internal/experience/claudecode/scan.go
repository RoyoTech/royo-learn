package claudecode

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

type jsonlTurn struct {
	Type, UUID, SessionID string
	Timestamp             json.RawMessage
	StopReason            string `json:"stop_reason"`
	Message               struct{ Content json.RawMessage }
}

func (a *Adapter) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	data, err := os.ReadFile(req.Instance.JSONLPath)
	if err != nil {
		return ScanResult{}, err
	}
	h := sha256.Sum256(data)
	sourceHash := hex.EncodeToString(h[:])
	result := ScanResult{Instance: req.Instance, Status: "ok", ScannedAt: a.now()}
	var turns []jsonlTurn
	s := bufio.NewScanner(strings.NewReader(string(data)))
	s.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for s.Scan() {
		var turn jsonlTurn
		if json.Unmarshal(s.Bytes(), &turn) != nil || turn.UUID == "" || turn.SessionID == "" || parseTimestamp(turn.Timestamp).IsZero() {
			result.SkippedMalformed++
			continue
		}
		turns = append(turns, turn)
	}
	if err := s.Err(); err != nil {
		return ScanResult{}, err
	}
	for i, turn := range turns {
		if err := ctx.Err(); err != nil {
			return ScanResult{}, err
		}
		if turn.Type == "system" {
			result.SkippedSystem++
			continue
		}
		if turn.Type != "user" && turn.Type != "assistant" {
			result.SkippedUnknown++
			continue
		}
		complete := turn.StopReason != ""
		for j := i + 1; !complete && j < len(turns); j++ {
			complete = turns[j].SessionID == turn.SessionID && turns[j].Type == "user"
		}
		if !complete {
			result.SkippedIncomplete++
			continue
		}
		e := makeEnvelope(req, turn, sourceHash)
		result.Envelopes = append(result.Envelopes, e)
	}
	sort.Slice(result.Envelopes, func(i, j int) bool {
		a, b := result.Envelopes[i], result.Envelopes[j]
		return a.Session.ExternalID < b.Session.ExternalID || a.Session.ExternalID == b.Session.ExternalID && a.Turn.ExternalID < b.Turn.ExternalID
	})
	if len(result.Envelopes) > 0 {
		last := result.Envelopes[len(result.Envelopes)-1]
		result.NextCursor = map[string]any{"last_session_id": last.Session.ExternalID, "last_turn_uuid": last.Turn.ExternalID}
	}
	return result, nil
}

func makeEnvelope(req ScanRequest, turn jsonlTurn, hash string) experience.ExperienceEnvelope {
	var e experience.ExperienceEnvelope
	at := parseTimestamp(turn.Timestamp)
	e.SchemaVersion = experience.ExperienceEnvelopeSchemaVersion
	e.Source = domain.SourceClaudeCode
	e.ProjectRoot = req.ProjectRoot
	e.Session.ExternalID, e.Session.UpdatedAt = turn.SessionID, at
	e.Session.Locator = domain.TranscriptLocator{Kind: "jsonl", Path: req.Instance.JSONLPath, SessionID: turn.SessionID, TurnID: turn.UUID, SourceHash: hash}
	e.Turn.ExternalID, e.Turn.Complete, e.Turn.FinishReason, e.Turn.OccurredAt = turn.UUID, true, turn.StopReason, at
	e.Actor = domain.Actor{Kind: "agent", Name: "claude_code"}
	var text string
	if json.Unmarshal(turn.Message.Content, &text) == nil {
		if turn.Type == "user" {
			e.Turn.UserText = text
		} else {
			e.Turn.AssistantText = text
		}
		return e
	}
	var blocks []struct {
		Type, Text, Name, ID string
		Input                map[string]any
	}
	_ = json.Unmarshal(turn.Message.Content, &blocks)
	for _, b := range blocks {
		if b.Type == "text" {
			if turn.Type == "user" {
				e.Turn.UserText += b.Text
			} else {
				e.Turn.AssistantText += b.Text
			}
		}
		if b.Type == "tool_use" {
			e.Turn.ToolCalls = append(e.Turn.ToolCalls, experience.SafeToolCall{Name: b.Name, Arguments: b.Input, Outcome: b.ID})
		}
	}
	return e
}

func parseTimestamp(raw json.RawMessage) time.Time {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}
	var ms int64
	if json.Unmarshal(raw, &ms) == nil {
		return time.UnixMilli(ms).UTC()
	}
	return time.Time{}
}
