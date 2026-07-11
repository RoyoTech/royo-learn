package publish

import (
	"agent-royo-learn/internal/domain"
)

// EvaluatePolicies checks all applicable publish policies against a learning
// and destination. Returns a list of policy evaluation results.
func EvaluatePolicies(learning *domain.Learning, curation *domain.Curation) []PolicyEvaluation {
	var evaluations []PolicyEvaluation

	evaluations = append(evaluations, policyPreferenceTypeRequiresHuman(learning, curation))
	evaluations = append(evaluations, policySharedScopeRequiresApproval(learning, curation))
	evaluations = append(evaluations, policyAgentsRuleRequiresApproval(learning, curation))

	return evaluations
}

// RequiresHumanApproval returns true if ANY policy requires explicit human approval.
func RequiresHumanApproval(evaluations []PolicyEvaluation) bool {
	for _, e := range evaluations {
		if !e.Passed {
			return true
		}
	}
	return false
}

// policyPreferenceTypeRequiresHuman: preference-type learnings cannot be
// auto-published as shared rules without explicit user decision.
func policyPreferenceTypeRequiresHuman(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	if learning.Type == domain.TypePreference {
		dest := curation.Destination
		if dest != nil && (dest.Type == domain.DestShared || dest.Type == domain.DestAgentsRule) {
			return PolicyEvaluation{
				PolicyName: "preference_shared_requires_human",
				Passed:     false,
				Reason:     "preference type learning cannot be auto-published as shared/agents rule without explicit human approval",
			}
		}
	}
	return PolicyEvaluation{
		PolicyName: "preference_shared_requires_human",
		Passed:     true,
		Reason:     "either not a preference type or not a shared/agents destination",
	}
}

// policySharedScopeRequiresApproval: shared scope publications require a
// human-approved curation decision.
func policySharedScopeRequiresApproval(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	dest := curation.Destination
	if dest != nil && dest.Type == domain.DestShared {
		// shared destinations must have been explicitly approved via curation.
		if curation.Decision != domain.CurationApproveSharedKnowledge {
			return PolicyEvaluation{
				PolicyName: "shared_requires_curation_approval",
				Passed:     false,
				Reason:     "shared destination requires curation decision 'approve_shared_knowledge'",
			}
		}
	}
	return PolicyEvaluation{
		PolicyName: "shared_requires_curation_approval",
		Passed:     true,
		Reason:     "not a shared destination or properly approved",
	}
}

// policyAgentsRuleRequiresApproval: AGENTS.md modifications require explicit
// human approval.
func policyAgentsRuleRequiresApproval(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	dest := curation.Destination
	if dest != nil && dest.Type == domain.DestAgentsRule {
		if curation.Decision != domain.CurationApproveAgentsRule {
			return PolicyEvaluation{
				PolicyName: "agents_rule_requires_approval",
				Passed:     false,
				Reason:     "AGENTS.md modification requires curation decision 'approve_agents_rule'",
			}
		}
	}
	return PolicyEvaluation{
		PolicyName: "agents_rule_requires_approval",
		Passed:     true,
		Reason:     "not an AGENTS.md destination or properly approved",
	}
}
