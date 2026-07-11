package capture

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"agent-royo-learn/internal/domain"
)

// Fingerprint produces a stable, lexical fingerprint of a learning's
// semantically meaningful fields for similarity detection.
func Fingerprint(learning *domain.Learning) string {
	if learning == nil {
		return ""
	}

	source := strings.Join([]string{
		string(learning.Type),
		learning.Title,
		learning.Context,
		learning.Observation,
	}, "\x00")

	sum := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x", sum)
}
