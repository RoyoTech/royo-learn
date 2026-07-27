package claudecode

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

// healthHeaderBytes bounds the read window when confirming the upstream
// JSONL header (docs/22 §3). A multi-gigabyte transcript cannot stall the
// ingestor because the probe never reads beyond this.
const healthHeaderBytes = 1 << 10

// Health performs a read-only check on the candidate JSONL. Status mapping
// mirrors opencode/health.go per docs/22 §6: "ok" / "degraded" / "error".
// The source file is never written to (the no-side-effects test would fail).
func (a *Adapter) Health(ctx context.Context, instance SourceInstance) HealthResult {
	now := a.now()
	if err := ctx.Err(); err != nil {
		return HealthResult{Status: "error", JSONLPath: instance.JSONLPath, Code: string(domain.ErrTimeout), Message: err.Error(), CheckedAt: now}
	}
	if instance.Source != domain.SourceClaudeCode {
		return HealthResult{Status: "error", JSONLPath: instance.JSONLPath, Code: string(domain.ErrInvalidArgument), Message: "claudecode health: instance source is not claude_code", CheckedAt: now}
	}
	if strings.TrimSpace(instance.JSONLPath) == "" {
		return HealthResult{Status: "error", JSONLPath: instance.JSONLPath, Code: string(domain.ErrInvalidArgument), Message: "claudecode health: instance JSONLPath is required", CheckedAt: now}
	}
	info, err := os.Stat(instance.JSONLPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return HealthResult{Status: "degraded", JSONLPath: instance.JSONLPath, Code: string(domain.ErrExperienceSourceNotFound), Message: fmt.Sprintf("claudecode health: cannot stat source: %v", err), CheckedAt: now}
	}
	if err != nil {
		return HealthResult{Status: "degraded", JSONLPath: instance.JSONLPath, Code: string(domain.ErrExperienceSourceNotFound), Message: "claudecode health: source JSONL file does not exist", CheckedAt: now}
	}
	if info.IsDir() {
		return HealthResult{Status: "degraded", JSONLPath: instance.JSONLPath, Code: string(domain.ErrExperienceSourceNotFound), Message: "claudecode health: source path is a directory, not a JSONL file", CheckedAt: now}
	}
	// Re-check cancellation between stat and open so a racing shutdown
	// signal still gets ErrTimeout instead of a degraded result.
	if err := ctx.Err(); err != nil {
		return HealthResult{Status: "error", JSONLPath: instance.JSONLPath, Code: string(domain.ErrTimeout), Message: err.Error(), CheckedAt: now}
	}
	if err := verifyClaudeCodeHeader(instance.JSONLPath); err != nil {
		return HealthResult{Status: "degraded", JSONLPath: instance.JSONLPath, Readable: true, SchemaOK: false, Code: string(domain.ErrExperienceSchemaUnsupported), Message: fmt.Sprintf("claudecode health: source JSONL does not match the expected Claude Code schema: %v", err), CheckedAt: now}
	}
	return HealthResult{Status: "ok", JSONLPath: instance.JSONLPath, Readable: true, SchemaOK: true, CheckedAt: now}
}

// verifyClaudeCodeHeader returns nil iff at least one JSON object in the
// first healthHeaderBytes carries non-empty type, uuid, and sessionId.
func verifyClaudeCodeHeader(jsonlPath string) error {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	dec := json.NewDecoder(bufio.NewReaderSize(io.LimitReader(f, int64(healthHeaderBytes)), healthHeaderBytes))
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("no complete JSON object in first %d bytes", healthHeaderBytes)
			}
			return fmt.Errorf("decode first object: %w", err)
		}
		t, _ := obj["type"].(string)
		u, _ := obj["uuid"].(string)
		s, _ := obj["sessionId"].(string)
		if strings.TrimSpace(t) != "" && strings.TrimSpace(u) != "" && strings.TrimSpace(s) != "" {
			return nil
		}
	}
}
