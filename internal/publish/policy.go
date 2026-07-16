package publish

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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

// PolicySignature renders the policy evaluation as a deterministic string so it
// can be folded into the preview hash. Binding the hash to the policy outcome
// means that a change to the relevant policy yields a different preview hash,
// which invalidates any approval bound to the old hash (D11 §11.1, the sixth
// invalidation condition). The signature is order-independent.
func PolicySignature(evaluations []PolicyEvaluation) string {
	parts := make([]string, 0, len(evaluations))
	for _, e := range evaluations {
		parts = append(parts, fmt.Sprintf("%s=%t", e.PolicyName, e.Passed))
	}
	sort.Strings(parts)
	return fmt.Sprintf("requires_approval=%t;%s",
		RequiresHumanApproval(evaluations), strings.Join(parts, ","))
}

// PlanSignature renders the per-destination plan as a deterministic, order-
// independent string so it can be folded into the preview hash. It binds the
// WHOLE plan — every destination's root, path, operation, prior file hash and
// posterior content hash — not merely the combined diff text (Recorrido D). A
// change to any field yields a different signature, which invalidates an
// approval bound to the old plan.
func PlanSignature(targets []domain.PublicationPlanTarget) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		parts = append(parts, fmt.Sprintf("root=%s;path=%s;op=%s;prior=%s;posterior=%s",
			t.Root, t.Path, t.Operation, t.PriorHash, t.PosteriorHash))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func previewHash(learningID domain.LearningID, targets []domain.PublicationPlanTarget, policySignature string) string {
	payload := fmt.Sprintf("learning=%s\npolicy=%s\n%s", learningID, policySignature, PlanSignature(targets))
	return HashContent([]byte(payload))
}

func requiresSensitiveApproval(targets []TargetResolution) bool {
	if len(targets) > 1 {
		return true
	}
	for _, target := range targets {
		if filepath.Clean(target.Path) == "AGENTS.md" || filepath.Clean(target.Root) == "shared" {
			return true
		}
		if target.Exists && strings.EqualFold(filepath.Base(target.Path), "SKILL.md") {
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
