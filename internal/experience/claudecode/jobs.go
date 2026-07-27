package claudecode

import "agent-royo-learn/internal/experience/jobs"

// JobName is the canonical job identifier for the Claude Code ingest
// loop. It is the single source of truth across the orchestrator
// (cmd/royo-learn/experience.go) and the Hito 3 watch loop, which
// reads from job_registry at startup.
const JobName = "experience_ingest:claude_code"

// JobRegistryEntry returns the static registration row for the Claude
// Code ingest job. Mirrors the opencode precedent (5-minute interval,
// 3 retries) and ships disabled so Hito 3 (--watch) is the single
// switch that flips it on. Re-registering the same name is a no-op
// thanks to the engine's ON CONFLICT(job_name) DO UPDATE upsert
// (docs/22-ADAPTER-CONTRACT.md §6, docs/25 §2 Hito 8 jobs row).
func JobRegistryEntry() jobs.JobRegistryEntry {
	return jobs.JobRegistryEntry{
		JobName:            JobName,
		Description:        "Incremental ingest of Claude Code JSONL transcripts.",
		DefaultIntervalSec: 300,
		DefaultMaxRetries:  3,
		Enabled:            false,
	}
}
