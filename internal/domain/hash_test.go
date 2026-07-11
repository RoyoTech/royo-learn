package domain

import (
	"testing"
	"time"
)

func newHashTestLearning() *Learning {
	now := time.Now().UTC().Truncate(time.Second)
	return &Learning{
		ID:                   "learn-hash-001",
		ProjectID:            "proj-001",
		Status:               StatusCaptured,
		Type:                 TypeProcedure,
		Title:                "Test Hash Learning",
		Context:              "Testing hash determinism",
		Observation:          "The hash should be deterministic",
		ReusableLesson:       "Hashes must be stable",
		RecommendedProcedure: []string{"step1", "step2"},
		Limits:               "applies to Go",
		ScopeGuess:           ScopeProject,
		Confidence:           ConfidenceHigh,
		EvidenceLevel:        EvidenceModerate,
		ProposedDestination:  DestProject,
		RetrievalTerms:       []string{"hash", "test"},
		Fingerprint:          "fp-001",
		NormalizedHash:       "",
		Actor:                Actor{Kind: "agent", Name: "test", Model: "test-model", SessionID: "sess-001"},
		Revision:             1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestNormalizeIsDeterministic(t *testing.T) {
	t.Parallel()

	l := newHashTestLearning()

	normalized1, err := Normalize(l)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	normalized2, err := Normalize(l)
	if err != nil {
		t.Fatalf("Normalize (second): %v", err)
	}

	if len(normalized1) == 0 {
		t.Fatal("Normalize returned empty bytes")
	}

	if len(normalized1) != len(normalized2) {
		t.Fatal("Normalize returned different lengths for same input")
	}

	// Second call to Normalize must produce identical bytes.
	hash1, err := ComputeHash(l)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	hash2, err := ComputeHash(l)
	if err != nil {
		t.Fatalf("ComputeHash (second): %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("ComputeHash not deterministic: %q vs %q", hash1, hash2)
	}
	if hash1 == "" {
		t.Error("ComputeHash returned empty string")
	}
}

func TestSameContentSameHash(t *testing.T) {
	t.Parallel()

	l1 := newHashTestLearning()
	l2 := newHashTestLearning()
	// Same content, different IDs and timestamps (which are excluded from hash).
	l2.ID = "learn-hash-002"
	l2.CreatedAt = time.Now().Add(-1 * time.Hour)
	l2.UpdatedAt = time.Now().Add(-1 * time.Hour)

	hash1, err := ComputeHash(l1)
	if err != nil {
		t.Fatalf("ComputeHash l1: %v", err)
	}
	hash2, err := ComputeHash(l2)
	if err != nil {
		t.Fatalf("ComputeHash l2: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("Same content should produce same hash: %q vs %q", hash1, hash2)
	}
}

func TestDifferentContentDifferentHash(t *testing.T) {
	t.Parallel()

	l1 := newHashTestLearning()
	l2 := newHashTestLearning()
	l2.Title = "A completely different title"

	hash1, err := ComputeHash(l1)
	if err != nil {
		t.Fatalf("ComputeHash l1: %v", err)
	}
	hash2, err := ComputeHash(l2)
	if err != nil {
		t.Fatalf("ComputeHash l2: %v", err)
	}
	if hash1 == hash2 {
		t.Error("Different content should produce different hashes")
	}
}

func TestNormalizedFields(t *testing.T) {
	t.Parallel()

	fields := NormalizedFields()
	if len(fields) == 0 {
		t.Fatal("NormalizedFields returned empty slice")
	}

	// Make sure some key fields are present.
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}
	for _, required := range []string{"title", "context", "observation", "reusable_lesson", "type"} {
		if !fieldSet[required] {
			t.Errorf("NormalizedFields missing required field %q", required)
		}
	}

	// Make sure excluded fields are NOT present.
	for _, excluded := range []string{"id", "created_at", "updated_at", "revision"} {
		if fieldSet[excluded] {
			t.Errorf("NormalizedFields should NOT include %q", excluded)
		}
	}
}

func TestNormalizeNilLearning(t *testing.T) {
	t.Parallel()

	_, err := Normalize(nil)
	if err == nil {
		t.Fatal("Normalize(nil): expected error, got nil")
	}
}

func TestComputeHashNilLearning(t *testing.T) {
	t.Parallel()

	_, err := ComputeHash(nil)
	if err == nil {
		t.Fatal("ComputeHash(nil): expected error, got nil")
	}
}

func TestHashIsValidSHA256(t *testing.T) {
	t.Parallel()

	l := newHashTestLearning()
	hash, err := ComputeHash(l)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// SHA-256 hex is exactly 64 characters.
	if len(hash) != 64 {
		t.Errorf("Hash length = %d, want 64", len(hash))
	}

	for i, c := range hash {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Errorf("Hash character at position %d is %q, want lowercase hex", i, c)
		}
	}
}
