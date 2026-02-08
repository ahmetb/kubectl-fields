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
	// With outlier threshold=40, a 29-char difference (37-8) is NOT an outlier,
	// so they still align together.
	input := "short: 1 # mgr (1h ago)\nvery-long-field-name: very-long-value # mgr (1h ago)\n"

	got := AlignComments(input)

	// "short: 1" is 8 chars, "very-long-field-name: very-long-value" is 37 chars
	// Difference is 29, under threshold of 40, so same block
	assert.Contains(t, got, "very-long-field-name: very-long-value  # mgr (1h ago)")
	assert.Contains(t, got, "short: 1"+spaces(31)+"# mgr (1h ago)")
}

func TestAlignComments_OutlierEjected(t *testing.T) {
	// A line exceeding OutlierThreshold (40) beyond the block minimum gets ejected.
	// The outlier splits the block: lines before/after it form separate sub-blocks.
	short := "a: 1"                                                            // 4 chars
	medium := "bb: 22"                                                          // 6 chars
	long := "very-long-key-that-exceeds-threshold: very-long-value-here-too!!" // 65 chars (61 > 40 over min of 4)

	input := short + " # mgr-a (1h ago)\n" + long + " # mgr-b (2h ago)\n" + medium + " # mgr-c (3h ago)\n"

	got := AlignComments(input)

	// short is alone in sub-block 1 (before outlier): aligned at 4+2=6
	// long is outlier in sub-block 2: aligned at 65+2=67
	// medium is alone in sub-block 3 (after outlier): aligned at 6+2=8
	assert.Contains(t, got, "a: 1  # mgr-a (1h ago)")      // MinGap (sole member)
	assert.Contains(t, got, long+"  # mgr-b (2h ago)")     // MinGap for outlier
	assert.Contains(t, got, "bb: 22  # mgr-c (3h ago)")    // MinGap (sole member)
}

func TestAlignComments_OutlierBetweenGroup(t *testing.T) {
	// Non-outlier lines on the same side of an outlier group together.
	a := "aaa: 111"  // 8 chars
	b := "bb: 22"    // 6 chars
	long := "this-is-a-very-long-line-that-exceeds-the-outlier-threshold-by-far!!" // 69 chars
	c := "ccc: 333"  // 8 chars
	d := "dddd: 4444" // 10 chars

	input := a + " # m1 (1h)\n" + b + " # m2 (2h)\n" + long + " # m3 (3h)\n" + c + " # m4 (4h)\n" + d + " # m5 (5h)\n"

	got := AlignComments(input)

	// a (8) and b (6) form sub-block 1: aligned at max=8+2=10
	assert.Contains(t, got, "aaa: 111  # m1 (1h)")    // 2 spaces (10-8)
	assert.Contains(t, got, "bb: 22    # m2 (2h)")    // 4 spaces (10-6)
	// long is outlier: MinGap
	assert.Contains(t, got, long+"  # m3 (3h)")
	// c (8) and d (10) form sub-block 3: aligned at max=10+2=12
	assert.Contains(t, got, "ccc: 333    # m4 (4h)")  // 4 spaces (12-8)
	assert.Contains(t, got, "dddd: 4444  # m5 (5h)")  // 2 spaces (12-10)
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
