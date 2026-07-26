// Sanitize normalizes the user's free-form query into a list of
// FTS5-safe terms. The contract is documented in docs/27 §Sanitization
// and pinned by contract_test.go.
//
// Rules (v1):
//
//   1. Tokenize on Unicode whitespace.
//   2. For each token, drop it when:
//        - it is empty after trim,
//        - it equals ".." or starts with "/" (path traversal),
//        - it is longer than MaxTermLength (256 chars),
//        - it contains any character outside the whitelist
//          `[\p{L}\p{N}_.\-]+`.
//   3. Drop duplicates preserving first-occurrence order.
//   4. If the input yields more than MaxTermsPerQuery (16) raw
//      tokens before filtering, return ErrTooManyTerms. This is
//      distinct from the "filtered down to zero" case: an
//      oversize QUERY is an operator error, an oversize TERM is
//      a quality-of-input issue.
//   5. Empty input (or all-invalid input) returns an empty slice
//      with nil error: the caller decides whether to treat "no
//      searchable terms" as a hard failure at the Service layer.
//
// The v1 fix vs. the prior sanitizeFTS:
//
//   - AND/OR/NOT/NEAR are no longer stripped. They survive the
//     whitelist (they are letters) and reach the FTS5 MATCH clause
//     wrapped in `"..."`, so FTS5 sees them as literal terms, not
//     boolean operators.
//   - Length and character validation happen BEFORE the FTS5 call,
//     so a hostile query cannot blow up the SQLite MATCH parser.
//   - Path-traversal fragments ("..", "/etc/...") are dropped
//     before they reach FTS5. The v1 retains "etc" and "passwd"
//     as searchable terms because the user clearly meant the words.
//   - Double-quote escaping: each surviving term is escaped with
//     FTS5's `""` so a literal quote inside a term does not break
//     out of the phrase.

package retrieval

import (
	"regexp"
	"strings"
	"unicode"
)

// termWhitelist accepts Unicode letters and numbers plus the three
// safe punctuation characters that commonly appear in identifiers
// (`_`, `.`, `-`). Anything else (control characters, slashes,
// quotes, FTS5 operators like `*` `:` `(` `)` `^` `~`) is rejected.
//
// We compile once at package init so the hot path stays cheap.
var termWhitelist = regexp.MustCompile(`^[\p{L}\p{N}_.\-]+$`)

// Sanitize returns the searchable terms extracted from query.
//
// The error is non-nil only when the raw token count exceeds
// MaxTermsPerQuery (ErrTooManyTerms). All other rejections are
// silent because the operator can recover from "too narrow a
// search" but not from a UI that explodes on a typo.
func Sanitize(query string) ([]string, error) {
	if query == "" {
		return []string{}, nil
	}

	raw := tokenize(query)
	if len(raw) > MaxTermsPerQuery {
		return nil, ErrTooManyTerms
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, term := range raw {
		clean := reject(term)
		if clean == "" {
			continue
		}
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	if out == nil {
		out = []string{}
	}
	return out, nil
}

// tokenize splits the query into candidate terms. The split is
// aggressive: we break on Unicode whitespace AND on every rune
// that is not whitelisted. This guarantees that a single hostile
// token like "../etc/passwd" cannot smuggle `/` past the
// per-term validator — each piece is validated independently and
// `..` is filtered out by reject().
//
// The trade-off is "hello-world" (legitimate compound word) gets
// split into ["hello", "world"]. That is acceptable for v1: the
// user can quote it if they need exact-phrase matching, and the
// FTS5 phrase query is AND'd across terms anyway.
func tokenize(query string) []string {
	return strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || !isWhitelistedRune(r)
	})
}

// isWhitelistedRune mirrors the termWhitelist character class:
// Unicode letters and numbers, plus `_`, `.`, `-`. Anything else
// (whitespace, slashes, control chars, FTS5 operators, quotes)
// is a split boundary.
func isWhitelistedRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-'
}

// reject applies the per-term validation. It returns the term
// unchanged when valid, or "" when the term must be dropped.
//
// The rules, in order:
//
//   - empty after trim: drop
//   - ".." or starts with "/": drop (path traversal)
//   - longer than MaxTermLength: drop
//   - any character outside the whitelist: drop
func reject(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	if term == ".." {
		return ""
	}
	if strings.HasPrefix(term, "/") {
		return ""
	}
	if len(term) > MaxTermLength {
		return ""
	}
	if !termWhitelist.MatchString(term) {
		return ""
	}
	return term
}

// EscapeFTSPhrase wraps a term in FTS5 phrase quotes and doubles any
// embedded quote so it survives the MATCH parser. Empty input
// returns "" (caller can skip it).
//
// Example:
//
//	EscapeFTSPhrase("hello") -> `"hello"`
//	EscapeFTSPhrase(`say "hi"`) -> `"say ""hi"""`
func EscapeFTSPhrase(term string) string {
	if term == "" {
		return ""
	}
	escaped := strings.ReplaceAll(term, `"`, `""`)
	return `"` + escaped + `"`
}

// FTS5Query joins the supplied terms into a single MATCH expression.
// Each term is wrapped in FTS5 phrase quotes; the AND between terms
// is implicit (FTS5's default is "term AND term"). Empty input
// returns "".
//
// This is exported so the Repository (and any future helper) can
// share the exact escaping rules without re-deriving them.
func FTS5Query(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		if term == "" {
			continue
		}
		parts = append(parts, EscapeFTSPhrase(term))
	}
	return strings.Join(parts, " ")
}
