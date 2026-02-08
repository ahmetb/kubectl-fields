package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorManager_InsertionOrder(t *testing.T) {
	cm := NewColorManager()

	// First manager gets index 0
	c1 := cm.ColorFor("kubectl-apply")
	assert.Equal(t, BrightPalette[0], c1)

	// Second manager gets index 1
	c2 := cm.ColorFor("helm")
	assert.Equal(t, BrightPalette[1], c2)

	// Same manager returns same color
	c1again := cm.ColorFor("kubectl-apply")
	assert.Equal(t, c1, c1again)

	// Third manager gets index 2
	c3 := cm.ColorFor("kube-controller-manager")
	assert.Equal(t, BrightPalette[2], c3)
}

func TestColorManager_CyclesPalette(t *testing.T) {
	cm := NewColorManager()

	// Assign all 8 palette colors
	names := []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8"}
	for i, name := range names {
		c := cm.ColorFor(name)
		assert.Equal(t, BrightPalette[i], c)
	}

	// 9th manager cycles back to index 0
	c9 := cm.ColorFor("m9")
	assert.Equal(t, BrightPalette[0], c9)
}

func TestColorManager_Wrap(t *testing.T) {
	cm := NewColorManager()
	wrapped := cm.Wrap("# kubectl-apply (5d ago)", "kubectl-apply")
	expected := BrightPalette[0] + "# kubectl-apply (5d ago)" + Reset
	assert.Equal(t, expected, wrapped)
}

func TestExtractManagerName(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "with age",
			comment:  "manager (5d ago)",
			expected: "manager",
		},
		{
			name:     "with subresource and age",
			comment:  "manager /status (5d ago)",
			expected: "manager",
		},
		{
			name:     "with subresource only (hide mode)",
			comment:  "manager /status",
			expected: "manager",
		},
		{
			name:     "manager only (hide mode, no sub)",
			comment:  "manager",
			expected: "manager",
		},
		{
			name:     "with hash prefix",
			comment:  "# manager (5d ago)",
			expected: "manager",
		},
		{
			name:     "with hash prefix and subresource",
			comment:  "# kube-controller-manager /status (1h ago)",
			expected: "kube-controller-manager",
		},
		{
			name:     "complex manager name with dashes",
			comment:  "kubectl-client-side-apply (50m ago)",
			expected: "kubectl-client-side-apply",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractManagerName(tc.comment)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestResolveColor_Always(t *testing.T) {
	assert.True(t, ResolveColor("always", false))
	assert.True(t, ResolveColor("always", true))
}

func TestResolveColor_Never(t *testing.T) {
	assert.False(t, ResolveColor("never", false))
	assert.False(t, ResolveColor("never", true))
}

func TestResolveColor_AutoTTY(t *testing.T) {
	// Ensure NO_COLOR is not set
	t.Setenv("NO_COLOR", "")
	assert.True(t, ResolveColor("auto", true))
	assert.False(t, ResolveColor("auto", false))
}

func TestResolveColor_AutoNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	assert.False(t, ResolveColor("auto", true))
	assert.False(t, ResolveColor("auto", false))
}

func TestResolveColor_AutoNoColorEmpty(t *testing.T) {
	// Empty NO_COLOR should NOT disable color
	t.Setenv("NO_COLOR", "")
	assert.True(t, ResolveColor("auto", true))
}
