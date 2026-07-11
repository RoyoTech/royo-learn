package recurrence

import (
	"context"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestComputeMetrics_SingleOccurrence(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	_, err := RecordRecurrence(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("RecordRecurrence: %v", err)
	}

	fp := RecurrenceFingerprint(learning)
	metrics, err := ComputeMetrics(ctx, db, proj.ID, fp)
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	if metrics.Count != 1 {
		t.Fatalf("Count = %d, want 1", metrics.Count)
	}
	if metrics.Trend != domain.TrendFirst {
		t.Fatalf("Trend = %q, want %q", metrics.Trend, domain.TrendFirst)
	}
	if metrics.FirstSeen.IsZero() {
		t.Fatal("FirstSeen should not be zero")
	}
	if metrics.LastSeen.IsZero() {
		t.Fatal("LastSeen should not be zero")
	}
}

func TestComputeMetrics_MultipleOccurrences(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		l := newTestLearning()
		// Set content identical so all produce same fingerprint.
		l.Title = "Same Pattern"
		l.Observation = "Same observation"
		l.ReusableLesson = "Same lesson"
		saveLearningInDB(t, db, l, proj.ID)
		if _, err := RecordRecurrence(ctx, db, proj.ID, l); err != nil {
			t.Fatalf("RecordRecurrence #%d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond) // ensure different timestamps
	}

	fp := RecurrenceFingerprint(&domain.Learning{
		Title:          "Same Pattern",
		Observation:    "Same observation",
		ReusableLesson: "Same lesson",
	})

	metrics, err := ComputeMetrics(ctx, db, proj.ID, fp)
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	if metrics.Count != 3 {
		t.Fatalf("Count = %d, want 3", metrics.Count)
	}
	if metrics.FirstSeen.Equal(metrics.LastSeen) || metrics.FirstSeen.After(metrics.LastSeen) {
		t.Fatalf("FirstSeen (%v) should be before LastSeen (%v)", metrics.FirstSeen, metrics.LastSeen)
	}
	if !metrics.LastSeen.After(metrics.FirstSeen) {
		t.Fatal("LastSeen should be after FirstSeen")
	}
}

func TestComputeMetrics_EmptyFingerprint(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()

	metrics, err := ComputeMetrics(ctx, db, proj.ID, "nonexistent-fingerprint")
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if metrics.Count != 0 {
		t.Fatalf("Count = %d, want 0", metrics.Count)
	}
	if metrics.Trend != domain.TrendFirst {
		t.Fatalf("Trend = %q, want %q", metrics.Trend, domain.TrendFirst)
	}
}
