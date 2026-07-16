package recurrence

import (
	"context"
	"testing"

	"agent-royo-learn/internal/domain"
)

// TestListRecurrences_ReturnsNineFields proves an occurrence recorded with the
// full detail the plan 4.4 enumerates round-trips through every read path, not
// only through the idempotency-key lookup. The nine fields are: learning ID,
// fingerprint, event (summary), date (occurred_at), result (outcome), whether
// the learning was retrieved, whether the skill activated, evidence, and actor.
func TestListRecurrences_ReturnsNineFields(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	in := OccurrenceInput{
		Summary:        "the crash came back on deploy",
		Outcome:        string(domain.OutcomeRecurred),
		Retrieved:      true,
		SkillActivated: true,
		Evidence:       "evidence://run/42",
		Actor:          domain.Actor{Kind: "agent", Name: "session-bot"},
		IdempotencyKey: "occ-nine-1",
	}
	rec, isNew, err := RecordOccurrence(ctx, db, proj.ID, learning, in)
	if err != nil {
		t.Fatalf("RecordOccurrence: %v", err)
	}
	if !isNew {
		t.Fatal("first occurrence should be new")
	}

	assertNine := func(name string, got *domain.RecurrenceRecord) {
		t.Helper()
		if got.LearningID != learning.ID {
			t.Errorf("%s: LearningID = %q, want %q", name, got.LearningID, learning.ID)
		}
		if got.RecurrenceFingerprint != rec.RecurrenceFingerprint {
			t.Errorf("%s: fingerprint = %q, want %q", name, got.RecurrenceFingerprint, rec.RecurrenceFingerprint)
		}
		if got.Summary != in.Summary {
			t.Errorf("%s: Summary (event) = %q, want %q", name, got.Summary, in.Summary)
		}
		if got.OccurredAt.IsZero() {
			t.Errorf("%s: OccurredAt (date) is zero", name)
		}
		if got.Outcome != in.Outcome {
			t.Errorf("%s: Outcome (result) = %q, want %q", name, got.Outcome, in.Outcome)
		}
		if !got.Retrieved {
			t.Errorf("%s: Retrieved = false, want true", name)
		}
		if !got.SkillActivated {
			t.Errorf("%s: SkillActivated = false, want true", name)
		}
		if got.Evidence != in.Evidence {
			t.Errorf("%s: Evidence = %q, want %q", name, got.Evidence, in.Evidence)
		}
		if got.ActorKind != in.Actor.Kind || got.ActorName != in.Actor.Name {
			t.Errorf("%s: actor = %q/%q, want %q/%q", name, got.ActorKind, got.ActorName, in.Actor.Kind, in.Actor.Name)
		}
	}

	byLearning, err := ListRecurrencesForLearning(ctx, db, learning.ID, 10)
	if err != nil {
		t.Fatalf("ListRecurrencesForLearning: %v", err)
	}
	if len(byLearning) != 1 {
		t.Fatalf("ListRecurrencesForLearning returned %d records, want 1", len(byLearning))
	}
	assertNine("ListRecurrencesForLearning", byLearning[0])

	all, err := ListAllRecurrences(ctx, db, proj.ID, 10)
	if err != nil {
		t.Fatalf("ListAllRecurrences: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListAllRecurrences returned %d records, want 1", len(all))
	}
	assertNine("ListAllRecurrences", all[0])
}

// TestComputeMetrics_FourStates proves the metrics distinguish the four states
// plan 4.4 requires: zero recurrences, insufficient data, repeated recurrence,
// and prevented recurrence.
func TestComputeMetrics_FourStates(t *testing.T) {
	t.Run("zero_recurrences", func(t *testing.T) {
		db, proj := setupRecurrenceDB(t)
		ctx := context.Background()
		learning := newTestLearning()
		saveLearningInDB(t, db, learning, proj.ID)

		fp := RecurrenceFingerprint(learning)
		m, err := ComputeMetrics(ctx, db, proj.ID, fp)
		if err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
		if m.State != domain.StateZeroRecurrences {
			t.Fatalf("State = %q, want %q", m.State, domain.StateZeroRecurrences)
		}
	})

	t.Run("insufficient_data", func(t *testing.T) {
		db, proj := setupRecurrenceDB(t)
		ctx := context.Background()
		learning := newTestLearning()
		saveLearningInDB(t, db, learning, proj.ID)

		if _, _, err := RecordOccurrence(ctx, db, proj.ID, learning, OccurrenceInput{
			Outcome: string(domain.OutcomeRecurred),
			Actor:   domain.Actor{Kind: "agent", Name: "bot"},
		}); err != nil {
			t.Fatalf("RecordOccurrence: %v", err)
		}

		fp := RecurrenceFingerprint(learning)
		m, err := ComputeMetrics(ctx, db, proj.ID, fp)
		if err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
		if m.State != domain.StateInsufficientData {
			t.Fatalf("State = %q, want %q", m.State, domain.StateInsufficientData)
		}
	})

	t.Run("repeated_recurrence", func(t *testing.T) {
		db, proj := setupRecurrenceDB(t)
		ctx := context.Background()
		learning := newTestLearning()
		saveLearningInDB(t, db, learning, proj.ID)

		for i := 0; i < 2; i++ {
			if _, _, err := RecordOccurrence(ctx, db, proj.ID, learning, OccurrenceInput{
				Outcome: string(domain.OutcomeRecurred),
				Actor:   domain.Actor{Kind: "agent", Name: "bot"},
			}); err != nil {
				t.Fatalf("RecordOccurrence #%d: %v", i, err)
			}
		}

		fp := RecurrenceFingerprint(learning)
		m, err := ComputeMetrics(ctx, db, proj.ID, fp)
		if err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
		if m.State != domain.StateRepeatedRecurrence {
			t.Fatalf("State = %q, want %q", m.State, domain.StateRepeatedRecurrence)
		}
	})

	t.Run("prevented_recurrence", func(t *testing.T) {
		db, proj := setupRecurrenceDB(t)
		ctx := context.Background()
		learning := newTestLearning()
		saveLearningInDB(t, db, learning, proj.ID)

		if _, _, err := RecordOccurrence(ctx, db, proj.ID, learning, OccurrenceInput{
			Outcome:        string(domain.OutcomePrevented),
			Retrieved:      true,
			SkillActivated: true,
			Actor:          domain.Actor{Kind: "agent", Name: "bot"},
		}); err != nil {
			t.Fatalf("RecordOccurrence: %v", err)
		}

		fp := RecurrenceFingerprint(learning)
		m, err := ComputeMetrics(ctx, db, proj.ID, fp)
		if err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
		if m.State != domain.StatePreventedRecurrence {
			t.Fatalf("State = %q, want %q", m.State, domain.StatePreventedRecurrence)
		}
	})
}
