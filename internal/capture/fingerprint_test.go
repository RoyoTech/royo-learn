package capture

import (
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func newTestCaptureLearning() *domain.Learning {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.Learning{
		ID:                   "learn-cap-001",
		ProjectID:            "proj-cap-001",
		Status:               domain.StatusCaptured,
		Type:                 domain.TypeProcedure,
		Title:                "Test Capture",
		Context:              "Testing the capture service",
		Observation:          "The capture works correctly",
		ReusableLesson:       "Always test capture idempotency",
		RecommendedProcedure: []string{"verify dedup", "check markdown"},
		Limits:               "n/a",
		ScopeGuess:           domain.ScopeProject,
		Confidence:           domain.ConfidenceHigh,
		EvidenceLevel:        domain.EvidenceModerate,
		ProposedDestination:  domain.DestProject,
		RetrievalTerms:       []string{"capture", "test"},
		Fingerprint:          "fp-cap-001",
		Actor: domain.Actor{
			Kind:      "agent",
			Name:      "test-agent",
			Model:     "test-model",
			SessionID: "sess-001",
		},
		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	t.Parallel()

	l1 := &domain.Learning{
		Title:       "Test fingerprinting",
		Context:     "Same context",
		Observation: "Same observation",
		Type:        domain.TypeProcedure,
	}

	l2 := &domain.Learning{
		Title:       "Test fingerprinting",
		Context:     "Same context",
		Observation: "Same observation",
		Type:        domain.TypeProcedure,
	}

	fp1 := Fingerprint(l1)
	fp2 := Fingerprint(l2)

	if fp1 == "" {
		t.Fatal("Fingerprint returned empty string")
	}
	if fp1 != fp2 {
		t.Errorf("Same content produced different fingerprints: %q vs %q", fp1, fp2)
	}
}

func TestFingerprintDifferentContent(t *testing.T) {
	t.Parallel()

	l1 := &domain.Learning{Title: "Title A", Context: "Context A", Observation: "Obs A", Type: domain.TypeProcedure}
	l2 := &domain.Learning{Title: "Title B", Context: "Context B", Observation: "Obs B", Type: domain.TypeProcedure}

	fp1 := Fingerprint(l1)
	fp2 := Fingerprint(l2)

	if fp1 == fp2 {
		t.Error("Different content should produce different fingerprints")
	}
}

func TestFingerprintNilLearning(t *testing.T) {
	t.Parallel()

	fp := Fingerprint(nil)
	if fp != "" {
		t.Errorf("Fingerprint(nil) = %q, want empty string", fp)
	}
}
