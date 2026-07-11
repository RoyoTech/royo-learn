package engram

import (
	"context"
	"errors"
	"testing"
)

func TestDegradedClient_ImplementsClient(t *testing.T) {
	var _ Client = (*DegradedClient)(nil)
}

func TestDegradedClient_Health_DelegatesToInner(t *testing.T) {
	inner := NewFakeClient()
	inner.SetHealth(HealthDegraded, "slow")
	dc := NewDegradedClient(inner)

	result, err := dc.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsHealthy() {
		t.Error("expected degraded")
	}
	if result.Message != "slow" {
		t.Errorf("message = %q, want 'slow'", result.Message)
	}
}

func TestDegradedClient_Search_WhenHealthy_Delegates(t *testing.T) {
	inner := NewFakeClient()
	inner.SetSearchResults([]SearchResult{
		{ID: "1", Title: "result"},
	})
	dc := NewDegradedClient(inner)

	results, err := dc.Search(context.Background(), "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestDegradedClient_Search_WhenUnavailable_ReturnsDegraded(t *testing.T) {
	inner := NewFakeClient()
	inner.SetHealth(HealthUnavailable, "down")
	dc := NewDegradedClient(inner)

	results, err := dc.Search(context.Background(), "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return empty results, not error
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestDegradedClient_Context_WhenUnavailable_ReturnsEmpty(t *testing.T) {
	inner := NewFakeClient()
	inner.SetHealth(HealthUnavailable, "down")
	dc := NewDegradedClient(inner)

	result, err := dc.Context(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.RecentSessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result.RecentSessions))
	}
}

func TestDegradedClient_Save_WhenUnavailable_ReturnsDegradedResult(t *testing.T) {
	inner := NewFakeClient()
	inner.SetHealth(HealthUnavailable, "down")
	dc := NewDegradedClient(inner)

	result, err := dc.Save(context.Background(), &SaveInput{Title: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Saved {
		t.Error("expected Saved=false when unavailable")
	}
	if !result.Degraded {
		t.Error("expected Degraded=true")
	}
}

func TestDegradedClient_Save_WhenDegraded_StillSaves(t *testing.T) {
	// Degraded (not unavailable) should still attempt the save.
	inner := NewFakeClient()
	inner.SetHealth(HealthDegraded, "slow but available")
	dc := NewDegradedClient(inner)

	result, err := dc.Save(context.Background(), &SaveInput{Title: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Saved {
		t.Error("expected Saved=true when degraded but available")
	}
}

func TestDegradedClient_HealthError_DeclaresDegraded(t *testing.T) {
	// When the inner health check itself errors, treat as unavailable.
	inner := NewFakeClient()
	inner.SetSearchError(errors.New("health check failed"))
	// Create a minimal fake that errors on Health
	dc := NewDegradedClient(&errorHealthFake{err: errors.New("boom")})

	result, err := dc.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsAvailable() {
		t.Error("health error should result in unavailable")
	}
}

// errorHealthFake is a minimal Client that always errors on Health.
type errorHealthFake struct {
	err error
}

func (f *errorHealthFake) Health(_ context.Context) (HealthResult, error) {
	return HealthResult{}, f.err
}
func (f *errorHealthFake) Search(_ context.Context, _ string) ([]SearchResult, error) {
	return nil, f.err
}
func (f *errorHealthFake) Context(_ context.Context) (*ContextResult, error) {
	return &ContextResult{}, f.err
}
func (f *errorHealthFake) Save(_ context.Context, _ *SaveInput) (*SaveResult, error) {
	return &SaveResult{}, f.err
}
