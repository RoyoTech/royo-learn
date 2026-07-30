// Package semantic — tests for the taxonomy enum types. See types.go.

package semantic

import "testing"

func TestJobIntent_KnownValues(t *testing.T) {
	// Each documented value must pass IsValid and round-trip the literal.
	cases := []struct {
		got  JobIntent
		want string
	}{
		{JobIntentIngest, "ingest"},
		{JobIntentPromote, "promote"},
		{JobIntentRebuild, "rebuild"},
		{JobIntentCleanup, "cleanup"},
		{JobIntentDrift, "drift"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("JobIntent literal = %q, want %q", string(c.got), c.want)
		}
		if !c.got.IsValid() {
			t.Errorf("JobIntent(%q).IsValid() = false, want true", c.want)
		}
		if !IsValidIntent(c.got) {
			t.Errorf("IsValidIntent(%q) = false, want true", c.want)
		}
	}
}

// TestJobIntent_DriftAccepted asserts that the Hito 12 JobIntentDrift
// constant (introduced for the publication drift checker) is accepted by
// IsValidIntent. Without the switch extension, the repository upsert path
// at internal/experience/jobs/repository.go rejects the new value with a
// ErrInvalidArgument at write time.
func TestJobIntent_DriftAccepted(t *testing.T) {
	if string(JobIntentDrift) != "drift" {
		t.Errorf("JobIntentDrift literal = %q, want %q", string(JobIntentDrift), "drift")
	}
	if !JobIntentDrift.IsValid() {
		t.Errorf("JobIntentDrift.IsValid() = false, want true")
	}
	if !IsValidIntent(JobIntentDrift) {
		t.Errorf("IsValidIntent(JobIntentDrift) = false, want true")
	}
}

func TestJobIntent_UnknownRejected(t *testing.T) {
	// The values the migration column defaults cannot change silently.
	cases := []JobIntent{
		"",
		"Ingest",            // case sensitive
		"unknown_intent",    // arbitrary unknown
		"scrape",            // explicitly rejected at upsert time
		"ingest ",           // trailing whitespace
		" ingest",           // leading whitespace
		"ingest;drop table", // SQL-injection-shaped
	}
	for _, c := range cases {
		if IsValidIntent(c) {
			t.Errorf("IsValidIntent(%q) = true, want false", string(c))
		}
		if c.IsValid() {
			t.Errorf("JobIntent(%q).IsValid() = true, want false", string(c))
		}
	}
}

func TestJobIntent_IsValid(t *testing.T) {
	// The two halves of the IsValid contract must agree for every input.
	for _, v := range []JobIntent{
		JobIntentIngest, JobIntentPromote, JobIntentRebuild, JobIntentCleanup,
		"", "garbage",
	} {
		if v.IsValid() != IsValidIntent(v) {
			t.Errorf("JobIntent(%q).IsValid()=%v disagrees with IsValidIntent=%v",
				string(v), v.IsValid(), IsValidIntent(v))
		}
	}
}

func TestJobScope_KnownValues(t *testing.T) {
	cases := []struct {
		got  JobScope
		want string
	}{
		{JobScopeProject, "project"},
		{JobScopeGlobal, "global"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("JobScope literal = %q, want %q", string(c.got), c.want)
		}
		if !c.got.IsValid() {
			t.Errorf("JobScope(%q).IsValid() = false, want true", c.want)
		}
		if !IsValidScope(c.got) {
			t.Errorf("IsValidScope(%q) = false, want true", c.want)
		}
	}
}

func TestJobScope_UnknownRejected(t *testing.T) {
	cases := []JobScope{
		"",
		"Project",
		"workspace",
		"all",
		"project;",
	}
	for _, c := range cases {
		if IsValidScope(c) {
			t.Errorf("IsValidScope(%q) = true, want false", string(c))
		}
		if c.IsValid() {
			t.Errorf("JobScope(%q).IsValid() = true, want false", string(c))
		}
	}
}

func TestJobScope_IsValid(t *testing.T) {
	for _, v := range []JobScope{
		JobScopeProject, JobScopeGlobal,
		"", "garbage",
	} {
		if v.IsValid() != IsValidScope(v) {
			t.Errorf("JobScope(%q).IsValid()=%v disagrees with IsValidScope=%v",
				string(v), v.IsValid(), IsValidScope(v))
		}
	}
}

func TestJobRiskClass_KnownValues(t *testing.T) {
	cases := []struct {
		got  JobRiskClass
		want string
	}{
		{JobRiskClassLow, "low"},
		{JobRiskClassMedium, "medium"},
		{JobRiskClassHigh, "high"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("JobRiskClass literal = %q, want %q", string(c.got), c.want)
		}
		if !c.got.IsValid() {
			t.Errorf("JobRiskClass(%q).IsValid() = false, want true", c.want)
		}
		if !IsValidRiskClass(c.got) {
			t.Errorf("IsValidRiskClass(%q) = false, want true", c.want)
		}
	}
}

func TestJobRiskClass_UnknownRejected(t *testing.T) {
	cases := []JobRiskClass{
		"",
		"Low",
		"critical",
		"none",
		"low;high",
	}
	for _, c := range cases {
		if IsValidRiskClass(c) {
			t.Errorf("IsValidRiskClass(%q) = true, want false", string(c))
		}
		if c.IsValid() {
			t.Errorf("JobRiskClass(%q).IsValid() = true, want false", string(c))
		}
	}
}

func TestJobRiskClass_IsValid(t *testing.T) {
	for _, v := range []JobRiskClass{
		JobRiskClassLow, JobRiskClassMedium, JobRiskClassHigh,
		"", "garbage",
	} {
		if v.IsValid() != IsValidRiskClass(v) {
			t.Errorf("JobRiskClass(%q).IsValid()=%v disagrees with IsValidRiskClass=%v",
				string(v), v.IsValid(), IsValidRiskClass(v))
		}
	}
}
