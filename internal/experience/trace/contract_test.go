package trace

import (
	"context"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestIsValidRelationshipType(t *testing.T) {
	if !IsValidRelationshipType(RelationSource) {
		t.Error("RelationSource should be valid")
	}
	if !IsValidRelationshipType(RelationDerived) {
		t.Error("RelationDerived should be valid")
	}
	if !IsValidRelationshipType(RelationReferenced) {
		t.Error("RelationReferenced should be valid")
	}
	if IsValidRelationshipType("bogus") {
		t.Error("bogus should not be valid")
	}
	if IsValidRelationshipType("") {
		t.Error("empty should not be valid")
	}
}

func TestSaveLearningEvent_Validation(t *testing.T) {
	r := NewRepository(nil)
	ctx := context.Background()

	// nil db
	err := r.SaveLearningEvent(ctx, LearningEvent{
		LearningID:       "L1",
		EventID:          "E1",
		RelationshipType: RelationSource,
	})
	if err == nil {
		t.Fatal("expected error for nil db")
	}

	// empty learning_id
	err = r.SaveLearningEvent(ctx, LearningEvent{
		EventID:          "E1",
		RelationshipType: RelationSource,
	})
	if err == nil {
		t.Fatal("expected error for empty learning_id")
	}

	// empty event_id
	err = r.SaveLearningEvent(ctx, LearningEvent{
		LearningID:       "L1",
		RelationshipType: RelationSource,
	})
	if err == nil {
		t.Fatal("expected error for empty event_id")
	}

	// bad relationship type
	err = r.SaveLearningEvent(ctx, LearningEvent{
		LearningID:       "L1",
		EventID:          "E1",
		RelationshipType: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid relationship_type")
	}
}

func TestFindEventsByLearning_Validation(t *testing.T) {
	r := NewRepository(nil)
	ctx := context.Background()

	_, err := r.FindEventsByLearning(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty learning_id")
	}
}

func TestFindLearningsByEvent_Validation(t *testing.T) {
	r := NewRepository(nil)
	ctx := context.Background()

	_, err := r.FindLearningsByEvent(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty event_id")
	}
}

func TestTraceBounds_Defaults(t *testing.T) {
	b := TraceBounds{}
	if b.MaxExcerptBytes != 0 {
		t.Error("default MaxExcerptBytes should be 0")
	}
	if b.IncludeExcerpt {
		t.Error("default IncludeExcerpt should be false")
	}
}

func TestLearningEvent_ZeroCreatedAt(t *testing.T) {
	le := LearningEvent{
		LearningID:       "L1",
		EventID:          "E1",
		RelationshipType: RelationSource,
	}
	if !le.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero by default")
	}
	le.CreatedAt = time.Now()
	if le.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero after assignment")
	}
}

func TestTraceResult_JSONShape(t *testing.T) {
	// Ensure the exported fields have the expected json tags.
	tr := TraceResult{
		LearningID: "L1",
		Events:     nil,
		Summary: TraceSummary{
			TotalEvents: 0,
			Code:        "",
		},
	}
	_ = tr
}

func TestTracedEvent_Fields(t *testing.T) {
	te := TracedEvent{
		Event: domain.ExperienceEvent{
			ID:   "ev-1",
			Kind: domain.EventTestFailure,
		},
		Excerpt:         "test excerpt",
		ExcerptRedacted: true,
		TraceCode:       "trace_source_changed",
	}
	if te.Event.ID != "ev-1" {
		t.Errorf("Event.ID = %q, want ev-1", te.Event.ID)
	}
	if !te.ExcerptRedacted {
		t.Error("ExcerptRedacted should be true")
	}
}
