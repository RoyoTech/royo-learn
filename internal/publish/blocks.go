package publish

import (
	"fmt"
	"strings"
)

const (
	// ManagedBlockStart marks the beginning of a royo-learn managed content block.
	ManagedBlockStart = "<!-- royo-learn:managed start -->"
	// ManagedBlockEnd marks the end of a royo-learn managed content block.
	ManagedBlockEnd = "<!-- royo-learn:managed end -->"
)

// ManagedBlock represents a royo-learn managed content block within a file.
type ManagedBlock struct {
	StartLine int
	EndLine   int
	Content   string
}

// ParseManagedBlocks extracts all managed blocks from file content.
func ParseManagedBlocks(content string) []ManagedBlock {
	lines := strings.Split(content, "\n")
	var blocks []ManagedBlock

	inBlock := false
	blockStart := 0
	var blockLines []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == ManagedBlockStart {
			inBlock = true
			blockStart = i + 1
			blockLines = nil
			continue
		}
		if trimmed == ManagedBlockEnd {
			if inBlock {
				blocks = append(blocks, ManagedBlock{
					StartLine: blockStart,
					EndLine:   i + 1,
					Content:   strings.Join(blockLines, "\n"),
				})
				inBlock = false
			}
			continue
		}
		if inBlock {
			blockLines = append(blockLines, line)
		}
	}

	return blocks
}

// FindManagedBlock finds a managed block by ID (the first line of the block
// content is expected to contain an identifying comment).
func FindManagedBlock(content string, blockID string) (*ManagedBlock, error) {
	blocks := ParseManagedBlocks(content)
	for i := range blocks {
		if strings.Contains(blocks[i].Content, blockID) {
			return &blocks[i], nil
		}
	}
	return nil, fmt.Errorf("FindManagedBlock: block %q not found", blockID)
}

// ReplaceManagedBlock replaces the content of a managed block within file content.
// Returns the modified content and an error if the block is not found.
func ReplaceManagedBlock(content string, blockID string, newContent string) (string, error) {
	block, err := FindManagedBlock(content, blockID)
	if err != nil {
		return "", err
	}

	lines := strings.Split(content, "\n")

	// Build new content: lines before block + managed start + new content + managed end + lines after block.
	var result []string
	result = append(result, lines[:block.StartLine-1]...)        // before managed start
	result = append(result, "<!-- royo-learn:managed start -->") // managed start
	result = append(result, strings.Split(newContent, "\n")...)  // new content
	result = append(result, "<!-- royo-learn:managed end -->")   // managed end
	result = append(result, lines[block.EndLine:]...)            // after managed end

	return strings.Join(result, "\n"), nil
}

// InsertManagedBlock inserts a new managed block at the end of file content.
// If the file already has managed blocks, appends to the last one's end.
func InsertManagedBlock(content string, blockContent string) string {
	// If content already has managed blocks, append to the last managed block.
	blocks := ParseManagedBlocks(content)
	if len(blocks) > 0 {
		// Find the end of the last managed block and insert new content after it.
		lines := strings.Split(content, "\n")
		lastBlock := blocks[len(blocks)-1]
		var result []string

		// Everything up to and including the last managed block end.
		result = append(result, lines[:lastBlock.EndLine]...)
		// New managed block.
		result = append(result, "<!-- royo-learn:managed start -->")
		result = append(result, strings.Split(blockContent, "\n")...)
		result = append(result, "<!-- royo-learn:managed end -->")
		// Everything after.
		if lastBlock.EndLine < len(lines) {
			result = append(result, lines[lastBlock.EndLine:]...)
		}

		return strings.Join(result, "\n")
	}

	// No existing managed blocks — append at the end.
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n<!-- royo-learn:managed start -->\n" + blockContent + "\n<!-- royo-learn:managed end -->\n"
}

// HasManagedBlocks returns true if the content contains at least one managed block.
func HasManagedBlocks(content string) bool {
	return strings.Contains(content, ManagedBlockStart) && strings.Contains(content, ManagedBlockEnd)
}
