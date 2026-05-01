// Package emoji provides custom emoji parsing from Discord message content.
//
// Custom Discord emojis appear in message content as <:name:id> (static) or
// <a:name:id> (animated). This package parses all occurrences, counting each
// occurrence individually (not deduplicating within a message).
package emoji

import "regexp"

// ParsedEmoji represents a single custom emoji occurrence found in message content.
type ParsedEmoji struct {
	Name string
	ID   string
}

// Matches both animated (<a:name:id>) and static (<:name:id>) custom Discord emojis.
// Capturing groups: [1]=emoji name, [2]=emoji ID.
var customEmojiRegex = regexp.MustCompile(`<a?:([a-zA-Z0-9_]+):(\d+)>`)

// Parse returns all custom emoji occurrences found in [content].
// Each occurrence is counted individually — if the same emoji appears twice
// in one message, it produces two entries.
func Parse(content string) []ParsedEmoji {
	matches := customEmojiRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make([]ParsedEmoji, 0, len(matches))
	for _, match := range matches {
		result = append(result, ParsedEmoji{
			Name: match[1],
			ID:   match[2],
		})
	}
	return result
}
