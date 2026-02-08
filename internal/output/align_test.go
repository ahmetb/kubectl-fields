package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitInlineComment_WithComment(t *testing.T) {
	content, comment, has := splitInlineComment("replicas: 3 # kubectl-apply (30m ago)")
	assert.True(t, has)
	assert.Equal(t, "replicas: 3", content)
	assert.Equal(t, "# kubectl-apply (30m ago)", comment)
}

func TestSplitInlineComment_NoComment(t *testing.T) {
	content, comment, has := splitInlineComment("replicas: 3")
	assert.False(t, has)
	assert.Equal(t, "replicas: 3", content)
	assert.Equal(t, "", comment)
}

func TestSplitInlineComment_AboveMode(t *testing.T) {
	// Above-mode head comment: whitespace + "# ..." should NOT be treated as inline
	content, comment, has := splitInlineComment("  # kubectl-apply (5m ago)")
	assert.False(t, has)
	assert.Equal(t, "  # kubectl-apply (5m ago)", content)
	assert.Equal(t, "", comment)
}

func TestSplitInlineComment_AboveModeNoIndent(t *testing.T) {
	// Above-mode head comment at column 0
	_, _, has := splitInlineComment("# kubectl-apply (5m ago)")
	assert.False(t, has)
}

func TestAlignComments_BlockOf3(t *testing.T) {
	input := "replicas: 3 # mgr-a (1h ago)\nimage: nginx # mgr-b (2h ago)\nname: test # mgr-a (1h ago)\n"

	got := AlignComments(input)

	// All three lines should be aligned to the same column
	// "replicas: 3" is 11 chars, "image: nginx" is 12 chars, "name: test" is 10 chars
	// Max content = 12, align col = 14
	assert.Contains(t, got, "replicas: 3   # mgr-a (1h ago)")  // 3 spaces (14-11)
	assert.Contains(t, got, "image: nginx  # mgr-b (2h ago)")  // 2 spaces (14-12)
	assert.Contains(t, got, "name: test    # mgr-a (1h ago)")  // 4 spaces (14-10)
}

func TestAlignComments_BrokenByUnannotatedLine(t *testing.T) {
	input := "a: 1 # mgr (1h ago)\nb: 2\nc: longer-value # mgr (1h ago)\n"

	got := AlignComments(input)

	// "a: 1" and "c: longer-value" are in different blocks
	// Block 1: just "a: 1" (4 chars, aligned at 6)
	// Block 2: just "c: longer-value" (15 chars, aligned at 17)
	assert.Contains(t, got, "a: 1  # mgr (1h ago)")
	assert.Contains(t, got, "b: 2")
	assert.Contains(t, got, "c: longer-value  # mgr (1h ago)")
}

func TestAlignComments_LongLineMinGap(t *testing.T) {
	input := "short: 1 # mgr (1h ago)\nvery-long-field-name: very-long-value # mgr (1h ago)\n"

	got := AlignComments(input)

	// "short: 1" is 8 chars, "very-long-field-name: very-long-value" is 37 chars
	// Max = 37, align col = 39
	// "short: 1" gets padded to 39 then comment
	assert.Contains(t, got, "very-long-field-name: very-long-value  # mgr (1h ago)")
	// short line should be padded with spaces
	assert.Contains(t, got, "short: 1"+spaces(31)+"# mgr (1h ago)")
}

func TestAlignComments_MixedAnnotatedAndBare(t *testing.T) {
	input := "apiVersion: v1\nkind: Pod\nmetadata: # mgr (1h ago)\n  name: test # mgr (1h ago)\nspec:\n  containers: # mgr (2h ago)\n"

	got := AlignComments(input)

	// "apiVersion: v1" and "kind: Pod" are bare, pass through
	assert.Contains(t, got, "apiVersion: v1\n")
	assert.Contains(t, got, "kind: Pod\n")
	// "metadata:" and "  name: test" form a block
	// "metadata:" is 9 chars, "  name: test" is 12 chars -> max=12, align=14
	assert.Contains(t, got, "metadata:     # mgr (1h ago)")
	assert.Contains(t, got, "  name: test  # mgr (1h ago)")
	// "spec:" is bare
	assert.Contains(t, got, "spec:\n")
	// "  containers:" alone in its block
	assert.Contains(t, got, "  containers:  # mgr (2h ago)")
}

func TestAlignComments_AboveModePassthrough(t *testing.T) {
	input := "# kubectl-apply (5m ago)\nreplicas: 3\n"

	got := AlignComments(input)

	// Above-mode comments should pass through unchanged
	assert.Equal(t, input, got)
}

func TestAlignComments_EmptyInput(t *testing.T) {
	assert.Equal(t, "", AlignComments(""))
}

func TestAlignComments_NoComments(t *testing.T) {
	input := "replicas: 3\nimage: nginx\n"
	assert.Equal(t, input, AlignComments(input))
}

// spaces is a helper that generates n space characters.
func spaces(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += " "
	}
	return s
}
