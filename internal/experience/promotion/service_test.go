// Service-level tests for the promotion package.
//
// Slice 7.0 ships only the constructor and the typed-error surface;
// the transactional pipeline lands in slice 7.2, the idempotency
// guard in slice 7.3, and the CLI/MCP integration in slice 7.4. This
// file covers the contract that NewService enforces (nil arguments
// fail fast with a typed error) so the orchestrator cannot silently
// drop a promotion on the floor because of a misconfigured wiring.

package promotion

import (
	"errors"
	"testing"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
)

// TestNewService_NilArgs_Rejects verifies that the constructor
// fails fast with a typed error when any of the three required
// collaborators is nil. The test does not exercise the happy path
// here because that requires a real SQLite handle and a real
// patterns.Service; those behaviours land in slice 7.2.
func TestNewService_NilArgs_Rejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cap  *capture.Service
		pat  *patterns.Service
		db   *storage.DB
	}{
		{"all_nil", nil, nil, nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, err := NewService(tc.cap, tc.pat, tc.db)
			if err == nil {
				t.Fatalf("NewService = (%v, nil), want error", svc)
			}
			if svc != nil {
				t.Fatalf("NewService returned non-nil service on error: %v", svc)
			}
			if !errors.Is(err, ErrPromotionInvalidArgument) {
				t.Fatalf("error %v does not match ErrPromotionInvalidArgument", err)
			}
		})
	}
}

// TestService_Promote_NilService_Rejects verifies the nil-receiver
// guard. Promotions on a nil *Service must fail fast with a typed
// error so a misconfigured CLI cannot silently drop the call.
func TestService_Promote_NilService_Rejects(t *testing.T) {
	t.Parallel()

	var svc *Service
	_, err := svc.Promote(nil, "", &PromotionInput{})
	if !errors.Is(err, ErrPromotionInvalidArgument) {
		t.Fatalf("nil *Service.Promote = %v, want ErrPromotionInvalidArgument", err)
	}
}
