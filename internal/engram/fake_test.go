package engram

import (
	"context"
	"errors"
	"testing"
)

func TestFakeClient_ImplementsClient(t *testing.T) {
	// Compile-time check that FakeClient satisfies the Client interface.
	var _ Client = (*FakeClient)(nil)
}

func TestFakeClient_Health_DefaultHealthy(t *testing.T) {
	c := NewFakeClient()
	result, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsHealthy() {
		t.Errorf("FakeClient should be healthy by default, got %v", result.Status)
	}
}

func TestFakeClient_Health_SetDegraded(t *testing.T) {
	c := NewFakeClient()
	c.SetHealth(HealthDegraded, "slow response")

	result, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsHealthy() {
		t.Error("expected degraded, got healthy")
	}
	if result.Message != "slow response" {
		t.Errorf("message = %q, want 'slow response'", result.Message)
	}
}

func TestFakeClient_Health_SetUnavailable(t *testing.T) {
	c := NewFakeClient()
	c.SetHealth(HealthUnavailable, "connection refused")

	result, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsAvailable() {
		t.Error("expected unavailable")
	}
}

func TestFakeClient_Search_ReturnsConfiguredResults(t *testing.T) {
	c := NewFakeClient()
	c.SetSearchResults([]SearchResult{
		{ID: "1", Title: "auth pattern", Score: 0.9},
		{ID: "2", Title: "db optimization", Score: 0.8},
	})

	results, err := c.Search(context.Background(), "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "auth pattern" {
		t.Errorf("results[0].Title = %q, want 'auth pattern'", results[0].Title)
	}
	if results[1].Score != 0.8 {
		t.Errorf("results[1].Score = %v, want 0.8", results[1].Score)
	}
}

func TestFakeClient_Search_EmptyResults(t *testing.T) {
	c := NewFakeClient()
	c.SetSearchResults(nil)

	results, err := c.Search(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestFakeClient_Search_ConfiguredError(t *testing.T) {
	c := NewFakeClient()
	want := errors.New("search failure")
	c.SetSearchError(want)

	_, err := c.Search(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("got error %v, want %v", err, want)
	}
}

func TestFakeClient_Context_ReturnsConfiguredData(t *testing.T) {
	c := NewFakeClient()
	c.SetContextResult(&ContextResult{
		RecentSessions: []SessionSummary{
			{ID: "s1", Goal: "added JWT auth", Discoveries: []string{"use RS256"}},
			{ID: "s2", Goal: "fixed memory leak", Discoveries: []string{"goroutine in defer"}},
		},
	})

	result, err := c.Context(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.RecentSessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(result.RecentSessions))
	}
	if result.RecentSessions[0].Goal != "added JWT auth" {
		t.Errorf("goal = %q, want 'added JWT auth'", result.RecentSessions[0].Goal)
	}
	if len(result.RecentSessions[0].Discoveries) != 1 {
		t.Errorf("discoveries count = %d, want 1", len(result.RecentSessions[0].Discoveries))
	}
}

func TestFakeClient_Context_Empty(t *testing.T) {
	c := NewFakeClient()

	result, err := c.Context(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.RecentSessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result.RecentSessions))
	}
}

func TestFakeClient_Save_Succeeds(t *testing.T) {
	c := NewFakeClient()

	result, err := c.Save(context.Background(), &SaveInput{
		Title:   "new pattern",
		Content: "discovered a useful pattern",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Saved {
		t.Error("expected Saved=true")
	}
}

func TestFakeClient_Save_SetDegraded(t *testing.T) {
	c := NewFakeClient()
	c.SetSaveDegraded(true)

	result, err := c.Save(context.Background(), &SaveInput{Title: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Saved {
		t.Error("expected Saved=false when degraded")
	}
	if !result.Degraded {
		t.Error("expected Degraded=true")
	}
}
