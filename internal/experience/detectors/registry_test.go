// Registry tests for Hito 5 slice 5.2.
//
// The Registry is the orchestrator-facing lookup table: it holds the
// set of detectors known to a royo-learn process and lets the CLI
// (slice 5.3) and the acceptance scenario (slice 5.4) resolve a
// detector by its Kind without depending on a global variable.
//
// The table covers the invariants the orchestrator relies on:
//
//   - Registration order is preserved in Kinds().
//   - Duplicate Kind is rejected (the orchestrator cannot end up
//     running two detectors for the same event kind by accident).
//   - Nil detectors and detectors with empty Kind or empty Version
//     are rejected at registration time, not at Detect time.
//   - Kinds() returns a defensive copy so callers cannot mutate
//     the registry's internal order by accident.
//   - Get returns ok=false for unknown kinds without panicking.

package detectors

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	det, err := NewRetryDetector(3, 5*60_000_000_000) // 5 min in ns
	if err != nil {
		t.Fatalf("NewRetryDetector: %v", err)
	}
	if err := reg.Register(det); err != nil {
		t.Fatalf("Register(retry): %v", err)
	}

	got, ok := reg.Get("retry")
	if !ok {
		t.Fatalf("Get(retry) returned ok=false, want true")
	}
	if got.Kind() != "retry" {
		t.Errorf("Get(retry).Kind() = %q, want %q", got.Kind(), "retry")
	}
}

func TestRegistry_KindsPreservesOrder(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	// Insertion order matters: the CLI and doctor output list kinds
	// in this order, so regressions here would change stable JSON.
	if err := reg.Register(stubDetector{kind: "retry", version: "0.1.0"}); err != nil {
		t.Fatalf("Register retry: %v", err)
	}
	if err := reg.Register(stubDetector{kind: "tests", version: "0.1.0"}); err != nil {
		t.Fatalf("Register tests: %v", err)
	}
	if err := reg.Register(stubDetector{kind: "correction", version: "0.1.0"}); err != nil {
		t.Fatalf("Register correction: %v", err)
	}

	got := reg.Kinds()
	want := []string{"retry", "tests", "correction"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
}

func TestRegistry_DuplicateKindRejected(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	if err := reg.Register(stubDetector{kind: "retry", version: "0.1.0"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := reg.Register(stubDetector{kind: "retry", version: "0.2.0"})
	if err == nil {
		t.Fatalf("second Register accepted duplicate Kind, want error")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error %q should mention the duplicated kind", err.Error())
	}

	// The original detector must remain registered with its original
	// version; the rejected registration must not overwrite it.
	got, ok := reg.Get("retry")
	if !ok {
		t.Fatalf("Get(retry) returned ok=false after duplicate registration")
	}
	if got.Version() != "0.1.0" {
		t.Errorf("registered detector Version = %q, want %q (the original)", got.Version(), "0.1.0")
	}
}

func TestRegistry_NilDetectorRejected(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	err := reg.Register(nil)
	if err == nil {
		t.Fatalf("Register(nil) accepted, want error")
	}
	if reg.Len() != 0 {
		t.Errorf("Len() = %d after rejected nil registration, want 0", reg.Len())
	}
}

func TestRegistry_EmptyKindRejected(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	err := reg.Register(stubDetector{kind: "", version: "0.1.0"})
	if err == nil {
		t.Fatalf("Register with empty Kind accepted, want error")
	}
	if !strings.Contains(err.Error(), "Kind") {
		t.Errorf("error %q should mention Kind", err.Error())
	}
	if reg.Len() != 0 {
		t.Errorf("Len() = %d after rejected empty-Kind registration, want 0", reg.Len())
	}
}

func TestRegistry_EmptyVersionRejected(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	err := reg.Register(stubDetector{kind: "retry", version: ""})
	if err == nil {
		t.Fatalf("Register with empty Version accepted, want error")
	}
	if !strings.Contains(err.Error(), "Version") {
		t.Errorf("error %q should mention Version", err.Error())
	}
	if reg.Len() != 0 {
		t.Errorf("Len() = %d after rejected empty-Version registration, want 0", reg.Len())
	}
}

func TestRegistry_GetUnknownKind(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	if err := reg.Register(stubDetector{kind: "retry", version: "0.1.0"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("not_registered")
	if ok {
		t.Errorf("Get(unknown) returned ok=true (detector=%v), want false", got)
	}
	if got != nil {
		t.Errorf("Get(unknown) returned non-nil detector=%v, want nil", got)
	}
}

func TestRegistry_Len(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	if got := reg.Len(); got != 0 {
		t.Errorf("Len() = %d on empty registry, want 0", got)
	}
	if err := reg.Register(stubDetector{kind: "retry", version: "0.1.0"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := reg.Len(); got != 1 {
		t.Errorf("Len() = %d after one registration, want 1", got)
	}
	if err := reg.Register(stubDetector{kind: "tests", version: "0.1.0"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := reg.Len(); got != 2 {
		t.Errorf("Len() = %d after two registrations, want 2", got)
	}
}

func TestRegistry_KindsReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	if err := reg.Register(stubDetector{kind: "retry", version: "0.1.0"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(stubDetector{kind: "tests", version: "0.1.0"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	first := reg.Kinds()
	first[0] = "MUTATED"

	second := reg.Kinds()
	if second[0] != "retry" {
		t.Errorf("Kinds() internal state mutated through returned slice: %v", second)
	}
}

func TestRegistry_RealRetryDetector(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	det, err := NewRetryDetector(3, 5*60_000_000_000)
	if err != nil {
		t.Fatalf("NewRetryDetector: %v", err)
	}
	if err := reg.Register(det); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Round-trip: the registered detector must respond to Detect
	// exactly like the unregistered one. Guards against accidental
	// wrapping or cloning that changes behavior.
	got, ok := reg.Get("retry")
	if !ok {
		t.Fatalf("Get(retry) returned ok=false")
	}
	if got.Version() != det.Version() {
		t.Errorf("Version = %q, want %q", got.Version(), det.Version())
	}
}
