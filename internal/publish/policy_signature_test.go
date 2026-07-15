package publish

import "testing"

// TestPolicySignature_ChangesWithPolicyOutcome proves the preview hash is
// sensitive to the policy outcome: two plans with identical diffs but different
// policy verdicts hash differently, so an approval bound to one does not carry
// over to the other. This is the sixth invalidation condition of D11 §11.1
// ("the relevant policy changes").
func TestPolicySignature_ChangesWithPolicyOutcome(t *testing.T) {
	requiresApproval := []PolicyEvaluation{
		{PolicyName: "agents_rule_requires_human_approval", Passed: false},
		{PolicyName: "shared_scope_requires_human_approval", Passed: true},
	}
	autoApproved := []PolicyEvaluation{
		{PolicyName: "agents_rule_requires_human_approval", Passed: true},
		{PolicyName: "shared_scope_requires_human_approval", Passed: true},
	}

	sameDiff := "identical diff content"
	hashA := HashContent([]byte(sameDiff + "\x00policy:" + PolicySignature(requiresApproval)))
	hashB := HashContent([]byte(sameDiff + "\x00policy:" + PolicySignature(autoApproved)))

	if hashA == hashB {
		t.Fatal("identical diffs with different policy outcomes must produce different preview hashes")
	}
}

// TestPolicySignature_Deterministic proves the signature is order-independent so
// preview hashes are stable across evaluation ordering.
func TestPolicySignature_Deterministic(t *testing.T) {
	a := []PolicyEvaluation{
		{PolicyName: "b_policy", Passed: false},
		{PolicyName: "a_policy", Passed: true},
	}
	b := []PolicyEvaluation{
		{PolicyName: "a_policy", Passed: true},
		{PolicyName: "b_policy", Passed: false},
	}
	if PolicySignature(a) != PolicySignature(b) {
		t.Fatalf("signature must be order-independent: %q vs %q", PolicySignature(a), PolicySignature(b))
	}
}
