package codex

import (
	"context"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/jobs"
	"agent-royo-learn/internal/experience/semantic"
)

// JobName is the canonical job identifier for the Codex ingest loop.
const JobName = "experience_ingest:codex"

var jobNow = func() time.Time { return time.Now().UTC() }

// Job returns a fresh runtime binding for the Codex ingest job.
func (a *Adapter) Job() *semantic.Job {
	entry := newIngestJobRegistryEntry()
	return &semantic.Job{
		Name:               entry.JobName,
		Source:             string(domain.SourceCodex),
		Intent:             entry.Intent,
		Scope:              entry.Scope,
		RiskClass:          entry.RiskClass,
		Enabled:            entry.Enabled,
		DefaultIntervalSec: entry.DefaultIntervalSec,
		DefaultMaxRetries:  entry.DefaultMaxRetries,
		Func: func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
			if err := ctx.Err(); err != nil {
				return semantic.Result{ErrorCode: "context_cancelled", ErrorMessage: err.Error()}, err
			}
			instance, ok := deps.SourceInstance.(SourceInstance)
			if !ok {
				return semantic.Result{}, nil
			}
			result, err := a.Scan(ctx, ScanRequest{ProjectRoot: instance.ProjectRoot, Instance: instance})
			out := semantic.Result{SkippedIncomplete: result.SkippedIncomplete, SkippedMalformed: result.SkippedMalformed}
			out.Envelopes = make([]any, len(result.Envelopes))
			for i := range result.Envelopes {
				out.Envelopes[i] = result.Envelopes[i]
			}
			return out, err
		},
	}
}

func newIngestJobRegistryEntry() jobs.JobRegistryEntry {
	return jobs.JobRegistryEntry{
		JobName:            JobName,
		Description:        "Incremental ingest of Codex rollout JSONL transcripts.",
		DefaultIntervalSec: 300,
		DefaultMaxRetries:  3,
		Enabled:            false,
		CreatedAt:          jobNow(),
		Intent:             semantic.JobIntentIngest,
		Scope:              semantic.JobScopeProject,
		RiskClass:          semantic.JobRiskClassLow,
	}
}
