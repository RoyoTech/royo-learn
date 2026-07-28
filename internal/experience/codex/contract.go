package codex

import (
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

// SchemaTag is the stable upstream schema identifier for Codex rollouts.
const SchemaTag = "codex/rollout-v1"

// SourceInstance identifies one Codex rollout file reachable from a project.
type SourceInstance struct {
	Source      domain.ExperienceSource
	ProjectRoot string
	RolloutPath string
	Schema      string
	Discovered  time.Time
}

// ScanRequest carries the inputs for one incremental rollout scan.
type ScanRequest struct {
	ProjectRoot string
	Instance    SourceInstance
	Since       *time.Time
	Cursor      map[string]any
}

// ScanResult describes one rollout scan and its explicit skip counters.
type ScanResult struct {
	Instance          SourceInstance
	Envelopes         []experience.ExperienceEnvelope
	NextCursor        map[string]any
	Status            string
	Code              string
	Message           string
	Degraded          bool
	SkippedIncomplete int
	SkippedMalformed  int
	ScannedAt         time.Time
}

// TraceBounds caps a trace excerpt. Offset and Length are advisory.
type TraceBounds struct {
	MaxBytes int
	Offset   int64
	Length   int64
}

// TraceResult is a bounded, redacted trace lookup result.
type TraceResult struct {
	Excerpt       string
	Redacted      bool
	SourceChanged bool
	Code          string
	Message       string
}

// HealthResult reports whether a rollout is readable and schema-compatible.
type HealthResult struct {
	Status      string
	RolloutPath string
	Readable    bool
	SchemaOK    bool
	Message     string
	Code        string
	CheckedAt   time.Time
}
