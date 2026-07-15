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

// policyPreferenceTypeRequiresHuman: preference-type learnings routed to shared
// scope or AGENTS.md always require explicit human approval (D11 §11.3).
func policyPreferenceTypeRequiresHuman(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	if learning.Type == domain.TypePreference {
		dest := curation.Destination
		if dest != nil && (dest.Type == domain.DestShared || dest.Type == domain.DestAgentsRule) {
			return PolicyEvaluation{
				PolicyName: "preference_shared_requires_human_approval",
				Passed:     false,
				Reason:     "preference type learning cannot be auto-published as shared/agents rule without explicit human approval",
			}
		}
	}
	return PolicyEvaluation{
		PolicyName: "preference_shared_requires_human_approval",
		Passed:     true,
		Reason:     "either not a preference type or not a shared/agents destination",
	}
}

// policySharedScopeRequiresApproval: publishing to shared scope always requires
// explicit human approval (D4, D11 §11.4).
//
// The gate is the EFFECTIVE destination, never the curation decision that
// derived it. Gating on the decision is a tautology — curate.deriveDestination
// only ever yields DestShared for approve_shared_knowledge — so its failure
// branch was unreachable and shared knowledge published with no approval. The
// real enforcement of the approval record happens at publish time via
// Service.CheckApproval, which this policy's requires-approval verdict triggers.
func policySharedScopeRequiresApproval(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	dest := curation.Destination
	if dest != nil && dest.Type == domain.DestShared {
		return PolicyEvaluation{
			PolicyName: "shared_scope_requires_human_approval",
			Passed:     false,
			Reason:     "shared-scope publications always require explicit human approval bound to the preview",
		}
	}
	return PolicyEvaluation{
		PolicyName: "shared_scope_requires_human_approval",
		Passed:     true,
		Reason:     "not a shared destination",
	}
}

// policyAgentsRuleRequiresApproval: modifying AGENTS.md always requires explicit
// human approval (D4, D11 §11.4). See policySharedScopeRequiresApproval for why
// the gate is the effective destination and not the curation decision.
func policyAgentsRuleRequiresApproval(learning *domain.Learning, curation *domain.Curation) PolicyEvaluation {
	dest := curation.Destination
	if dest != nil && dest.Type == domain.DestAgentsRule {
		return PolicyEvaluation{
			PolicyName: "agents_rule_requires_human_approval",
			Passed:     false,
			Reason:     "AGENTS.md publications always require explicit human approval bound to the preview",
		}
	}
	return PolicyEvaluation{
		PolicyName: "agents_rule_requires_human_approval",
		Passed:     true,
		Reason:     "not an AGENTS.md destination",
	}
}
