package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"agent-royo-learn/internal/domain"
)

const healthHeaderBytes = 1 << 10

// Health probes one rollout read-only and validates its session_meta anchor.
func (a *Adapter) Health(ctx context.Context, instance SourceInstance) HealthResult {
	now := a.now()
	result := HealthResult{RolloutPath: instance.RolloutPath, CheckedAt: now}
	if err := ctx.Err(); err != nil {
		result.Status, result.Code, result.Message = "error", string(domain.ErrTimeout), err.Error()
		return result
	}
	if instance.Source != domain.SourceCodex {
		result.Status, result.Code, result.Message = "error", string(domain.ErrInvalidArgument), "codex health: instance source is not codex"
		return result
	}
	if strings.TrimSpace(instance.RolloutPath) == "" {
		result.Status, result.Code, result.Message = "error", string(domain.ErrInvalidArgument), "codex health: rollout path is required"
		return result
	}
	info, err := os.Stat(instance.RolloutPath)
	if err != nil || info.IsDir() {
		result.Status, result.Code = "degraded", string(domain.ErrExperienceSourceNotFound)
		result.Message = "codex health: rollout source is unavailable"
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Status, result.Code, result.Message = "error", string(domain.ErrTimeout), err.Error()
		return result
	}
	if err := verifyCodexHeader(instance.RolloutPath); err != nil {
		result.Status, result.Readable, result.Code = "degraded", true, string(domain.ErrExperienceSchemaUnsupported)
		result.Message = fmt.Sprintf("codex health: unsupported rollout schema: %v", err)
		return result
	}
	result.Status, result.Readable, result.SchemaOK = "ok", true, true
	return result
}

func verifyCodexHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(bufio.NewReader(io.LimitReader(file, healthHeaderBytes)))
	for {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				CodexSessionID string `json:"codex_session_id"`
				SessionID      string `json:"session_id"`
				ID             string `json:"id"`
				CWD            string `json:"cwd"`
				CLIVersion     string `json:"cli_version"`
			} `json:"payload"`
		}
		if err := decoder.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("session_meta not found in first %d bytes", healthHeaderBytes)
			}
			return err
		}
		if line.Type != "session_meta" {
			continue
		}
		sessionID := firstNonEmpty(line.Payload.CodexSessionID, line.Payload.SessionID, line.Payload.ID)
		if sessionID == "" || line.Payload.CWD == "" || line.Payload.CLIVersion == "" {
			return fmt.Errorf("session_meta is missing required fields")
		}
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
