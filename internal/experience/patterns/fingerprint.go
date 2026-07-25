// Deterministic pattern fingerprint and retrieval-term normalizer for
// Hito 6 slice 6.1.
//
// The pattern fingerprint extends the per-event fingerprint
// (internal/experience/detectors/persist.go:EventFingerprint) into the
// pattern-level identity the clustering algorithm uses. The contract
// follows docs/23-PATTERN-MINING.md §3:
//
//   - kind, problem tokens, tool, result, retrieval terms;
//   - NO timestamps, UUIDs, ports, hashes, absolute paths or session
//     IDs (the "eliminate" list).
//
// Retrieval-term normalization prepares the terms that flow into the
// fingerprint and into the Jaccard clustering of slice 6.2. It
// lowercases, trims, sorts, deduplicates, strips UUIDs / ports /
// hashes / absolute paths / redacted markers and collapses empty
// tokens. The output is the canonical form the fingerprint and the
// Jaccard comparison share.

package patterns

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// volatileValuePattern matches the "eliminate" categories listed in
// docs/23 §3:
//
//   - UUIDs (8-4-4-4-12 with hex)
//   - sha-256 / sha-1 / md5 / generic hex hashes
//   - ports
//   - absolute POSIX or Windows paths
//   - redacted markers
//
// Compiled once at package init so the normalizer stays cheap.
var volatileValuePattern = regexp.MustCompile(
	`(?i)` +
		`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}` + // uuid
		`|` + `(?:^|[\s=:_-])sha-?[0-9a-z]*[0-9a-f]{8,}` + // sha prefix + hex
		`|` + `\b[0-9a-f]{32,}\b` + // bare hex hash (md5/sha1/sha256)
		`|` + `\bport[ :=-]?[0-9]{2,5}\b` + // ports
		`|` + `[a-z]:[/\\][^\s]*` + // windows absolute paths
		`|` + `(?:^|[\s])/[a-z0-9_./-]+` + // posix absolute paths
		`|` + `\[REDACTED:[a-z]+\]` + // redacted markers
		`|` + `<private>[^<]*</private>` + // private markers
		`|` + `(?i)password\s*=\s*\S+`,
)

// whitespaceSplit collapses all whitespace (including newlines) into
// a single space so terms like "secret\nvalue" or "port\n8080" are
// split before the volatile-value check runs.
var whitespaceSplit = regexp.MustCompile(`\s+`)

// sensitiveKeywordPattern catches tokens that would leak the
// upstream redaction marker names back into the retrieval set.
// Real redaction has already happened at the ingestion boundary
// (docs/22-ADAPTER-CONTRACT.md §4); this is defense in depth so a
// misconfigured adapter cannot smuggle "secret", "password" etc.
// into a downstream pattern fingerprint.
var sensitiveKeywordPattern = regexp.MustCompile(
	`(?i)\b(secret|password|passwd|token|credential|apikey|api[-_]?key|access[-_]?key)\b`,
)

// PatternInput is the canonical, already-redacted input the
// fingerprint hashes. Volatile fields (Timestamps, UUIDs, AbsolutePaths,
// SessionIDs, Hashes) are accepted so callers do not have to pre-clean
// the input; the fingerprint function strips them.
type PatternInput struct {
	// Kind is the canonical ExperienceEventKind, lower_snake_case.
	Kind string `json:"kind"`

	// ProblemTokens are normalized tokens from the event problem
	// (deduplicated, lowercased, sorted).
	ProblemTokens []string `json:"problem_tokens"`

	// Tool is the primary tool/command without volatile values.
	Tool string `json:"tool"`

	// Result is the outcome kind: fail, success, corrected, fallback.
	Result string `json:"result"`

	// RetrievalTerms are normalized terms for downstream lexical
	// retrieval.
	RetrievalTerms []string `json:"retrieval_terms"`

	// Volatile fields. The fingerprint strips them; they exist on the
	// input so callers do not have to pre-clean the event payload.
	Timestamps    []string `json:"-"`
	UUIDs         []string `json:"-"`
	AbsolutePaths []string `json:"-"`
	SessionIDs    []string `json:"-"`
	Hashes        []string `json:"-"`
}

// NormalizeRetrievalTerms returns the canonical form of a list of
// retrieval terms: lowercase, trimmed, sorted, deduplicated, free of
// volatile values (UUIDs, ports, hashes, absolute paths, redacted
// markers). The output is the canonical set used by the fingerprint
// and the Jaccard comparison.
//
// Each input entry is first split on whitespace (so embedded newlines
// or tabs do not smuggle volatile fragments past the regex), then
// each sub-token is filtered independently.
//
// A nil or empty input returns an empty slice (never nil, never error).
// Callers should treat the slice as read-only.
func NormalizeRetrievalTerms(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		parts := whitespaceSplit.Split(raw, -1)
		for _, part := range parts {
			clean := strings.ToLower(strings.TrimSpace(part))
			if clean == "" {
				continue
			}
			if volatileValuePattern.MatchString(clean) {
				continue
			}
			if sensitiveKeywordPattern.MatchString(clean) {
				continue
			}
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out
}

// NormalizeProblemTokens is the lighter-weight variant used for the
// fingerprint input. It keeps alphanumerics + dash + dot + colon so
// structured tokens like "exit.code:1" survive.
func NormalizeProblemTokens(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		parts := whitespaceSplit.Split(raw, -1)
		for _, part := range parts {
			clean := strings.ToLower(strings.TrimSpace(part))
			if clean == "" {
				continue
			}
			if volatileValuePattern.MatchString(clean) {
				continue
			}
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out
}

// PatternFingerprint returns a stable sha256 fingerprint for the
// canonical pattern input. Same canonical inputs produce the same
// fingerprint across runs; volatile fields are stripped before
// hashing. The output is 64 lowercase hex characters.
//
// The fingerprint is built from a sorted JSON encoding of:
//
//	{
//	  "kind":            <kind>,
//	  "tool":            <tool>,
//	  "result":          <result>,
//	  "problem_tokens":  [...sorted...],
//	  "retrieval_terms": [...sorted...]
//	}
//
// JSON is used (instead of raw concatenation) so that future schema
// additions remain backwards-compatible: an old fingerprint is
// unchanged when a new field is added and unset.
func PatternFingerprint(in PatternInput) string {
	problemTokens := NormalizeProblemTokens(in.ProblemTokens)
	retrievalTerms := NormalizeRetrievalTerms(in.RetrievalTerms)
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	tool := normalizeScalar(in.Tool)
	result := normalizeScalar(in.Result)

	// We do not call json.Marshal here because the field set is
	// fully controlled; concatenation with deterministic ordering
	// (already sorted) avoids reflection overhead in the hot path.
	var b strings.Builder
	b.WriteString("k=")
	b.WriteString(kind)
	b.WriteByte(0)
	b.WriteString("t=")
	b.WriteString(tool)
	b.WriteByte(0)
	b.WriteString("r=")
	b.WriteString(result)
	b.WriteByte(0)
	b.WriteString("p=")
	for _, t := range problemTokens {
		b.WriteString(t)
		b.WriteByte(0x1f) // unit separator: avoids accidental joins
	}
	b.WriteByte(0)
	b.WriteString("s=")
	for _, t := range retrievalTerms {
		b.WriteString(t)
		b.WriteByte(0x1f)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// normalizeScalar removes volatile and sensitive command values before they
// participate in a fingerprint. Stable command words remain in input order.
func normalizeScalar(value string) string {
	parts := whitespaceSplit.Split(strings.ToLower(strings.TrimSpace(value)), -1)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || isAbsolutePathToken(part) || volatileValuePattern.MatchString(part) || sensitiveKeywordPattern.MatchString(part) {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, " ")
}

func isAbsolutePathToken(value string) bool {
	if strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 3 && value[1] == 58 && (value[2] == 47 || value[2] == 92)
}
