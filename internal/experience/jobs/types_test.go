// Tests for the JobRegistryEntry type extensions added by Hito 11
// (PR #13): the Intent/Scope/RiskClass taxonomy fields and the
// Validate() helper. The deeper integration tests for the upsert path
// land in PR #14 (TestUpsertRegistryEntry_PopulatesThreeColumns); the
// tests here focus on the in-process validator only.

package jobs

import (
	"testing"

	"agent-royo-learn/internal/experience/semantic"
)

func TestJobRegistryEntry_Validate_AllKnown(t *testing.T) {
	e := JobRegistryEntry{
		JobName:   "experience_ingest:codex",
		Intent:    semantic.JobIntentIngest,
		Scope:     semantic.JobScopeProject,
		RiskClass: semantic.JobRiskClassLow,
	}
	if err := e.Validate(); err != nil {
		t.Errorf("Validate() with all known values = %v, want nil", err)
	}
}

func TestJobRegistryEntry_Validate_RejectsUnknown(t *testing.T) {
	cases := []struct {
		name      string
		intent    semantic.JobIntent
		scope     semantic.JobScope
		riskClass semantic.JobRiskClass
	}{
		{"unknown intent", "scrape", semantic.JobScopeProject, semantic.JobRiskClassLow},
		{"unknown scope", semantic.JobIntentIngest, "workspace", semantic.JobRiskClassLow},
		{"unknown risk", semantic.JobIntentIngest, semantic.JobScopeProject, "critical"},
		{"all empty", "", "", ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			e := JobRegistryEntry{
				JobName:   "experience_ingest:codex",
				Intent:    c.intent,
				Scope:     c.scope,
				RiskClass: c.riskClass,
			}
			if err := e.Validate(); err == nil {
				t.Errorf("Validate() with %s = nil, want error", c.name)
			}
		})
	}
}

func TestJobRegistryEntry_Validate_NilReceiver(t *testing.T) {
	var e *JobRegistryEntry
	if err := e.Validate(); err == nil {
		t.Error("Validate() on nil receiver = nil, want error")
	}
}
