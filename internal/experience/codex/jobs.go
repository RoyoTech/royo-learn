package codex

import (
	"time"

	"agent-royo-learn/internal/experience/jobs"
)

// JobName is the canonical job identifier for the Codex ingest loop.
const JobName = "experience_ingest:codex"

// JobRegistryEntry returns the static registration row for the Codex
// ingest job. Mirrors the opencode and Claude Code precedents (5-minute
// interval, 3 retries) and ships disabled so Ola 2 (--watch) is the
// single switch that flips it on. Re-registering the same name is a
// no-op thanks to the engine's ON CONFLICT(job_name) DO UPDATE upsert
// (docs/22-ADAPTER-CONTRACT.md §6, docs/25 §2 Hito 10 jobs row).
func JobRegistryEntry() jobs.JobRegistryEntry {
	return jobs.JobRegistryEntry{
		JobName:            JobName,
		Description:        "Incremental ingest of Codex rollout JSONL transcripts.",
		DefaultIntervalSec: 300,
		DefaultMaxRetries:  3,
		Enabled:            false,
		CreatedAt:          time.Now().UTC(),
	}
}
