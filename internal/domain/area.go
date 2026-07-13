package domain

import (
	"fmt"
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
