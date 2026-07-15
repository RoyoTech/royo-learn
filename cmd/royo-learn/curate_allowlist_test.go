package main

import (
	"testing"

	"agent-royo-learn/internal/domain"
)

// TestContract_CLIAndDomainShareCurationAllowlist proves the CLI accepts exactly
// the canonical curation decisions defined once in internal/domain and rejects
// everything else — the same allowlist the MCP handler validates against
// (D11 §11.2). A divergence between the two surfaces breaks the build.
func TestContract_CLIAndDomainShareCurationAllowlist(t *testing.T) {
	// Every canonical decision is accepted by the CLI and maps to itself.
	for _, decision := range domain.ValidCurationDecisions() {
		got, err := parseCurateAction(string(decision))
		if err != nil {
			t.Errorf("CLI rejects canonical decision %q: %v", decision, err)
			continue
		}
		if got != decision {
			t.Errorf("CLI maps %q to %q, want identity", decision, got)
		}
		// The MCP surface must accept exactly the same value.
		if _, mcpErr := domain.ParseCurationDecision(string(decision)); mcpErr != nil {
			t.Errorf("MCP rejects canonical decision %q that the CLI accepts: %v", decision, mcpErr)
		}
	}

	// The historical shortcut is a deprecated CLI-only alias.
	if got, err := parseCurateAction("approve"); err != nil || got != domain.CurationApproveProjectKnowledge {
		t.Errorf("CLI alias \"approve\" = (%q, %v), want approve_project_knowledge", got, err)
	}

	// Unknown decisions are rejected identically on both surfaces.
	for _, bogus := range []string{"", "approve_everything", "yolo", "APPROVE"} {
		if _, err := parseCurateAction(bogus); err == nil {
			t.Errorf("CLI accepted invalid decision %q", bogus)
		}
		if _, err := domain.ParseCurationDecision(bogus); err == nil {
			t.Errorf("MCP accepted invalid decision %q", bogus)
		}
	}
}
