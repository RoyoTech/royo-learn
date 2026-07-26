// Promotion-side redaction pipeline. Slice 7.1 of Hito 7.
//
// Promotion bridges a qualified ExperiencePattern (Hito 6) into a
// persistent Learning via capture.Service (Hito 1). Before that handoff
// the promotion code must redact every free-text field it derived from
// the pattern and produce a deterministic preview fingerprint of the
// redacted bag; the audit row stores that fingerprint so reviewers can
// reproduce what Promotion saw byte-for-byte.
//
// Why a separate pipeline (and not just evidence.RedactLearning)?
//
//   - RedactLearning mutates a *domain.Learning in place and depends on
//     a fully constructed Learning. Promotion extracts its fields from a
//     pattern BEFORE building the Learning; mutating a non-existent
//     Learning is not an option.
//   - The "what Promotion saw" hash must reflect redacted content; the
//     normalized hash capture.Service computes later is a separate
//     audit-row field used for dedup. Defence in depth, single source
//     of truth per purpose.
//   - The report (which fields were scrubbed) is a structured value the
//     audit row carries, not just a boolean. Promotion needs the field
//     list to populate promotion.RedactionSummary accurately.
//
// The package never imports internal/experience/patterns; PromotionFields
// is the structural contract between Promotion and the evidence layer.
// This keeps evidence stackable under promotion without creating a
// cycle.

package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// PromotionFields is the structured bag of fields Promotion extracts
// from a pattern and hands to capture.Service.Capture. The values are
// the pre-redaction inputs: callers MUST feed them through
// RedactPromotionFields before hashing or persisting.
//
// Field names match the canonical snake_case identifiers the audit row
// uses for the "redacted_fields" list. Keeping the names aligned with
// domain.Learning's JSON tags means a reviewer can correlate the report
// against a persisted Learning without translation.
type PromotionFields struct {
	Title          string
	Context        string
	Observation    string
	ReusableLesson string
	Limits         string
	Recommended    []string
	RetrievalTerms []string
}

// RedactionReport records which fields RedactPromotionFields changed.
// The shape mirrors promotion.RedactionSummary so the promotion package
// can lift it into its result type without translation.
//
// AnyRedacted is the boolean short-circuit; RedactedFields lists every
// field whose content differed after redaction. The list is
// deduplicated (a slice with two redacted elements reports the field
// name once) and the order matches the canonical field order so
// reviewers can rely on the deterministic shape.
type RedactionReport struct {
	AnyRedacted    bool
	RedactedFields []string
}

// RedactPromotionFields applies RedactString to every free-text field
// of f in place and returns the report describing what changed. Run
// this BEFORE hashing: the preview fingerprint must reflect redacted
// content so the audit row is reproducible and the "what Promotion
// saw" digest stays free of secret bytes.
//
// The function is pure (no I/O, no clock, no random) so it composes
// deterministically in tests. A nil receiver returns an empty report
// rather than panicking; the promotion pipeline must never crash on a
// partial bag, only fail later with a typed validation error.
func RedactPromotionFields(f *PromotionFields) RedactionReport {
	if f == nil {
		return RedactionReport{}
	}

	var changed []string

	if r := RedactString(f.Title); r != f.Title {
		changed = append(changed, "title")
		f.Title = r
	}
	if r := RedactString(f.Context); r != f.Context {
		changed = append(changed, "context")
		f.Context = r
	}
	if r := RedactString(f.Observation); r != f.Observation {
		changed = append(changed, "observation")
		f.Observation = r
	}
	if r := RedactString(f.ReusableLesson); r != f.ReusableLesson {
		changed = append(changed, "reusable_lesson")
		f.ReusableLesson = r
	}
	if r := RedactString(f.Limits); r != f.Limits {
		changed = append(changed, "limits")
		f.Limits = r
	}
	for i, step := range f.Recommended {
		if r := RedactString(step); r != step {
			appendUnique(&changed, "recommended")
			f.Recommended[i] = r
		}
	}
	for i, term := range f.RetrievalTerms {
		if r := RedactString(term); r != term {
			appendUnique(&changed, "retrieval_terms")
			f.RetrievalTerms[i] = r
		}
	}

	return RedactionReport{
		AnyRedacted:    len(changed) > 0,
		RedactedFields: changed,
	}
}

// appendUnique adds name to changed only if it is not already present.
// Keeping the slice deduplicated lets the audit row report
// "recommended" once even when every element of the slice was redacted,
// which keeps the report shape predictable for the reviewer UI.
func appendUnique(changed *[]string, name string) {
	for _, existing := range *changed {
		if existing == name {
			return
		}
	}
	*changed = append(*changed, name)
}

// PromotionFingerprint computes a deterministic, order-independent
// SHA-256 digest over the canonical form of f. The input MUST already
// be redacted (run RedactPromotionFields first); this is the "what
// Promotion saw" hash, separate from the normalized hash
// capture.Service computes over the persisted Learning.
//
// Recommended and RetrievalTerms are sorted before serialisation so the
// digest is stable regardless of the order in which the caller built
// the slices. The hash is a 64-character lowercase hex string, matching
// the existing patterns.digestCluster and domain.ComputeHash output
// shape so the audit row carries a uniform fingerprint format.
func PromotionFingerprint(f PromotionFields) string {
	recommended := append([]string(nil), f.Recommended...)
	sort.Strings(recommended)

	retrievalTerms := append([]string(nil), f.RetrievalTerms...)
	sort.Strings(retrievalTerms)

	payload := struct {
		Title          string   `json:"title"`
		Context        string   `json:"context"`
		Observation    string   `json:"observation"`
		ReusableLesson string   `json:"reusable_lesson"`
		Limits         string   `json:"limits"`
		Recommended    []string `json:"recommended"`
		RetrievalTerms []string `json:"retrieval_terms"`
	}{
		Title:          f.Title,
		Context:        f.Context,
		Observation:    f.Observation,
		ReusableLesson: f.ReusableLesson,
		Limits:         f.Limits,
		Recommended:    recommended,
		RetrievalTerms: retrievalTerms,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		// json.Marshal over the fixed struct shape cannot fail at
		// runtime. The fallback matches patterns.digestCluster: a
		// deterministic, fingerprint-shaped digest even on the
		// impossible path, so callers never see a panic.
		encoded = []byte(strconv.Itoa(len(recommended)) + ":" + strings.Join(retrievalTerms, ","))
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
