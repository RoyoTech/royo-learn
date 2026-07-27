package claudecode

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
	// projectRoot. It performs no I/O on the source files; locating means
	// resolving candidate paths against the configured root and reporting
	// which ones look like Claude Code session JSONL files.
	Discover(ctx context.Context, projectRoot string) ([]SourceInstance, error)

	// Health opens the source JSONL in read-only mode just long enough to
	// confirm the file is readable and the expected schema is present
	// (type / uuid / sessionId on the first object). It never returns
	// connection handles the caller must close.
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

// Adapter is the Claude Code implementation of ExperienceAdapter. It is
// deliberately stateless: all per-call state lives in the request and the
// source file. Tests inject fixtures by setting JSONLPath explicitly.
type Adapter struct {
	// Now is injectable for deterministic tests. Nil falls back to time.Now.
	Now func() time.Time
}

// NewAdapter returns an Adapter with default timing settings.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns domain.SourceClaudeCode. The remaining four methods live in
// dedicated files (discover.go, health.go, scan.go, resolve_trace.go) so each
// slice stays focused and reviewable.
func (a *Adapter) Name() domain.ExperienceSource {
	return domain.SourceClaudeCode
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

// Stub implementations for slice 10.0 scaffold. Each slice replaces its
// method body with the real implementation; the contract table at
// claudecode_test.go covers the surface. The signatures and error paths
// here are intentionally minimal so the package compiles and the contract
// tests are RED in the right places (cancellation, type mismatch).

// Health lives in health.go (slice 10.2).

// Scan returns a not-yet-implemented error. Slice 10.3 replaces this body.
func (a *Adapter) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	_ = ctx
	_ = req
	return ScanResult{Status: "degraded", Code: "experience_source_schema_unsupported", Message: "claudecode adapter: Scan not yet implemented", ScannedAt: a.now()}, nil
}

// ResolveTrace returns a not-yet-implemented error. Slice 10.5 replaces this
// body.
func (a *Adapter) ResolveTrace(ctx context.Context, locator domain.TranscriptLocator, bounds TraceBounds) TraceResult {
	_ = ctx
	_ = locator
	_ = bounds
	return TraceResult{Code: "experience_source_schema_unsupported", Message: "claudecode adapter: ResolveTrace not yet implemented"}
}

// Compile-time guarantee that *Adapter satisfies ExperienceAdapter.
var _ ExperienceAdapter = (*Adapter)(nil)
