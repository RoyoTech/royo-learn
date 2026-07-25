// Contract tests for the promotion package (Hito 7 slice 7.0).
//
// These tests verify the structural invariants documented in
// docs/23-PATTERN-MINING.md §8 and docs/20-EXPERIENCE-INGESTION-PRD.md
// §6 RF-E08:
//
//   - SourceKind is a closed enum with the v1 canonical value
//     (pattern_mining) and rejects unknown kinds at construction.
//   - PromotionInput carries the documented fields and Validate
//     rejects nil input, empty pattern_id, missing actor, oversized
//     note, etc.
//   - PromotionResult exposes the documented fields with stable
//     JSON shape (was_new, audit_id, redaction_summary).
//   - RedactionSummary pins the JSON shape so the audit row can be
//     interpreted by external consumers.
//   - PromotionService is the documented interface; the production
//     TypeAssertion check confirms *Service implements it.
//   - PromotionInput.Validate rejects notes larger than
//     MaxPromotionNoteBytes before any DB work happens.
//   - All typed errors surface as the package-level sentinels
//     (ErrPromotionPatternNotFound, ErrPromotionNotEligible,
//     ErrPromotionAlreadyPromoted, ErrPromotionInvalidArgument,
//     ErrPromotionNotImplemented) and carry the canonical domain
//     code so the CLI/MCP layer can render stable error envelopes
//     without re-classifying them.
//
// Slice 7.0 ships only the contract. Behavioural tests for the
// redaction pipeline, the atomic transactional promotion, the
// idempotency guard, and the CLI/MCP integration land in slices
// 7.1–7.4 alongside their implementation files.

package promotion

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// TestSourceKind_Enum pins the closed enum so accidental renames are
// caught at compile time. Adding a new kind requires editing this
// table and the IsValidSourceKind switch.
func TestSourceKind_Enum(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind SourceKind
		want string
	}{
		{"pattern_mining", SourcePatternMining, "pattern_mining"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.kind) != tc.want {
				t.Fatalf("SourceKind(%s) = %q, want %q", tc.name, string(tc.kind), tc.want)
			}
		})
	}
}

// TestIsValidSourceKind verifies the closed enum check accepts the
// canonical value and rejects unknown kinds. The check is
// intentionally conservative: an unknown kind is rejected at the
// constructor, not at first use.
func TestIsValidSourceKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind SourceKind
		want bool
	}{
		{"pattern_mining", SourcePatternMining, true},
		{"empty", "", false},
		{"unknown", "unknown", false},
		{"typo", SourceKind("pattern_mining "), false},
		{"case_sensitive", SourceKind("PATTERN_MINING"), false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsValidSourceKind(tc.kind); got != tc.want {
				t.Fatalf("IsValidSourceKind(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// TestPromotionInput_Validate covers the documented validation rules.
func TestPromotionInput_Validate(t *testing.T) {
	t.Parallel()

	validActor := domain.Actor{Kind: "user", Name: "operator"}

	cases := []struct {
		name       string
		input      *PromotionInput
		wantErrSub string
	}{
		{"nil", nil, "input is nil"},
		{"empty_pattern_id", &PromotionInput{Actor: validActor}, "pattern id is required"},
		{"empty_actor", &PromotionInput{PatternID: "p1"}, "actor is required"},
		{"empty_actor_kind", &PromotionInput{PatternID: "p1", Actor: domain.Actor{Name: "operator"}}, "actor is required"},
		{"empty_actor_name", &PromotionInput{PatternID: "p1", Actor: domain.Actor{Kind: "user"}}, "actor is required"},
		{"note_too_large", &PromotionInput{PatternID: "p1", Actor: validActor, Note: strings.Repeat("x", MaxPromotionNoteBytes+1)}, "exceeds the permitted byte limit"},
		{"valid", &PromotionInput{PatternID: "p1", Actor: validActor}, ""},
		{"valid_with_note", &PromotionInput{PatternID: "p1", Actor: validActor, Note: "promote from review"}, ""},
		{"note_at_limit", &PromotionInput{PatternID: "p1", Actor: validActor, Note: strings.Repeat("x", MaxPromotionNoteBytes)}, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.input.Validate()
			if tc.wantErrSub == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("Validate() = %q, want error containing %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

// TestPromotionResult_StableJSON pins the JSON shape so external
// consumers (CLI, MCP, audit logs) can rely on it. The keys are
// pinned to snake_case by the package-level struct tags.
func TestPromotionResult_StableJSON(t *testing.T) {
	t.Parallel()

	res := &PromotionResult{
		PatternID:  "p1",
		LearningID: "l1",
		WasNew:     true,
		AuditID:    "a1",
		RedactionSummary: RedactionSummary{
			AnyRedacted:    true,
			RedactedFields: []string{"context"},
		},
	}
	enc, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(enc)
	for _, key := range []string{`"pattern_id":"p1"`, `"learning_id":"l1"`, `"was_new":true`, `"audit_id":"a1"`, `"any_redacted":true`, `"redacted_fields":["context"]`} {
		if !strings.Contains(got, key) {
			t.Fatalf("Marshal missing %q; got %s", key, got)
		}
	}
}

// TestRedactionSummary_OmitsEmptyFields pins the JSON shape so an
// empty RedactionSummary does not emit a noisy empty array.
func TestRedactionSummary_OmitsEmptyFields(t *testing.T) {
	t.Parallel()

	enc, err := json.Marshal(RedactionSummary{AnyRedacted: false})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(enc)
	want := `{"any_redacted":false}`
	if got != want {
		t.Fatalf("Marshal = %s, want %s", got, want)
	}
}

// TestPromotionService_InterfaceConformance verifies that *Service
// implements the PromotionService interface. A compile-time check
// would be ideal but the interface uses context.Context + pointers,
// so this is the explicit runtime check.
func TestPromotionService_InterfaceConformance(t *testing.T) {
	t.Parallel()

	// nil *Service is intentionally rejected with
	// ErrPromotionInvalidArgument; the interface contract is verified
	// at compile time by the assignment below.
	var _ PromotionService = (*Service)(nil)
}

// TestTypedErrors_AreDistinct checks that the package-level typed
// errors expose distinct canonical codes so the CLI/MCP layer can
// render stable error envelopes.
func TestTypedErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"ErrPromotionPatternNotFound", ErrPromotionPatternNotFound},
		{"ErrPromotionNotEligible", ErrPromotionNotEligible},
		{"ErrPromotionAlreadyPromoted", ErrPromotionAlreadyPromoted},
		{"ErrPromotionInvalidArgument", ErrPromotionInvalidArgument},
		{"ErrPromotionNotImplemented", ErrPromotionNotImplemented},
	}
	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			dom, ok := domain.AsDomainError(tc.err)
			if !ok {
				t.Fatalf("%s does not carry a DomainError (got %T)", tc.name, tc.err)
			}
			code := string(dom.Code)
			if other, exists := seen[code]; exists {
				t.Fatalf("canonical code %q shared by %s and %s", code, other, tc.name)
			}
			seen[code] = tc.name
		})
	}
}

// TestErrorIs_MatchesTypedErrors verifies that the ErrorIs helper
// returns true for matching typed errors and false for non-matching
// ones. Callers and tests rely on this to compare with errors.Is.
func TestErrorIs_MatchesTypedErrors(t *testing.T) {
	t.Parallel()

	if !ErrorIs(ErrPromotionPatternNotFound, ErrPromotionPatternNotFound) {
		t.Fatalf("ErrorIs(self, self) = false")
	}
	if !errors.Is(ErrPromotionPatternNotFound, ErrPromotionPatternNotFound) {
		t.Fatalf("errors.Is(self, self) = false")
	}
	if ErrorIs(ErrPromotionPatternNotFound, ErrPromotionNotEligible) {
		t.Fatalf("ErrorIs(PatternNotFound, NotEligible) = true, want false")
	}
}

// TestFormatPromotionContext_StableShape pins the audit-row context
// string format. Operators grep the audit log for "pattern=" and
// "source=" so the keys must stay stable.
func TestFormatPromotionContext_StableShape(t *testing.T) {
	t.Parallel()

	noNote := formatPromotionContext("p1", SourcePatternMining, "")
	if !strings.Contains(noNote, "pattern=p1") || !strings.Contains(noNote, "source=pattern_mining") {
		t.Fatalf("formatPromotionContext(no note) = %q, want pattern=p1 and source=pattern_mining", noNote)
	}
	withNote := formatPromotionContext("p1", SourcePatternMining, "reviewer note")
	if !strings.Contains(withNote, "note=reviewer note") {
		t.Fatalf("formatPromotionContext(with note) = %q, want note=reviewer note", withNote)
	}
}

// TestMaxPromotionNoteBytes_Stable pins the byte bound so the
// dismissal and promotion flows share the same audit-sink cap.
func TestMaxPromotionNoteBytes_Stable(t *testing.T) {
	t.Parallel()

	if MaxPromotionNoteBytes != 1024 {
		t.Fatalf("MaxPromotionNoteBytes = %d, want 1024 (matches patterns.MaxDismissalNoteBytes)", MaxPromotionNoteBytes)
	}
}
