package opencode

import (
	"context"
	"time"

	"agent-royo-learn/internal/domain"
)

// ExperienceAdapter is the contract every platform-specific adapter
// implements. The four methods are fixed by docs/22-ADAPTER-CONTRACT.md §1;
// callers reach the adapter exclusively through this interface so the core
// service has no compile-time dependency on a specific source.
//
// All methods respect context cancellation, close every resource they open,
// and return typed errors. They never mutate the upstream source.
type ExperienceAdapter interface {
	// Name returns the canonical ExperienceSource this adapter serves.
	Name() domain.ExperienceSource

	// Discover locates one or more SourceInstances reachable from
	// projectRoot. It performs no I/O on the source DB; locating means
	// resolving candidate paths against the configured roots and reporting
	// which ones look like OpenCode session stores.
	Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error)

	// Health opens the source DB in read-only mode just long enough to
	// confirm the file is readable and the expected schema is present.
	// It never returns connection handles the caller must close.
	Health(ctx context.Context, instance SourceInstance) HealthResult

	// Scan reads sessions and turns from instance and produces neutral
	// ExperienceEnvelopes. Incomplete turns are skipped; the returned
	// NextCursor must be persisted by the caller and passed back on the
	// next call to enable idempotent incremental ingestion.
	Scan(ctx context.Context, req ScanRequest) (ScanResult, error)

	// ResolveTrace returns a bounded, redacted excerpt for the locator and
	// bounds requested. Full transcript content is not produced; the
	// adapter only ever returns references and excerpts sized to the
	// contract limits.
	ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult
}

// Adapter is the OpenCode implementation of ExperienceAdapter. It is
// deliberately stateless: all per-call state lives in the request and the
// source DB. Tests inject fixtures by setting DBPath explicitly.
type Adapter struct {
	// Now is injectable for deterministic tests. Nil falls back to time.Now.
	Now func() time.Time
	// TailQuietPeriod bounds the quiet window used to decide a turn is
	// stable. Zero falls back to defaultTailQuietPeriod.
	TailQuietPeriod time.Duration
}

// NewAdapter returns an Adapter with default timing settings.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns domain.SourceOpenCode. The remaining four methods live in
// dedicated files (discover.go, health.go, scan.go, resolve_trace.go) so
// each slice stays focused and reviewable.
func (a *Adapter) Name() domain.ExperienceSource {
	return domain.SourceOpenCode
}

// ResolveTrace is a stub. Slice 2.5 will replace this body with the real
// excerpt-and-redaction path.
func (a *Adapter) ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult {
	if err := ctx.Err(); err != nil {
		return TraceResult{
			Code:    string(domain.ErrTimeout),
			Message: err.Error(),
		}
	}
	return TraceResult{
		Code:    string(domain.ErrExperienceSchemaUnsupported),
		Message: "opencode adapter ResolveTrace is not implemented yet",
	}
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

// Compile-time guarantee that *Adapter satisfies ExperienceAdapter.
var _ ExperienceAdapter = (*Adapter)(nil)
