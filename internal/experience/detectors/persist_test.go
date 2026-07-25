// Persistence tests for Hito 5 slice 5.4.
//
// EventFingerprint is a pure function over a CandidateEvent (see
// its table above). The full ingest path is exercised by the CLI
// acceptance test in cmd/royo-learn/experience_detect_test.go
// (TestRunExperienceDetect_PersistEndToEnd), which writes a real
// SQLite database via resolvePublishContext and verifies the
// detector event lands in experience_events with the expected
// fingerprint and idempotency.
//
// BuildDetectorEnvelope is the pure mapper extracted from Persist
// so it can be unit-tested without the experience.Service
// machinery. The CLI acceptance test covers the ingest path end to
// end.

package detectors

import (
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestEventFingerprint_Deterministic(t *testing.T) {
	t.Parallel()
	ev := CandidateEvent{
		Kind:           "retry",
		Problem:        "same fingerprint failed 3 times",
		Tool:           "npm test",
		Result:         "corrected",
		Paths:          []string{"a", "b"},
		RetrievalTerms: []string{"retry"},
		Extra:          map[string]any{"count": 3},
	}

	fp1 := EventFingerprint(ev)
	fp2 := EventFingerprint(ev)
	if fp1 != fp2 {
		t.Errorf("EventFingerprint non-deterministic: %q vs %q", fp1, fp2)
	}
	if len(fp1) != 64 {
		t.Errorf("EventFingerprint length = %d, want 64 (sha256 hex)", len(fp1))
	}
}

func TestEventFingerprint_OrderIndependent(t *testing.T) {
	t.Parallel()
	a := CandidateEvent{
		Kind:           "retry",
		Paths:          []string{"a", "b"},
		RetrievalTerms: []string{"retry", "fp-x"},
		Extra:          map[string]any{"x": 1, "y": 2},
	}
	b := CandidateEvent{
		Kind:           "retry",
		Paths:          []string{"b", "a"},
		RetrievalTerms: []string{"fp-x", "retry"},
		Extra:          map[string]any{"y": 2, "x": 1},
	}
	if EventFingerprint(a) != EventFingerprint(b) {
		t.Errorf("EventFingerprint depends on map/slice order:\n  a=%q\n  b=%q",
			EventFingerprint(a), EventFingerprint(b))
	}
}

func TestEventFingerprint_DistinctEvents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b CandidateEvent
	}{
		{
			name: "different Kind",
			a:    CandidateEvent{Kind: "retry"},
			b:    CandidateEvent{Kind: "tests"},
		},
		{
			name: "different Tool",
			a:    CandidateEvent{Kind: "retry", Tool: "npm test"},
			b:    CandidateEvent{Kind: "retry", Tool: "cargo test"},
		},
		{
			name: "different Result",
			a:    CandidateEvent{Kind: "retry", Result: "fail"},
			b:    CandidateEvent{Kind: "retry", Result: "corrected"},
		},
		{
			name: "different Problem",
			a:    CandidateEvent{Kind: "retry", Problem: "fail A"},
			b:    CandidateEvent{Kind: "retry", Problem: "fail B"},
		},
		{
			name: "different Paths",
			a:    CandidateEvent{Kind: "retry", Paths: []string{"a"}},
			b:    CandidateEvent{Kind: "retry", Paths: []string{"b"}},
		},
		{
			name: "different Extra",
			a:    CandidateEvent{Kind: "retry", Extra: map[string]any{"x": 1}},
			b:    CandidateEvent{Kind: "retry", Extra: map[string]any{"x": 2}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if EventFingerprint(tc.a) == EventFingerprint(tc.b) {
				t.Errorf("EventFingerprint collision:\n  a=%+v\n  b=%+v", tc.a, tc.b)
			}
		})
	}
}

func TestEventFingerprint_EmptyEvent(t *testing.T) {
	t.Parallel()
	fp := EventFingerprint(CandidateEvent{})
	if len(fp) != 64 {
		t.Errorf("EventFingerprint(empty) length = %d, want 64", len(fp))
	}
	if EventFingerprint(CandidateEvent{}) != EventFingerprint(CandidateEvent{}) {
		t.Errorf("EventFingerprint(empty) is not stable")
	}
}

func TestEventFingerprint_Sha256HexFormat(t *testing.T) {
	t.Parallel()
	fp := EventFingerprint(CandidateEvent{Kind: "retry"})
	if len(fp) != 64 {
		t.Fatalf("length = %d, want 64", len(fp))
	}
	for _, r := range fp {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("non-hex character %q in fingerprint %q", r, fp)
		}
	}
}

func TestBuildDetectorEnvelope_RejectsEmptyProjectID(t *testing.T) {
	t.Parallel()
	_, err := BuildDetectorEnvelope("", "/tmp/proj", CandidateEvent{Kind: "retry"}, time.Now())
	if err == nil {
		t.Fatalf("BuildDetectorEnvelope accepted empty projectID")
	}
	if !strings.Contains(err.Error(), "project id") {
		t.Errorf("error %q should mention project id", err.Error())
	}
}

func TestBuildDetectorEnvelope_RejectsEmptyProjectRoot(t *testing.T) {
	t.Parallel()
	_, err := BuildDetectorEnvelope("proj-1", "", CandidateEvent{Kind: "retry"}, time.Now())
	if err == nil {
		t.Fatalf("BuildDetectorEnvelope accepted empty projectRoot")
	}
	if !strings.Contains(err.Error(), "project root") {
		t.Errorf("error %q should mention project root", err.Error())
	}
}

func TestBuildDetectorEnvelope_RejectsEmptyKind(t *testing.T) {
	t.Parallel()
	_, err := BuildDetectorEnvelope("proj-1", "/tmp/proj", CandidateEvent{}, time.Now())
	if err == nil {
		t.Fatalf("BuildDetectorEnvelope accepted empty Kind")
	}
	if !strings.Contains(err.Error(), "Kind") {
		t.Errorf("error %q should mention Kind", err.Error())
	}
}

func TestBuildDetectorEnvelope_ShapeIsStable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	ev := CandidateEvent{
		Kind:           "retry",
		Problem:        "fingerprint failed 3 times",
		Tool:           "npm test",
		Result:         "corrected",
		Paths:          []string{"a", "b"},
		RetrievalTerms: []string{"retry", "fp-x"},
		Extra:          map[string]any{"count": 3},
	}

	envelope, err := BuildDetectorEnvelope("proj-1", "/tmp/proj", ev, now)
	if err != nil {
		t.Fatalf("BuildDetectorEnvelope: %v", err)
	}
	if envelope.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", envelope.SchemaVersion)
	}
	if envelope.Source != domain.SourceDetector {
		t.Errorf("Source = %q, want %q", envelope.Source, domain.SourceDetector)
	}
	if envelope.ProjectRoot != "/tmp/proj" {
		t.Errorf("ProjectRoot = %q, want %q", envelope.ProjectRoot, "/tmp/proj")
	}
	if envelope.Session.ExternalID != "detector:retry" {
		t.Errorf("Session.ExternalID = %q, want %q", envelope.Session.ExternalID, "detector:retry")
	}
	if envelope.Session.Locator.Kind != "detector" {
		t.Errorf("Session.Locator.Kind = %q, want %q", envelope.Session.Locator.Kind, "detector")
	}
	if envelope.Session.Locator.Path != "/tmp/proj" {
		t.Errorf("Session.Locator.Path = %q, want %q", envelope.Session.Locator.Path, "/tmp/proj")
	}
	if envelope.Turn.ExternalID != EventFingerprint(ev) {
		t.Errorf("Turn.ExternalID = %q, want the EventFingerprint", envelope.Turn.ExternalID)
	}
	if !envelope.Turn.Complete {
		t.Error("Turn.Complete = false, want true (the detector event is fully described)")
	}
	if envelope.Turn.OccurredAt != now {
		t.Errorf("Turn.OccurredAt = %v, want %v", envelope.Turn.OccurredAt, now)
	}
	if envelope.Turn.UserText != ev.Problem {
		t.Errorf("Turn.UserText = %q, want %q", envelope.Turn.UserText, ev.Problem)
	}
	if len(envelope.Turn.ToolCalls) != 1 {
		t.Fatalf("Turn.ToolCalls has %d entries, want 1", len(envelope.Turn.ToolCalls))
	}
	tc := envelope.Turn.ToolCalls[0]
	if tc.Name != ev.Tool {
		t.Errorf("ToolCalls[0].Name = %q, want %q", tc.Name, ev.Tool)
	}
	if tc.Outcome != ev.Result {
		t.Errorf("ToolCalls[0].Outcome = %q, want %q", tc.Outcome, ev.Result)
	}
	if envelope.Actor.Kind != "system" {
		t.Errorf("Actor.Kind = %q, want %q", envelope.Actor.Kind, "system")
	}
	if envelope.Actor.Name != "detector:retry" {
		t.Errorf("Actor.Name = %q, want %q", envelope.Actor.Name, "detector:retry")
	}
}

func TestBuildDetectorEnvelope_DeterministicTurnID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	ev := CandidateEvent{
		Kind:    "retry",
		Tool:    "npm test",
		Problem: "same fingerprint failed 3 times",
		Result:  "corrected",
		Paths:   []string{"b", "a"},             // intentionally unsorted
		Extra:   map[string]any{"y": 2, "x": 1}, // intentionally unsorted keys
	}

	first, err := BuildDetectorEnvelope("proj-1", "/tmp/proj", ev, now)
	if err != nil {
		t.Fatalf("first BuildDetectorEnvelope: %v", err)
	}
	second, err := BuildDetectorEnvelope("proj-1", "/tmp/proj", ev, now)
	if err != nil {
		t.Fatalf("second BuildDetectorEnvelope: %v", err)
	}
	if first.Turn.ExternalID != second.Turn.ExternalID {
		t.Errorf("Turn.ExternalID differs across calls:\n  first=%q\n  second=%q",
			first.Turn.ExternalID, second.Turn.ExternalID)
	}
	if first.Turn.SourceRevision != second.Turn.SourceRevision {
		t.Errorf("Turn.SourceRevision differs across calls")
	}
	if first.Session.Locator.SourceHash != second.Session.Locator.SourceHash {
		t.Errorf("Locator.SourceHash differs across calls")
	}
}

func TestBuildDetectorEnvelope_DefaultTimestampWhenZero(t *testing.T) {
	t.Parallel()
	ev := CandidateEvent{Kind: "retry", Tool: "npm test", Result: "corrected", Problem: "x"}

	envelope, err := BuildDetectorEnvelope("proj-1", "/tmp/proj", ev, time.Time{})
	if err != nil {
		t.Fatalf("BuildDetectorEnvelope: %v", err)
	}
	if envelope.Turn.OccurredAt.IsZero() {
		t.Error("Turn.OccurredAt is zero after BuildDetectorEnvelope with zero now")
	}
	if envelope.Session.UpdatedAt.IsZero() {
		t.Error("Session.UpdatedAt is zero after BuildDetectorEnvelope with zero now")
	}
}
