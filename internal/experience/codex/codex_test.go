package codex

import (
	"context"
	"errors"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestSchemaTag(t *testing.T) {
	if SchemaTag != "codex/rollout-v1" {
		t.Fatalf("SchemaTag = %q, want codex/rollout-v1", SchemaTag)
	}
}

func TestAdapter_ImplementsContract(t *testing.T) {
	var _ ExperienceAdapter = (*Adapter)(nil)
	var _ ExperienceAdapter = NewAdapter()
}

func TestAdapter_Name(t *testing.T) {
	if got := NewAdapter().Name(); got != domain.SourceCodex {
		t.Fatalf("Name() = %q, want %q", got, domain.SourceCodex)
	}
}

func TestAdapter_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()

	if _, err := adapter.Discover(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error = %v, want context.Canceled", err)
	}
	if _, err := adapter.Scan(ctx, ScanRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan error = %v, want context.Canceled", err)
	}
	if got := adapter.Health(ctx, SourceInstance{}).Code; got != string(domain.ErrTimeout) {
		t.Fatalf("Health code = %q, want %q", got, domain.ErrTimeout)
	}
	if got := adapter.ResolveTrace(ctx, domain.TranscriptLocator{}, TraceBounds{}).Code; got != string(domain.ErrTimeout) {
		t.Fatalf("ResolveTrace code = %q, want %q", got, domain.ErrTimeout)
	}
}

func TestNewAdapter_Defaults(t *testing.T) {
	if got := NewAdapter().Now; got != nil {
		t.Fatal("NewAdapter().Now is non-nil, want default clock")
	}
}

func domainCode(err error) domain.ErrorCode {
	if domainErr, ok := domain.AsDomainError(err); ok {
		return domainErr.Code
	}
	return ""
}
