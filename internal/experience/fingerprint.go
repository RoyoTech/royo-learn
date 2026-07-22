package experience

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

// FingerprintInput contains only normalized, already-redacted values. Callers
// must redact before constructing this value.
type FingerprintInput struct {
	Source          string
	ExternalSession string
	ExternalTurn    string
	Sequence        int64
	UserDigest      string
	AssistantDigest string
	ToolCallsDigest string
	FinishReason    string
	Complete        bool
	SourceRevision  string
}

// fingerprintInput is kept as an internal alias so package tests can exercise
// the pure function without creating a second representation.
type fingerprintInput = FingerprintInput

// Fingerprint returns a deterministic revision fingerprint. Its input is
// intentionally digest-based so raw transcript text cannot enter the hash
// payload after the service's redaction boundary.
func Fingerprint(input FingerprintInput) string {
	return fingerprint(input)
}

func fingerprint(input fingerprintInput) string {
	payload := struct {
		Source          string `json:"source"`
		ExternalSession string `json:"external_session_id"`
		ExternalTurn    string `json:"external_turn_id"`
		Sequence        int64  `json:"sequence"`
		UserDigest      string `json:"user_digest"`
		AssistantDigest string `json:"assistant_digest"`
		ToolCallsDigest string `json:"tool_calls_digest"`
		FinishReason    string `json:"finish_reason"`
		Complete        bool   `json:"complete"`
		SourceRevision  string `json:"source_revision"`
	}{
		Source:          input.Source,
		ExternalSession: input.ExternalSession,
		ExternalTurn:    input.ExternalTurn,
		Sequence:        input.Sequence,
		UserDigest:      input.UserDigest,
		AssistantDigest: input.AssistantDigest,
		ToolCallsDigest: input.ToolCallsDigest,
		FinishReason:    input.FinishReason,
		Complete:        input.Complete,
		SourceRevision:  input.SourceRevision,
	}
	encoded, _ := json.Marshal(payload)
	return digestBytes(encoded)
}

// digestString hashes a normalized, redacted string.
func digestString(value string) string {
	return digestBytes([]byte(value))
}

// DigestString hashes a normalized, redacted string for callers that need to
// compare a persisted digest without handling raw transcript content.
func DigestString(value string) string {
	return digestString(value)
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func revisionSeed(input fingerprintInput) string {
	// Keep the fallback source revision deterministic without making it circular
	// with the final fingerprint.
	return digestString(input.Source + "\x00" + input.ExternalSession + "\x00" +
		input.ExternalTurn + "\x00" + strconv.FormatInt(input.Sequence, 10) + "\x00" +
		input.UserDigest + "\x00" + input.AssistantDigest + "\x00" +
		input.ToolCallsDigest + "\x00" + input.FinishReason + "\x00" +
		strconv.FormatBool(input.Complete))
}
