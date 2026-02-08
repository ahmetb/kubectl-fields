package output

import (
	"strings"
)

// MinGap is the minimum number of spaces between YAML content and an inline comment.
const MinGap = 2

// splitInlineComment splits a line at the inline comment delimiter " # ".
// Returns the content before the delimiter, the comment from "# " onward
// (including the "# " prefix), and whether an inline comment was found.
//
// Lines that are above-mode head comments (leading whitespace + "# ...") return
// false, as they are not inline comments. Lines without any " # " also return false.
func splitInlineComment(line string) (content string, comment string, hasComment bool) {
	// Check if this is an above-mode comment line: optional whitespace then "#"
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "#") {
		return line, "", false
	}

	// Find the last occurrence of " # " to handle edge cases with # in values
	idx := strings.LastIndex(line, " # ")
	if idx < 0 {
		return line, "", false
	}

	return line[:idx], line[idx+1:], true
}

// AlignComments performs per-block alignment of inline comments.
//
// Consecutive lines with inline comments form a block. A line without an
// inline comment breaks the block. Within each block, comments are aligned
// to (max content width + MinGap) so they form a uniform column.
//
// Above-mode head comments (lines starting with optional whitespace then "#")
// pass through unchanged. They are not considered inline comments and do not
// participate in block formation.
func AlignComments(text string) string {
	lines := strings.Split(text, "\n")

	type annotatedLine struct {
		content    string
		comment    string
		hasComment bool
		original   string
	}

	parsed := make([]annotatedLine, len(lines))
	for i, line := range lines {
		content, comment, has := splitInlineComment(line)
		parsed[i] = annotatedLine{
			content:    content,
			comment:    comment,
			hasComment: has,
			original:   line,
		}
	}

	// Process blocks of consecutive annotated lines
	result := make([]string, len(lines))
	i := 0
	for i < len(parsed) {
		if !parsed[i].hasComment {
			result[i] = parsed[i].original
			i++
			continue
		}

		// Start of a block: find extent
		blockStart := i
		maxContentLen := 0
		for i < len(parsed) && parsed[i].hasComment {
			if len(parsed[i].content) > maxContentLen {
				maxContentLen = len(parsed[i].content)
			}
			i++
		}

		// Alignment column = max content length + MinGap
		alignCol := maxContentLen + MinGap

		// Format each line in the block
		for j := blockStart; j < i; j++ {
			contentLen := len(parsed[j].content)
			gap := alignCol - contentLen
			if gap < MinGap {
				gap = MinGap
			}
			result[j] = parsed[j].content + strings.Repeat(" ", gap) + parsed[j].comment
		}
	}

	return strings.Join(result, "\n")
}
