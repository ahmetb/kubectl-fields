package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorManager_SameManagerSameColor(t *testing.T) {
	cm := NewColorManager()

	// Same manager always returns the same color
	c1 := cm.ColorFor("kubectl-apply")
	c2 := cm.ColorFor("kubectl-apply")
	assert.Equal(t, c1, c2)

	// Color is from the palette
	found := false
	for _, p := range BrightPalette {
		if p == c1 {
			found = true
			break
		}
	}
	assert.True(t, found, "color should be from BrightPalette")
}

func TestColorManager_RoundRobin_DistinctColors(t *testing.T) {
	cm := NewColorManager()

	// Each new manager gets the next color in the palette
	names := []string{
		"kubectl-create",
		"kubectl-rollout",
		"argocd-controller",
		"kubectl-client-side-apply",
		"kubectl-edit",
		"kube-controller-manager",
	}
	colors := make(map[string]bool)
	for _, name := range names {
		colors[cm.ColorFor(name)] = true
	}
	// With 6 names and 8 palette entries, round-robin guarantees all distinct
	assert.Equal(t, 6, len(colors), "6 managers should get 6 distinct colors with round-robin")
}

func TestColorManager_RoundRobin_WrapsAround(t *testing.T) {
	cm := NewColorManager()

	// Exhaust the palette and verify wrap-around
	for i := 0; i < len(BrightPalette); i++ {
		cm.ColorFor(string(rune('A' + i)))
	}
	// Next manager wraps to first palette color
	c := cm.ColorFor("overflow")
	assert.Equal(t, BrightPalette[0], c, "should wrap around to first palette color")
}

func TestColorManager_Wrap(t *testing.T) {
	cm := NewColorManager()
	wrapped := cm.Wrap("# kubectl-apply (5d ago)", "kubectl-apply")
	color := cm.ColorFor("kubectl-apply")
	expected := color + "# kubectl-apply (5d ago)" + Reset
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
		{
			name:     "with operation and age",
			comment:  "# manager (5d ago, apply)",
			expected: "manager",
		},
		{
			name:     "with subresource and operation",
			comment:  "# manager /status (1h ago, update)",
			expected: "manager",
		},
		{
			name:     "operation only (hide mode)",
			comment:  "# manager (apply)",
			expected: "manager",
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
