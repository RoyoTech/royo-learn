package opencode

import (
	"context"
	"fmt"
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

// Name returns domain.SourceOpenCode. This is the only method with real
// behavior in the scaffold; the remaining four return typed errors until
// slices 2.1-2.5 land.
func (a *Adapter) Name() domain.ExperienceSource {
	return domain.SourceOpenCode
}

// Discover is a stub. It returns ErrExperienceSchemaUnsupported to signal
// that the discovery path is not implemented yet. Slices 2.1 will replace
// this body with the real path-security walk.
func (a *Adapter) Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
		"opencode adapter Discover is not implemented yet")
}

// Health is a stub. It returns an error HealthResult so callers can detect
// the unimplemented state without panicking.
func (a *Adapter) Health(ctx context.Context, instance SourceInstance) HealthResult {
	if err := ctx.Err(); err != nil {
		return HealthResult{
			Status:    "error",
			DBPath:    instance.DBPath,
			Code:      string(domain.ErrTimeout),
			Message:   err.Error(),
			CheckedAt: a.now(),
		}
	}
	return HealthResult{
		Status:    "error",
		DBPath:    instance.DBPath,
		Code:      string(domain.ErrExperienceSchemaUnsupported),
		Message:   fmt.Sprintf("opencode adapter Health is not implemented yet (schema=%s)", SchemaTag),
		CheckedAt: a.now(),
	}
}

// Scan is a stub. Slices 2.3-2.4 will replace this body with the real
// envelope construction and idempotent cursor handling.
func (a *Adapter) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{
		Instance:  req.Instance,
		Status:    "error",
		Code:      string(domain.ErrExperienceSchemaUnsupported),
		Message:   "opencode adapter Scan is not implemented yet",
		ScannedAt: a.now(),
	}, nil
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
