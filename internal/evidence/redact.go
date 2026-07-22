package evidence

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

// secretPatterns defines detection patterns with their replacement labels.
// Patterns are applied in order; the first match at a given position wins.
var secretPatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{"openai_key", regexp.MustCompile(`sk-(proj-)?[A-Za-z0-9]{8,}`)},
	{"anthropic_key", regexp.MustCompile(`sk-ant-[A-Za-z0-9]{10,}`)},
	{"github_token", regexp.MustCompile(`gh[ps]_[A-Za-z0-9]{16,}`)},
	{"aws_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"private_key", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----[\s\S]*?-----END (RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----`)},
	{"connection_string", regexp.MustCompile(`(?i)(?:Password|Pwd)\s*=\s*(\S+)`)},
	{"password_url", regexp.MustCompile(`://[^:@/]+:([^:@/]+)@`)},
	{"bearer_token", regexp.MustCompile(`Bearer\s+([A-Za-z0-9._\-]{20,})`)},
	{"cookie", regexp.MustCompile(`(?i)\b(?:Cookie|Set-Cookie)\s*[:=]\s*[^\r\n]+`)},
	{"private_tag", regexp.MustCompile(`(?is)<private\b[^>]*>.*?</private\s*>`)},
	{"env_assignment", regexp.MustCompile(`(?i)\b(?:SECRET|TOKEN|KEY|PASSWORD|API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY)\b["']?\s*[:=]\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)},
}

// Redact replaces known secrets in content with [REDACTED:<type>] markers.
// Patterns are applied in order; once content is replaced by an earlier
// pattern, later patterns won't re-match the replacement text.
func Redact(content []byte, knownSecrets []string) []byte {
	if len(content) == 0 {
		return content
	}

	// Phase 1: Replace known secrets with a temporary marker.
	for _, secret := range knownSecrets {
		if secret == "" {
			continue
		}
		content = bytes.ReplaceAll(content, []byte(secret), []byte("[REDACTED:known]"))
	}

	// Phase 2: Apply pattern-based redaction in order.
	// To prevent later patterns from matching text already replaced,
	// we apply patterns one by one and mark replaced ranges.
	for _, p := range secretPatterns {
		content = p.re.ReplaceAllFunc(content, func(match []byte) []byte {
			return []byte(fmt.Sprintf("[REDACTED:%s]", p.label))
		})
	}

	return content
}

// DetectSecrets scans content for known secret patterns and returns
// a deduplicated list of detected secret hashes (SHA-256 of the matched text).
func DetectSecrets(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	for _, p := range secretPatterns {
		matches := p.re.FindAll(content, -1)
		for _, match := range matches {
			key := sha256Hex(match)
			if !seen[key] {
				seen[key] = true
			}
		}
	}

	var result []string
	for k := range seen {
		result = append(result, k)
	}
	return result
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
