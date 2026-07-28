package codex

import (
	"context"
	"time"

	"agent-royo-learn/internal/domain"
)

// ExperienceAdapter is the platform adapter contract frozen by docs/22.
type ExperienceAdapter interface {
	Name() domain.ExperienceSource
	Discover(context.Context, string) ([]SourceInstance, error)
	Health(context.Context, SourceInstance) HealthResult
	Scan(context.Context, ScanRequest) (ScanResult, error)
	ResolveTrace(context.Context, domain.TranscriptLocator, TraceBounds) TraceResult
}

// Adapter implements Codex rollout ingestion without mutable per-call state.
type Adapter struct {
	Now func() time.Time
}

// NewAdapter returns an adapter with production defaults.
func NewAdapter() *Adapter { return &Adapter{} }

// Name returns the canonical Codex source discriminator.
func (a *Adapter) Name() domain.ExperienceSource { return domain.SourceCodex }

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

// Scan is replaced by slice 10.3.
func (a *Adapter) Scan(ctx context.Context, _ ScanRequest) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Status: "ok", ScannedAt: a.now()}, nil
}

// ResolveTrace is replaced by slice 10.5.
func (a *Adapter) ResolveTrace(ctx context.Context, _ domain.TranscriptLocator, _ TraceBounds) TraceResult {
	if err := ctx.Err(); err != nil {
		return TraceResult{Code: string(domain.ErrTimeout), Message: err.Error()}
	}
	return TraceResult{Code: string(domain.ErrExperienceLocatorInvalid)}
}

var _ ExperienceAdapter = (*Adapter)(nil)
