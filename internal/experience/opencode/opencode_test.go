package opencode

import (
	"context"
	"errors"
	"testing"
	"time"

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

// TestAdapter_Discover_StubReturnsTypedError verifies that the scaffold's
// Discover stub signals an unimplemented state through the documented
// ErrExperienceSchemaUnsupported error. Slice 2.1 will replace this body
// with the real path-security walk; the error code will then become
// ErrExperienceLocatorOutsideRoot or ErrExperienceSourceNotFound as
// appropriate.
func TestAdapter_Discover_StubReturnsTypedError(t *testing.T) {
	adapter := NewAdapter()
	instances, err := adapter.Discover(context.Background(), t.TempDir())
	if err == nil {
		t.Fatalf("Discover returned nil error and %d instances, want typed error", len(instances))
	}
	if len(instances) != 0 {
		t.Fatalf("Discover returned %d instances on stub path, want 0", len(instances))
	}
	if domainCode(err) != domain.ErrExperienceSchemaUnsupported {
		t.Fatalf("Discover error code = %v, want %q", err, domain.ErrExperienceSchemaUnsupported)
	}
}

// TestAdapter_Health_StubReturnsTypedError verifies that the scaffold's
// Health stub returns an error HealthResult with the documented error code.
// Slice 2.2 will replace this with the real read-only check.
func TestAdapter_Health_StubReturnsTypedError(t *testing.T) {
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }

	result := adapter.Health(context.Background(), SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: t.TempDir(),
		DBPath:      "/tmp/opencode.db",
		Schema:      SchemaTag,
		Discovered:  time.Unix(0, 0).UTC(),
	})
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
	if result.DBPath != "/tmp/opencode.db" {
		t.Fatalf("Health DBPath = %q, want %q", result.DBPath, "/tmp/opencode.db")
	}
	if result.CheckedAt.IsZero() {
		t.Fatalf("Health CheckedAt is zero, want the configured clock value")
	}
	if result.Readable || result.SchemaOK {
		t.Fatalf("Health flags readable=%v schemaOK=%v, want both false on stub", result.Readable, result.SchemaOK)
	}
}

// TestAdapter_Scan_StubReturnsTypedError verifies that the scaffold's Scan
// stub reports an unimplemented state via ScanResult.Code and does not
// surface any partial envelopes. Slice 2.3 will replace this body with the
// real envelope construction.
func TestAdapter_Scan_StubReturnsTypedError(t *testing.T) {
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }

	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: t.TempDir(),
		Instance: SourceInstance{
			Source: domain.SourceOpenCode,
			DBPath: "/tmp/opencode.db",
			Schema: SchemaTag,
		},
	})
	if err != nil {
		t.Fatalf("Scan returned error on stub: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("Scan Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Scan returned %d envelopes on stub, want 0", len(result.Envelopes))
	}
	if result.ScannedAt.IsZero() {
		t.Fatalf("Scan ScannedAt is zero, want the configured clock value")
	}
}

// TestAdapter_ResolveTrace_StubReturnsTypedError verifies the scaffold's
// ResolveTrace stub. Slice 2.5 will replace this body with the real
// excerpt-and-redaction path.
func TestAdapter_ResolveTrace_StubReturnsTypedError(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:      "sqlite",
		Path:      "/tmp/opencode.db",
		SessionID: "session",
		TurnID:    "turn",
		Offset:    0,
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
	if result.Excerpt != "" {
		t.Fatalf("ResolveTrace Excerpt = %q, want empty on stub", result.Excerpt)
	}
	if result.Redacted || result.SourceChanged {
		t.Fatalf("ResolveTrace Redacted=%v SourceChanged=%v, want both false on stub", result.Redacted, result.SourceChanged)
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
