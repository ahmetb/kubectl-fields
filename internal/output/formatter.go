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
// The "#" is included in the colored text per user decision. For co-managed
// fields, the comment contains multiple "manager (age)" segments joined by
// "; " (see annotate.formatTargetComment); each segment is colored with its
// own manager's color.
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
			return content + " " + colorizeComment(comment, cm)
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
			return prefix + colorizeComment(commentText, cm)
		}
	}

	return line
}

// colorizeComment colors a full comment string (including its leading "#" or
// "# " marker). When the comment describes a single manager, the entire
// string is wrapped in that manager's color.
//
// When the comment describes a co-managed field -- multiple "manager (age)"
// segments joined by "; " -- each segment is colored independently with its
// own manager's color, and the "; " separators are left uncolored. The
// leading "#"/"# " marker is kept attached to (and colored with) the first
// segment only.
func colorizeComment(commentText string, cm *ColorManager) string {
	hashPrefix := ""
	rest := commentText
	switch {
	case strings.HasPrefix(rest, "# "):
		hashPrefix, rest = "# ", rest[2:]
	case strings.HasPrefix(rest, "#"):
		hashPrefix, rest = "#", rest[1:]
	}

	segments := strings.Split(rest, "; ")
	wrapped := make([]string, len(segments))
	for i, seg := range segments {
		text := seg
		if i == 0 {
			text = hashPrefix + seg
		}
		manager := extractManagerName(seg)
		if manager == "" {
			wrapped[i] = text
			continue
		}
		wrapped[i] = cm.Wrap(text, manager)
	}

	return strings.Join(wrapped, "; ")
}
