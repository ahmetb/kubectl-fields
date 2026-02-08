package output

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatOutput_ColorDisabled(t *testing.T) {
	input := "replicas: 3 # kubectl-apply (30m ago)\nimage: nginx # helm (2h ago)\n"

	got := FormatOutput(input, false, nil)

	// Should be aligned but no ANSI codes
	assert.NotContains(t, got, "\x1b[")
	// Should still have comments
	assert.Contains(t, got, "# kubectl-apply (30m ago)")
	assert.Contains(t, got, "# helm (2h ago)")
}

func TestFormatOutput_ColorEnabled(t *testing.T) {
	input := "replicas: 3 # kubectl-apply (30m ago)\nimage: nginx # helm (2h ago)\n"

	cm := NewColorManager()
	got := FormatOutput(input, true, cm)

	// Should contain ANSI codes
	assert.Contains(t, got, "\x1b[")
	// Should contain reset
	assert.Contains(t, got, Reset)
	// YAML content should NOT be colorized
	assert.Contains(t, got, "replicas: 3")
	assert.Contains(t, got, "image: nginx")
}

func TestColorize_InlineComment(t *testing.T) {
	input := "replicas: 3  # kubectl-apply (30m ago)"

	cm := NewColorManager()
	got := Colorize(input, cm)

	// The comment portion should be wrapped in color
	expectedColor := BrightPalette[0]
	assert.Contains(t, got, expectedColor+"# kubectl-apply (30m ago)"+Reset)
	// YAML content should not be colored
	assert.True(t, strings.HasPrefix(got, "replicas: 3"))
}

func TestColorize_AboveComment(t *testing.T) {
	input := "  # kubectl-apply (5m ago)\n  replicas: 3"

	cm := NewColorManager()
	got := Colorize(input, cm)

	lines := strings.Split(got, "\n")
	// First line should have color
	assert.Contains(t, lines[0], BrightPalette[0])
	assert.Contains(t, lines[0], Reset)
	// Second line should not have color
	assert.Equal(t, "  replicas: 3", lines[1])
}

func TestColorize_NoComment(t *testing.T) {
	input := "replicas: 3\nimage: nginx"

	cm := NewColorManager()
	got := Colorize(input, cm)

	// No ANSI codes should be present
	assert.NotContains(t, got, "\x1b[")
	assert.Equal(t, input, got)
}

func TestColorize_MultipleManagers(t *testing.T) {
	input := "replicas: 3  # kubectl-apply (30m ago)\nimage: nginx  # helm (2h ago)"

	cm := NewColorManager()
	got := Colorize(input, cm)

	lines := strings.Split(got, "\n")

	// First line: kubectl-apply gets palette[0]
	assert.Contains(t, lines[0], BrightPalette[0])
	// Second line: helm gets palette[1]
	assert.Contains(t, lines[1], BrightPalette[1])
}

func TestColorize_SameManagerSameColor(t *testing.T) {
	input := "a: 1  # kubectl-apply (1h ago)\nb: 2  # kubectl-apply (1h ago)"

	cm := NewColorManager()
	got := Colorize(input, cm)

	lines := strings.Split(got, "\n")
	// Both lines should use the same color (palette[0])
	assert.Contains(t, lines[0], BrightPalette[0])
	assert.Contains(t, lines[1], BrightPalette[0])
}

func TestColorize_YAMLContentNotColored(t *testing.T) {
	input := "apiVersion: v1\nkind: Pod\nreplicas: 3  # kubectl-apply (30m ago)"

	cm := NewColorManager()
	got := Colorize(input, cm)

	lines := strings.Split(got, "\n")
	// Non-comment lines should be unchanged
	assert.Equal(t, "apiVersion: v1", lines[0])
	assert.Equal(t, "kind: Pod", lines[1])
	// Comment line should have YAML content intact (before the color)
	assert.True(t, strings.HasPrefix(lines[2], "replicas: 3"))
}

func TestFormatOutput_AlignmentAlwaysRuns(t *testing.T) {
	// Two lines with different content lengths but inline comments
	input := "a: 1 # mgr (1h ago)\nlong-name: value # mgr (1h ago)\n"

	got := FormatOutput(input, false, nil)

	// Comments should be aligned even with color disabled
	// "a: 1" = 4 chars, "long-name: value" = 16 chars
	// Alignment column = 16 + 2 = 18
	assert.Contains(t, got, "long-name: value  # mgr (1h ago)")
	// "a: 1" padded to 18 chars
	assert.Contains(t, got, "a: 1              # mgr (1h ago)")
}

func TestFormatOutput_NoANSIWhenPiped(t *testing.T) {
	// Simulate piped output (non-TTY): color disabled
	input := "replicas: 3 # kubectl-apply (50m ago)\n" +
		"image: nginx:1.14.2 # kubectl-client-side-apply (50m ago)\n" +
		"status:\n" +
		"  availableReplicas: 3 # kube-controller-manager /status (1h ago)\n"

	got := FormatOutput(input, false, nil)

	// Verify zero ANSI escape sequences in output
	assert.NotContains(t, got, "\x1b", "piped output must contain no ANSI escape codes")
	// But comments should still be present
	assert.Contains(t, got, "# kubectl-apply")
	assert.Contains(t, got, "# kube-controller-manager /status")
}

func TestFormatOutput_ANSIPresentWhenColorEnabled(t *testing.T) {
	// Color enabled: ANSI codes should be present in comment portions
	input := "replicas: 3 # kubectl-apply (50m ago)\n" +
		"image: nginx:1.14.2 # helm (2h ago)\n"

	cm := NewColorManager()
	got := FormatOutput(input, true, cm)

	// Must contain ANSI escape sequences
	assert.Contains(t, got, "\x1b[", "colored output must contain ANSI escape codes")
	assert.Contains(t, got, Reset, "colored output must contain ANSI reset")
	// YAML content should remain uncolored
	assert.Contains(t, got, "replicas: 3")
	assert.Contains(t, got, "image: nginx:1.14.2")
}

func TestFormatOutput_AlignmentIntegration_RealisticBlock(t *testing.T) {
	// Realistic block from a deployment: adjacent annotated lines should align
	input := "  replicas: 3 # kubectl-apply (50m ago)\n" +
		"  revisionHistoryLimit: 10 # kubectl-apply (50m ago)\n" +
		"  progressDeadlineSeconds: 600 # kubectl-apply (50m ago)\n"

	got := FormatOutput(input, false, nil)

	// All three lines form a block. Find the comment column for each line.
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	assert.Len(t, lines, 3)

	commentCols := make([]int, len(lines))
	for i, line := range lines {
		idx := strings.Index(line, " # ")
		assert.Greater(t, idx, 0, "line %d should have inline comment", i)
		commentCols[i] = idx
	}

	// All comments should be at the same column
	assert.Equal(t, commentCols[0], commentCols[1], "lines 0 and 1 should have aligned comments")
	assert.Equal(t, commentCols[1], commentCols[2], "lines 1 and 2 should have aligned comments")
}
