package domain

import (
	"testing"
	"time"
)

func validTestLearning() *Learning {
	now := time.Now().UTC().Truncate(time.Second)
	return &Learning{
		ID:                   "learn-val-001",
		ProjectID:            "proj-001",
		Status:               StatusCaptured,
		Type:                 TypeProcedure,
		Title:                "Test Validation Learning",
		Context:              "A context for validation testing",
		Observation:          "An observation was made",
		ReusableLesson:       "Always validate inputs",
		RecommendedProcedure: []string{"validate required fields"},
		Limits:               "",
		ScopeGuess:           ScopeProject,
		Confidence:           ConfidenceHigh,
		EvidenceLevel:        EvidenceModerate,
		ProposedDestination:  DestProject,
		RetrievalTerms:       []string{"validation"},
		Fingerprint:          "fp-val-001",
		NormalizedHash:       "hash-val-001",
		Actor:                Actor{Kind: "agent", Name: "test", Model: "test-model", SessionID: "sess-001"},
		Revision:             1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestValidateLearning(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	if err := Validate(l); err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
}

func TestValidateLearningNil(t *testing.T) {
	t.Parallel()

	err := Validate(nil)
	if err == nil {
		t.Fatal("Validate(nil): expected error")
	}
}

func TestValidateLearningEmptyTitle(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Title = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty title: expected error")
	}
}

func TestValidateLearningEmptyContext(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Context = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty context: expected error")
	}
}

func TestValidateLearningEmptyObservation(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Observation = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty observation: expected error")
	}
}

func TestValidateLearningEmptyReusableLesson(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ReusableLesson = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty reusable_lesson: expected error")
	}
}

func TestValidateLearningEmptyProjectID(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ProjectID = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty ProjectID: expected error")
	}
}

func TestValidateLearningEmptyID(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ID = ""
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with empty ID: expected error")
	}
}

func TestValidateLearningInvalidStatus(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Status = "invalid_status"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid status: expected error")
	}
}

func TestValidateLearningInvalidType(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Type = "invalid_type"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid type: expected error")
	}
}

func TestValidateLearningInvalidScope(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ScopeGuess = "invalid_scope"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid scope: expected error")
	}
}

func TestValidateLearningInvalidConfidence(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Confidence = "invalid_confidence"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid confidence: expected error")
	}
}

func TestValidateLearningInvalidEvidenceLevel(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.EvidenceLevel = "invalid_level"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid evidence level: expected error")
	}
}

func TestValidateLearningInvalidDestination(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ProposedDestination = "invalid_dest"
	err := Validate(l)
	if err == nil {
		t.Fatal("Validate with invalid destination: expected error")
	}
}

func TestValidateLearningPreferenceScopeRule(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Type = TypePreference
	l.ProposedDestination = DestShared

	err := Validate(l)
	if err == nil {
		t.Fatal("preference type with shared destination should be rejected")
	}
}

func TestValidateLearningPreferenceAgentsRule(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.Type = TypePreference
	l.ProposedDestination = DestAgentsRule

	err := Validate(l)
	if err == nil {
		t.Fatal("preference type with agents_rule destination should be rejected")
	}
}

func TestValidateLearningPreferenceValidDestinations(t *testing.T) {
	t.Parallel()

	// preference can go to project or personal
	l := validTestLearning()
	l.Type = TypePreference
	l.ProposedDestination = DestProject
	if err := Validate(l); err != nil {
		t.Errorf("preference + project destination should be valid: %v", err)
	}

	l.ProposedDestination = DestNone
	if err := Validate(l); err != nil {
		t.Errorf("preference + none destination should be valid: %v", err)
	}
}

func TestValidateLearningRequiresScope(t *testing.T) {
	t.Parallel()

	l := validTestLearning()
	l.ScopeGuess = ScopeUnknown

	// Should still validate, scope_guess=unknown is a valid enum value.
	if err := Validate(l); err != nil {
		t.Errorf("scope_guess=unknown should pass validation: %v", err)
	}
}

func TestValidateEvidenceValid(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:          "ev-001",
		LearningID:  "learn-001",
		Kind:        KindTest,
		URI:         "file:///test/result.txt",
		Summary:     "Tests passed",
		SHA256:      "abc123",
		Command:     []string{"go", "test", "./..."},
		Redacted:    false,
		SizeBytes:   1024,
		CollectedAt: time.Now().UTC(),
	}

	if err := ValidateEvidence(e); err != nil {
		t.Fatalf("ValidateEvidence: unexpected error: %v", err)
	}
}

func TestValidateEvidenceNil(t *testing.T) {
	t.Parallel()

	err := ValidateEvidence(nil)
	if err == nil {
		t.Fatal("ValidateEvidence(nil): expected error")
	}
}

func TestValidateEvidenceEmptyID(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:         "",
		LearningID: "learn-001",
		Kind:       KindTest,
		Summary:    "Tests passed",
	}
	err := ValidateEvidence(e)
	if err == nil {
		t.Fatal("ValidateEvidence with empty ID: expected error")
	}
}

func TestValidateEvidenceEmptyLearningID(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:         "ev-001",
		LearningID: "",
		Kind:       KindTest,
		Summary:    "Tests passed",
	}
	err := ValidateEvidence(e)
	if err == nil {
		t.Fatal("ValidateEvidence with empty LearningID: expected error")
	}
}

func TestValidateEvidenceEmptyKind(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:         "ev-001",
		LearningID: "learn-001",
		Kind:       "",
		Summary:    "Tests passed",
	}
	err := ValidateEvidence(e)
	if err == nil {
		t.Fatal("ValidateEvidence with empty Kind: expected error")
	}
}

func TestValidateEvidenceInvalidKind(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:         "ev-001",
		LearningID: "learn-001",
		Kind:       "invalid_kind",
		Summary:    "Tests passed",
	}
	err := ValidateEvidence(e)
	if err == nil {
		t.Fatal("ValidateEvidence with invalid Kind: expected error")
	}
}

func TestValidateEvidenceEmptySummary(t *testing.T) {
	t.Parallel()

	e := &Evidence{
		ID:         "ev-001",
		LearningID: "learn-001",
		Kind:       KindTest,
		Summary:    "",
	}
	err := ValidateEvidence(e)
	if err == nil {
		t.Fatal("ValidateEvidence with empty Summary: expected error")
	}
}
