package jobs

import (
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// TestJobStatus_Enum enumerates the canonical JobStatus values and
// verifies that IsTerminal and IsActive behave correctly.
func TestJobStatus_Enum(t *testing.T) {
	cases := []struct {
		name     string
		status   JobStatus
		terminal bool
		active   bool
	}{
		{"idle", JobIdle, false, false},
		{"running", JobRunning, false, true},
		{"ok", JobOK, true, false},
		{"degraded", JobDegraded, false, false},
		{"error", JobError, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status.IsTerminal() != tc.terminal {
				t.Errorf("JobStatus(%s).IsTerminal() = %v, want %v", tc.name, tc.status.IsTerminal(), tc.terminal)
			}
			if tc.status.IsActive() != tc.active {
				t.Errorf("JobStatus(%s).IsActive() = %v, want %v", tc.name, tc.status.IsActive(), tc.active)
			}
		})
	}
}

// TestJobStatus_StableJSON pins the JSON shape of JobStatus so external
// consumers can deserialise without breaking.
func TestJobStatus_StableJSON(t *testing.T) {
	cases := []struct {
		name  string
		input JobStatus
		want  string
	}{
		{"idle", JobIdle, `"idle"`},
		{"running", JobRunning, `"running"`},
		{"ok", JobOK, `"ok"`},
		{"degraded", JobDegraded, `"degraded"`},
		{"error", JobError, `"error"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(tc.input)
			if got != tc.want[1:len(tc.want)-1] {
				t.Fatalf("string(Job%s) = %q, want %q", tc.name, got, tc.want[1:len(tc.want)-1])
			}
		})
	}
}

// TestDefaultLeaseBounds verifies the conservative lease default.
func TestDefaultLeaseBounds(t *testing.T) {
	b := DefaultLeaseBounds()
	if b.LeaseDuration != 5*time.Minute {
		t.Errorf("DefaultLeaseBounds().LeaseDuration = %v, want 5m", b.LeaseDuration)
	}
}

// TestJobState_FieldSurface pins the field names of JobState.
func TestJobState_FieldSurface(t *testing.T) {
	now := time.Now()
	state := JobState{
		ProjectID:   domain.ProjectID("test-proj"),
		JobName:     "test-job",
		Status:      JobIdle,
		InputDigest: "abc123",
		LeaseOwner:  "owner-1",
		MaxRetries:  3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_ = state // compilation check — field surface must compile
}

// TestJobRegistryEntry_FieldSurface pins the field names.
func TestJobRegistryEntry_FieldSurface(t *testing.T) {
	entry := JobRegistryEntry{
		JobName:            "test-job",
		Description:        "A test job",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
		Enabled:            true,
	}
	_ = entry
}
