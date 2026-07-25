package opencode

import (
	"errors"
	"testing"

	"agent-royo-learn/internal/domain"
	projectpath "agent-royo-learn/internal/project"
)

// TestCursorCheckpoint_NumericVariants covers the four numeric value types the
// cursor map can carry depending on how the caller constructed it. The base
// scan test only exercises the int64 path; this test pins the int, float64,
// and int32 cases so future refactors cannot regress cursor decoding.
func TestCursorCheckpoint_NumericVariants(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		wantSeq int64
		wantOK  bool
	}{
		{"int64", int64(7), 7, true},
		{"int", int(8), 8, true},
		{"float64", float64(9), 9, true},
		{"int32", int32(10), 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cursor := map[string]any{
				"last_session_id": "s-1",
				"last_sequence":   tc.value,
			}
			sid, seq, ok := cursorCheckpoint(cursor)
			if !ok {
				t.Fatalf("cursorCheckpoint(%v) ok = false, want true", tc.value)
			}
			if sid != "s-1" {
				t.Fatalf("session = %q, want s-1", sid)
			}
			if seq != tc.wantSeq {
				t.Fatalf("sequence = %d, want %d", seq, tc.wantSeq)
			}
		})
	}
}

// TestCursorCheckpoint_EmptyAndUnknownType covers the negative paths of
// cursorCheckpoint: nil cursor, empty session id, and an unknown numeric type.
func TestCursorCheckpoint_EmptyAndUnknownType(t *testing.T) {
	if _, _, ok := cursorCheckpoint(nil); ok {
		t.Fatalf("nil cursor reported ok = true, want false")
	}
	if _, _, ok := cursorCheckpoint(map[string]any{}); ok {
		t.Fatalf("empty cursor reported ok = true, want false")
	}
	if _, _, ok := cursorCheckpoint(map[string]any{"last_session_id": "", "last_sequence": int64(1)}); ok {
		t.Fatalf("empty session id reported ok = true, want false")
	}
	if _, _, ok := cursorCheckpoint(map[string]any{"last_session_id": "s", "last_sequence": "string-not-supported"}); ok {
		t.Fatalf("unsupported type reported ok = true, want false")
	}
}

// TestLocatorError_Mapping covers the three branches of locatorError: the
// symlink-escape branch, the path-outside-root branch, and the fallback for
// any other error value. The base discover tests exercise the happy path and
// the generic failure path but do not drive the symlink / outside-root
// switch arms directly.
func TestLocatorError_Mapping(t *testing.T) {
	t.Run("symlink_escape", func(t *testing.T) {
		in := &projectpath.Error{Code: projectpath.ErrSymlinkEscape, Message: "test"}
		got := locatorError(in)
		var verr *domain.ValidationError
		if !errors.As(got, &verr) {
			t.Fatalf("got %T, want *domain.ValidationError", got)
		}
		if verr.Code != domain.ErrExperienceLocatorOutsideRoot {
			t.Fatalf("code = %q, want %q", verr.Code, domain.ErrExperienceLocatorOutsideRoot)
		}
	})
	t.Run("path_outside_root", func(t *testing.T) {
		in := &projectpath.Error{Code: projectpath.ErrPathOutsideRoot, Message: "test"}
		got := locatorError(in)
		var verr *domain.ValidationError
		if !errors.As(got, &verr) {
			t.Fatalf("got %T, want *domain.ValidationError", got)
		}
		if verr.Code != domain.ErrExperienceLocatorOutsideRoot {
			t.Fatalf("code = %q, want %q", verr.Code, domain.ErrExperienceLocatorOutsideRoot)
		}
	})
	t.Run("fallback_unknown", func(t *testing.T) {
		got := locatorError(errors.New("any other error"))
		var verr *domain.ValidationError
		if !errors.As(got, &verr) {
			t.Fatalf("got %T, want *domain.ValidationError", got)
		}
		if verr.Code != domain.ErrExperienceLocatorOutsideRoot {
			t.Fatalf("code = %q, want %q", verr.Code, domain.ErrExperienceLocatorOutsideRoot)
		}
	})
	t.Run("fallback_unrelated_projectpath_code", func(t *testing.T) {
		in := &projectpath.Error{Code: "some_other_code", Message: "test"}
		got := locatorError(in)
		var verr *domain.ValidationError
		if !errors.As(got, &verr) {
			t.Fatalf("got %T, want *domain.ValidationError", got)
		}
		if verr.Code != domain.ErrExperienceLocatorOutsideRoot {
			t.Fatalf("code = %q, want %q", verr.Code, domain.ErrExperienceLocatorOutsideRoot)
		}
	})
}
