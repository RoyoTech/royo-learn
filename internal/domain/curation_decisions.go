package domain

import "fmt"

// canonicalCurationDecisions is the single, authoritative allowlist of curation
// decisions (D11 §11.2). Both the CLI (parseCurateAction) and the MCP handler
// (curate_learning) validate against this list and nothing else, so the surface
// with the least human supervision (the agent over MCP) can never accept a
// decision that the surface with the most supervision (the human over CLI)
// rejects, or vice versa.
var canonicalCurationDecisions = []CurationDecision{
	CurationReject,
	CurationNeedsEvidence,
	CurationMerge,
	CurationApproveProjectKnowledge,
	CurationApproveSharedKnowledge,
	CurationApproveNewSkill,
	CurationApproveSkillUpdate,
	CurationApproveAgentsRule,
	CurationApproveTest,
}

// ValidCurationDecisions returns the canonical curation-decision allowlist.
// The returned slice is a copy; callers may not mutate the source of truth.
func ValidCurationDecisions() []CurationDecision {
	out := make([]CurationDecision, len(canonicalCurationDecisions))
	copy(out, canonicalCurationDecisions)
	return out
}

// IsValidCurationDecision reports whether d is a canonical curation decision.
func IsValidCurationDecision(d CurationDecision) bool {
	for _, valid := range canonicalCurationDecisions {
		if d == valid {
			return true
		}
	}
	return false
}

// ParseCurationDecision validates a raw wire value against the canonical
// allowlist and returns the decision, or a structured error naming every valid
// value. It is the shared gate used by both the CLI and the MCP server.
func ParseCurationDecision(raw string) (CurationDecision, error) {
	d := CurationDecision(raw)
	if IsValidCurationDecision(d) {
		return d, nil
	}
	return "", NewValidationError(ErrInvalidArgument,
		fmt.Sprintf("unknown curation decision %q: must be one of %s", raw, curationDecisionList()))
}

// curationDecisionList renders the allowlist for error messages.
func curationDecisionList() string {
	s := ""
	for i, d := range canonicalCurationDecisions {
		if i > 0 {
			s += ", "
		}
		s += string(d)
	}
	return s
}
