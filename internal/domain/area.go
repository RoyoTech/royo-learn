package domain

import (
	"fmt"
	"sort"
	"strings"
)

// MaxSkillAreaLength is the maximum allowed length for an explicit skill area.
const MaxSkillAreaLength = 64

// SanitizeSkillArea keeps alphanumeric, dash, underscore; replaces spaces with
// dash; drops everything else. Does NOT lowercase.
func SanitizeSkillArea(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, s)
}

// ValidateExplicitArea validates and normalizes an explicit skill area provided
// by the curator. Returns the normalized (sanitized + lowercased) area. An
// empty input is valid and returns "" (meaning: derive automatically). A
// non-empty input that is empty after sanitizing, or longer than
// MaxSkillAreaLength, is an invalid_argument error.
// DeriveSkillArea deterministically derives a skill area from a learning's
// retrieval terms. It sorts ALL terms case-insensitively, takes the first,
// sanitizes and lowercases it, and falls back to "general" when the learning
// is nil, has no retrieval terms, or the best term sanitizes to empty.
func DeriveSkillArea(learning *Learning) string {
	if learning == nil || len(learning.RetrievalTerms) == 0 {
		return "general"
	}
	terms := make([]string, len(learning.RetrievalTerms))
	copy(terms, learning.RetrievalTerms)
	sort.Slice(terms, func(i, j int) bool {
		return strings.ToLower(terms[i]) < strings.ToLower(terms[j])
	})
	best := terms[0]
	sanitized := strings.ToLower(SanitizeSkillArea(best))
	if sanitized == "" {
		return "general"
	}
	return sanitized
}

func ValidateExplicitArea(area string) (string, error) {
	if area == "" {
		return "", nil
	}
	if len(area) > MaxSkillAreaLength {
		return "", NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("area too long: %d chars (max %d)", len(area), MaxSkillAreaLength))
	}
	sanitized := strings.ToLower(SanitizeSkillArea(area))
	if sanitized == "" {
		return "", NewValidationError(ErrInvalidArgument, "area inválida: queda vacía tras sanitizar")
	}
	return sanitized, nil
}
