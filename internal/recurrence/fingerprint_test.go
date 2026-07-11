package recurrence

import (
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestRecurrenceFingerprint_Deterministic(t *testing.T) {
	l1 := &domain.Learning{
		Title:          "Fix N+1 query",
		Observation:    "Noticed slow page loads from repeated DB calls",
		ReusableLesson: "Always eager-load associations",
	}
	l2 := &domain.Learning{
		Title:          "Fix N+1 query",
		Observation:    "Noticed slow page loads from repeated DB calls",
		ReusableLesson: "Always eager-load associations",
	}

	fp1 := RecurrenceFingerprint(l1)
	fp2 := RecurrenceFingerprint(l2)

	if fp1 != fp2 {
		t.Fatalf("fingerprints differ for identical content: %q vs %q", fp1, fp2)
	}
	if fp1 == "" {
		t.Fatal("fingerprint should not be empty")
	}
}

func TestRecurrenceFingerprint_DifferentContentProducesDifferentFP(t *testing.T) {
	l1 := &domain.Learning{
		Title:          "Fix memory leak",
		Observation:    "RSS grew over 24h",
		ReusableLesson: "Close file handles promptly",
	}
	l2 := &domain.Learning{
		Title:          "Optimize startup time",
		Observation:    "Startup took 30s in production",
		ReusableLesson: "Lazy-load heavy modules",
	}

	fp1 := RecurrenceFingerprint(l1)
	fp2 := RecurrenceFingerprint(l2)

	if fp1 == fp2 {
		t.Fatalf("different content produced same fingerprint: %q", fp1)
	}
}

func TestRecurrenceFingerprint_Normalization(t *testing.T) {
	l1 := &domain.Learning{
		Title:          "  Fix N+1 Query  ",
		Observation:    "Noticed  slow  page  loads",
		ReusableLesson: "ALWAYS EAGER-LOAD ASSOCIATIONS",
	}
	l2 := &domain.Learning{
		Title:          "fix n+1 query",
		Observation:    "noticed slow page loads",
		ReusableLesson: "always eager-load associations",
	}

	fp1 := RecurrenceFingerprint(l1)
	fp2 := RecurrenceFingerprint(l2)

	if fp1 != fp2 {
		t.Fatalf("normalization failed: fingerprints differ: %q vs %q", fp1, fp2)
	}
}

func TestRecurrenceFingerprint_NilLearning(t *testing.T) {
	fp := RecurrenceFingerprint(nil)
	if fp != "" {
		t.Fatalf("expected empty fingerprint for nil learning, got %q", fp)
	}
}
