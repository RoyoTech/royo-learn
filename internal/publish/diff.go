package publish

import (
	"fmt"
	"strings"
)

// GenerateDiff produces a canonical unified diff string showing what would change
// without applying the changes.
func GenerateDiff(current []byte, proposed []byte, path string, exists bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", path))
	if exists {
		b.WriteString(fmt.Sprintf("+++ b/%s\n", path))
	} else {
		b.WriteString(fmt.Sprintf("+++ b/%s (new file)\n", path))
	}

	if !exists {
		// New file: show all lines as additions.
		lines := strings.Split(string(proposed), "\n")
		for _, line := range lines {
			b.WriteString(fmt.Sprintf("+%s\n", line))
		}
		return b.String()
	}

	// Simple line-by-line diff.
	currentLines := strings.Split(string(current), "\n")
	proposedLines := strings.Split(string(proposed), "\n")

	maxLen := len(currentLines)
	if len(proposedLines) > maxLen {
		maxLen = len(proposedLines)
	}

	for i := 0; i < maxLen; i++ {
		curLine := ""
		propLine := ""
		if i < len(currentLines) {
			curLine = currentLines[i]
		}
		if i < len(proposedLines) {
			propLine = proposedLines[i]
		}

		if curLine == propLine {
			b.WriteString(fmt.Sprintf(" %s\n", curLine))
		} else {
			if curLine != "" || propLine == "" {
				b.WriteString(fmt.Sprintf("-%s\n", curLine))
			}
			if propLine != "" || curLine == "" {
				b.WriteString(fmt.Sprintf("+%s\n", propLine))
			}
		}
	}

	return b.String()
}

// DiffSummary returns a human-readable summary of the diff, excluding headers.
func DiffSummary(diffString string, path string) string {
	lines := strings.Split(diffString, "\n")
	added := 0
	removed := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			added++
		}
		if strings.HasPrefix(line, "-") {
			removed++
		}
	}

	if added == 0 && removed == 0 {
		return fmt.Sprintf("No changes to %s", path)
	}
	return fmt.Sprintf("%s: +%d/-%d lines", path, added, removed)
}
