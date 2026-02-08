package output

import (
	"strings"
)

// MinGap is the minimum number of spaces between YAML content and an inline comment.
const MinGap = 2

// OutlierThreshold is the maximum allowed difference in content width between
// any line in an alignment block and the block's minimum. Lines exceeding this
// threshold are ejected into their own block to prevent a single long line from
// pushing all adjacent comments far to the right.
const OutlierThreshold = 40

// annotatedLine holds a parsed line with its inline comment separated out.
type annotatedLine struct {
	content    string
	comment    string
	hasComment bool
	original   string
}

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
// Lines that are outliers (content width exceeding the block minimum by more
// than OutlierThreshold) are aligned independently so a single long line does
// not push all adjacent comments far to the right.
//
// Above-mode head comments (lines starting with optional whitespace then "#")
// pass through unchanged. They are not considered inline comments and do not
// participate in block formation.
func AlignComments(text string) string {
	lines := strings.Split(text, "\n")

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

		// Collect consecutive annotated lines into a raw block
		blockStart := i
		for i < len(parsed) && parsed[i].hasComment {
			i++
		}

		// Split raw block into sub-blocks by ejecting outlier lines.
		alignBlock(parsed[blockStart:i], result[blockStart:i])
	}

	return strings.Join(result, "\n")
}

// alignBlock formats a slice of consecutive annotated lines, splitting outliers
// into their own alignment groups. Each non-outlier group is aligned to its own
// max content width. Outlier lines get MinGap spacing.
func alignBlock(block []annotatedLine, out []string) {
	// Find minimum content width to detect outliers
	minLen := len(block[0].content)
	for _, al := range block[1:] {
		if len(al.content) < minLen {
			minLen = len(al.content)
		}
	}

	// Partition into sub-blocks: consecutive non-outlier lines form a group,
	// each outlier is its own group.
	type span struct {
		start, end int
	}
	var spans []span

	i := 0
	for i < len(block) {
		if len(block[i].content)-minLen > OutlierThreshold {
			// Outlier: standalone span
			spans = append(spans, span{i, i + 1})
			i++
		} else {
			// Non-outlier: collect consecutive non-outliers
			start := i
			for i < len(block) && len(block[i].content)-minLen <= OutlierThreshold {
				i++
			}
			spans = append(spans, span{start, i})
		}
	}

	// Align each span independently
	for _, s := range spans {
		maxContentLen := 0
		for j := s.start; j < s.end; j++ {
			if len(block[j].content) > maxContentLen {
				maxContentLen = len(block[j].content)
			}
		}
		alignCol := maxContentLen + MinGap
		for j := s.start; j < s.end; j++ {
			gap := alignCol - len(block[j].content)
			if gap < MinGap {
				gap = MinGap
			}
			out[j] = block[j].content + strings.Repeat(" ", gap) + block[j].comment
		}
	}
}
