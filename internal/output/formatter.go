package output

import (
	"strings"
)

// FormatOutput orchestrates the output pipeline: alignment then optional colorization.
//
// Alignment always runs (per user decision). Colorization runs only when
// colorEnabled is true. The colorMgr may be nil when color is disabled.
func FormatOutput(text string, colorEnabled bool, colorMgr *ColorManager) string {
	aligned := AlignComments(text)
	if colorEnabled && colorMgr != nil {
		return Colorize(aligned, colorMgr)
	}
	return aligned
}

// Colorize applies ANSI color codes to YAML comments based on manager names.
//
// For each line, it detects:
//   - Inline comments: "content # manager ..." -> colors the "# manager ..." portion
//   - Above-mode comments: "  # manager ..." -> colors the "# manager ..." portion
//   - Non-comment lines: pass through unchanged
//
// The "#" is included in the colored text per user decision.
func Colorize(text string, cm *ColorManager) string {
	lines := strings.Split(text, "\n")
	result := make([]string, len(lines))

	for i, line := range lines {
		result[i] = colorizeLine(line, cm)
	}

	return strings.Join(result, "\n")
}

// colorizeLine applies color to a single line if it contains a comment.
func colorizeLine(line string, cm *ColorManager) string {
	// Try inline comment first: "content # comment"
	content, comment, hasInline := splitInlineComment(line)
	if hasInline {
		manager := extractManagerName(comment)
		if manager != "" {
			return content + " " + cm.Wrap(comment, manager)
		}
		return line
	}

	// Try above-mode comment: optional whitespace then "# ..."
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "# ") {
		// Find where the comment starts in the original line
		commentStart := strings.Index(line, "#")
		prefix := line[:commentStart]
		commentText := line[commentStart:]
		manager := extractManagerName(commentText)
		if manager != "" {
			return prefix + cm.Wrap(commentText, manager)
		}
	}

	return line
}
