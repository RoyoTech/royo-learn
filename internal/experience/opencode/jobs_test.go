package opencode

import (
	"context"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/semantic"
)

func TestOpencodeJob_AccessorReturnsTypedJob(t *testing.T) {
	var accessor func() *semantic.Job = NewAdapter().Job
	if accessor() == nil {
		t.Fatal("Job() returned nil")
	}
}

func TestOpencodeJob_SourceMatches(t *testing.T) {
	if got := NewAdapter().Job().Source; got != string(domain.SourceOpenCode) {
		t.Fatalf("Source = %q, want %q", got, domain.SourceOpenCode)
	}
}

func TestOpencodeJob_DistinctPerCall(t *testing.T) {
	a := NewAdapter()
	if a.Job() == a.Job() {
		t.Fatal("Job() returned the same pointer twice")
	}
}

func TestOpencodeJob_FuncScansRealSQLite(t *testing.T) {
	path := newFixtureDB(t, nil)
	result, err := NewAdapter().Job().Func(context.Background(), semantic.Deps{SourceInstance: validScanInstance(path)})
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("envelopes = %d, want 0", len(result.Envelopes))
	}
}

func TestSemanticResult_ConvertsEnvelopeAndCursor(t *testing.T) {
	result := semanticResult([]experience.ExperienceEnvelope{{}}, 2, 3, map[string]any{"last_message_id": "m-1"})
	if len(result.Envelopes) != 1 || result.SkippedIncomplete != 2 || result.SkippedMalformed != 3 || result.NextCursor != "m-1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpencodeJob_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Job().Func(ctx, semantic.Deps{})
	if err != context.Canceled {
		t.Fatalf("Func error = %v, want context.Canceled", err)
	}
}
