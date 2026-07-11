package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// NormalizedFields returns the list of Learning fields that are included in
// the canonical JSON for deduplication. Fields like ID, timestamps, revision,
// fingerprint, and normalized_hash are excluded because they vary between
// otherwise identical learnings.
func NormalizedFields() []string {
	return []string{
		"project_id",
		"type",
		"title",
		"context",
		"observation",
		"reusable_lesson",
		"recommended_procedure",
		"limits",
		"scope_guess",
		"confidence",
		"evidence_level",
		"proposed_destination",
		"retrieval_terms",
	}
}

// normalizable is the subset of Learning fields used for deduplication.
type normalizable struct {
	ProjectID            string   `json:"project_id"`
	Type                 string   `json:"type"`
	Title                string   `json:"title"`
	Context              string   `json:"context"`
	Observation          string   `json:"observation"`
	ReusableLesson       string   `json:"reusable_lesson"`
	RecommendedProcedure []string `json:"recommended_procedure"`
	Limits               string   `json:"limits"`
	ScopeGuess           string   `json:"scope_guess"`
	Confidence           string   `json:"confidence"`
	EvidenceLevel        string   `json:"evidence_level"`
	ProposedDestination  string   `json:"proposed_destination"`
	RetrievalTerms       []string `json:"retrieval_terms"`
}

// Normalize produces a canonical JSON representation of the dedup-relevant
// fields of learning. The output is suitable for hashing and comparison.
func Normalize(learning *Learning) ([]byte, error) {
	if learning == nil {
		return nil, fmt.Errorf("cannot normalize nil learning")
	}

	n := normalizable{
		ProjectID:            string(learning.ProjectID),
		Type:                 string(learning.Type),
		Title:                learning.Title,
		Context:              learning.Context,
		Observation:          learning.Observation,
		ReusableLesson:       learning.ReusableLesson,
		RecommendedProcedure: learning.RecommendedProcedure,
		Limits:               learning.Limits,
		ScopeGuess:           string(learning.ScopeGuess),
		Confidence:           string(learning.Confidence),
		EvidenceLevel:        string(learning.EvidenceLevel),
		ProposedDestination:  string(learning.ProposedDestination),
		RetrievalTerms:       learning.RetrievalTerms,
	}

	return json.Marshal(n)
}

// ComputeHash returns the SHA-256 hex digest of the normalized JSON.
func ComputeHash(learning *Learning) (string, error) {
	normalized, err := Normalize(learning)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return fmt.Sprintf("%x", sum), nil
}
