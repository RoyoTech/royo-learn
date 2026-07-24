package opencode

import (
	"context"
	"errors"
	"testing"

	"agent-royo-learn/internal/domain"
)

// TestSchemaTag pins the schema identifier. Bumping SchemaTag is a breaking
// change for any consumer that gates on it; this test makes that explicit.
func TestSchemaTag(t *testing.T) {
	if SchemaTag != "opencode/sqlite-v1" {
		t.Fatalf("SchemaTag = %q, want %q", SchemaTag, "opencode/sqlite-v1")
	}
}

// TestAdapter_ImplementsContract guarantees at compile time that *Adapter
// satisfies ExperienceAdapter. A drift here would be caught by `go build`,
// but the explicit assertion documents the intent for readers.
func TestAdapter_ImplementsContract(t *testing.T) {
	var _ ExperienceAdapter = (*Adapter)(nil)
	var _ ExperienceAdapter = NewAdapter()
}

// TestAdapter_Name verifies the canonical source identifier. Other agents in
// the codebase key on this string for routing and persistence.
func TestAdapter_Name(t *testing.T) {
	if got := NewAdapter().Name(); got != domain.SourceOpenCode {
		t.Fatalf("Name() = %q, want %q", got, domain.SourceOpenCode)
	}
}

// TestAdapter_RespectsContextCancellation verifies that every method bails
// out when the caller's context is already cancelled. The contract requires
// this so a hung upstream DB never blocks an ingestor shutdown.
func TestAdapter_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := NewAdapter()

	if _, err := adapter.Discover(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover with cancelled context error = %v, want context.Canceled", err)
	}
	if _, err := adapter.Scan(ctx, ScanRequest{ProjectRoot: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan with cancelled context error = %v, want context.Canceled", err)
	}
	health := adapter.Health(ctx, SourceInstance{DBPath: "/tmp/opencode.db"})
	if health.Code != string(domain.ErrTimeout) {
		t.Fatalf("Health with cancelled context code = %q, want %q", health.Code, domain.ErrTimeout)
	}
	trace := adapter.ResolveTrace(ctx, domain.TranscriptLocator{Kind: "sqlite", Path: "/tmp/opencode.db"}, TraceBounds{MaxBytes: 64})
	if trace.Code != string(domain.ErrTimeout) {
		t.Fatalf("ResolveTrace with cancelled context code = %q, want %q", trace.Code, domain.ErrTimeout)
	}
}

// TestNewAdapter_Defaults pins the default Adapter fields so future
// additions to the struct cannot silently change behavior.
func TestNewAdapter_Defaults(t *testing.T) {
	a := NewAdapter()
	if a.Now != nil {
		t.Fatalf("NewAdapter().Now is non-nil, want nil (default clock)")
	}
	if a.TailQuietPeriod != 0 {
		t.Fatalf("NewAdapter().TailQuietPeriod = %v, want zero value (caller decides)", a.TailQuietPeriod)
	}
}

// domainCode extracts a domain.ErrorCode from any error chain. It mirrors
// the helper used in internal/experience helpers_test.go so this file can
// stay self-contained.
func domainCode(err error) domain.ErrorCode {
	if err == nil {
		return ""
	}
	if domainErr, ok := domain.AsDomainError(err); ok {
		return domainErr.Code
	}
	return ""
}
