package opencode

import (
	"context"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/jobs"
	"agent-royo-learn/internal/experience/semantic"
)

// JobName is the canonical job identifier for the OpenCode ingest loop.
const JobName = "experience_ingest:opencode"

// Job returns a fresh runtime binding for the OpenCode ingest job.
func (a *Adapter) Job() *semantic.Job {
	entry := newIngestJobRegistryEntry(a.now())
	return &semantic.Job{
		Name:               entry.JobName,
		Source:             string(domain.SourceOpenCode),
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
			return semanticResult(result.Envelopes, result.SkippedIncomplete, 0, result.NextCursor), err
		},
	}
}

func newIngestJobRegistryEntry(createdAt time.Time) jobs.JobRegistryEntry {
	return jobs.JobRegistryEntry{
		JobName:            JobName,
		Description:        "Incremental ingest of OpenCode SQLite transcripts.",
		DefaultIntervalSec: 300,
		DefaultMaxRetries:  3,
		Enabled:            false,
		CreatedAt:          createdAt.UTC(),
		Intent:             semantic.JobIntentIngest,
		Scope:              semantic.JobScopeProject,
		RiskClass:          semantic.JobRiskClassLow,
	}
}

func semanticResult(envelopes []experience.ExperienceEnvelope, skippedIncomplete, skippedMalformed int, cursor map[string]any) semantic.Result {
	out := semantic.Result{SkippedIncomplete: skippedIncomplete, SkippedMalformed: skippedMalformed}
	out.Envelopes = make([]any, len(envelopes))
	for i := range envelopes {
		out.Envelopes[i] = envelopes[i]
	}
	if value, ok := cursor["last_message_id"].(string); ok {
		out.NextCursor = value
	}
	return out
}
