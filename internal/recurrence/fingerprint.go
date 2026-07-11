package recurrence

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"agent-royo-learn/internal/domain"
)

// RecurrenceFingerprint produces a stable, content-based fingerprint for
// cross-capture recurrence detection. Normalizes whitespace and case to
// group learnings that represent the same conceptual pattern even when
// captured by different agents or with minor formatting differences.
func RecurrenceFingerprint(learning *domain.Learning) string {
	if learning == nil {
		return ""
	}

	norm := func(s string) string {
		return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
	}

	source := strings.Join([]string{
		norm(learning.Title),
		norm(learning.Observation),
		norm(learning.ReusableLesson),
	}, "\x00")

	sum := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x", sum)
}
