package experience

import "testing"

func TestFingerprintIsDeterministicAndRevisionSensitive(t *testing.T) {
	input := fingerprintInput{
		Source:          "opencode",
		ExternalSession: "session-1",
		ExternalTurn:    "turn-1",
		Sequence:        1,
		UserDigest:      "user-digest",
		AssistantDigest: "assistant-digest",
		ToolCallsDigest: "tool-digest",
		FinishReason:    "stop",
		Complete:        true,
		SourceRevision:  "revision-1",
	}

	first := fingerprint(input)
	second := fingerprint(input)
	if first == "" || first != second {
		t.Fatalf("fingerprint is not deterministic: %q vs %q", first, second)
	}

	input.SourceRevision = "revision-2"
	if got := fingerprint(input); got == first {
		t.Fatal("fingerprint did not change for a new source revision")
	}
}

func TestDigestStringUsesTheSuppliedRedactedValue(t *testing.T) {
	redacted := "token [REDACTED:known]"
	if got, want := DigestString(redacted), digestString(redacted); got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
}

func TestFingerprintAndRevisionSeedPublicAndFallbackPaths(t *testing.T) {
	input := FingerprintInput{Source: "manual", ExternalSession: "session", ExternalTurn: "turn", Complete: true}
	if Fingerprint(input) == "" {
		t.Fatal("Fingerprint returned empty hash")
	}
	if revisionSeed(input) == "" {
		t.Fatal("revisionSeed returned empty hash")
	}
}
