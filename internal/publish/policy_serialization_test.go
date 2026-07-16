package publish

import (
	"encoding/json"
	"testing"
)

func TestPolicyEvaluationPreservesPublicJSONKeys(t *testing.T) {
	data, err := json.Marshal(PolicyEvaluation{PolicyName: "policy", Passed: true, Reason: "reason"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"PolicyName":"policy","Passed":true,"Reason":"reason"}` {
		t.Fatalf("policy JSON = %s", data)
	}
}
