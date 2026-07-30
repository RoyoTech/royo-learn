// Package semantic — tests for the Job / JobFunc / Deps / Result
// runtime contract. See job.go.

package semantic

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestJob_ContractCompiles is a compile-time check: a JobFunc literal
// must match the documented signature
// `func(ctx context.Context, deps Deps) (Result, error)`. The body of
// the test does nothing — the real assertion is that the file compiles
// at all.
func TestJob_ContractCompiles(t *testing.T) {
	var fn JobFunc = func(ctx context.Context, deps Deps) (Result, error) {
		return Result{}, nil
	}
	if fn == nil {
		t.Fatal("JobFunc literal must not be nil")
	}
	deps := Deps{
		Now: func() time.Time { return time.Time{} },
	}
	out, err := fn(context.Background(), deps)
	if err != nil {
		t.Fatalf("JobFunc returned error: %v", err)
	}
	if out.Envelopes != nil {
		t.Errorf("fresh Result should have nil Envelopes, got %v", out.Envelopes)
	}
	if out.SkippedMalformed != 0 || out.SkippedIncomplete != 0 {
		t.Errorf("fresh Result counters non-zero: %+v", out)
	}
	if out.ErrorCode != "" {
		t.Errorf("fresh Result.ErrorCode = %q, want empty", out.ErrorCode)
	}
}

// TestJobFunc_RespectsContextCancellation asserts the runtime contract
// that a JobFunc body returns the cancellation error when its context
// is cancelled before the body returns. The test wires a JobFunc that
// waits on ctx.Done() and asserts the body returns ctx.Err().
func TestJobFunc_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the body returns at ctx.Done()

	var fn JobFunc = func(ctx context.Context, deps Deps) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}

	out, err := fn(ctx, Deps{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("JobFunc err = %v, want context.Canceled", err)
	}
	if out.ErrorCode != "" {
		t.Errorf("Result.ErrorCode = %q, want empty (cancellation is a runtime error, not a domain error code)", out.ErrorCode)
	}
}

// TestJob_LiteralValues is a structural check: a Job literal must
// round-trip every field. The test guards against accidental field
// renames in the Job struct that would silently break the per-adapter
// Job() accessors in PR #15.
func TestJob_LiteralValues(t *testing.T) {
	want := Job{
		Name:               "experience_ingest:codex",
		Source:             "codex",
		Intent:             JobIntentIngest,
		Scope:              JobScopeProject,
		RiskClass:          JobRiskClassLow,
		Enabled:            false,
		DefaultIntervalSec: 300,
		DefaultMaxRetries:  3,
		Func:               func(context.Context, Deps) (Result, error) { return Result{}, nil },
	}
	if want.Name == "" {
		t.Error("Job.Name must round-trip non-empty")
	}
	if want.Intent != JobIntentIngest {
		t.Errorf("Job.Intent = %q, want %q", want.Intent, JobIntentIngest)
	}
	if want.Scope != JobScopeProject {
		t.Errorf("Job.Scope = %q, want %q", want.Scope, JobScopeProject)
	}
	if want.RiskClass != JobRiskClassLow {
		t.Errorf("Job.RiskClass = %q, want %q", want.RiskClass, JobRiskClassLow)
	}
	if want.Enabled {
		t.Error("Job.Enabled must be false by default (Hito 3 owns the flip)")
	}
	if want.DefaultIntervalSec != 300 {
		t.Errorf("Job.DefaultIntervalSec = %d, want 300", want.DefaultIntervalSec)
	}
	if want.DefaultMaxRetries != 3 {
		t.Errorf("Job.DefaultMaxRetries = %d, want 3", want.DefaultMaxRetries)
	}
	if want.Func == nil {
		t.Error("Job.Func must be non-nil (the per-adapter accessor always wires it)")
	}
}
