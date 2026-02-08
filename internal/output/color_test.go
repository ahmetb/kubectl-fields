package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorManager_HashBased_SameManagerSameColor(t *testing.T) {
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

func TestColorManager_HashBased_CrossInvocationConsistency(t *testing.T) {
	// Two independent ColorManagers should assign the same color
	// to the same manager name (hash-based, not insertion-order)
	cm1 := NewColorManager()
	cm2 := NewColorManager()

	// Encounter managers in different order
	cm1.ColorFor("kubectl-apply")
	cm1.ColorFor("helm")
	c1 := cm1.ColorFor("kube-controller-manager")

	cm2.ColorFor("kube-controller-manager") // encountered first in cm2
	c2 := cm2.ColorFor("kube-controller-manager")

	assert.Equal(t, c1, c2, "same manager should get same color regardless of encounter order")
}

func TestColorManager_HashBased_DifferentManagersDifferentColors(t *testing.T) {
	cm := NewColorManager()

	// Most distinct managers should get different colors (not guaranteed
	// since hash collisions are possible, but these common names don't collide)
	colors := make(map[string]bool)
	names := []string{"kubectl-apply", "helm", "kube-controller-manager", "argocd"}
	for _, name := range names {
		colors[cm.ColorFor(name)] = true
	}
	// With 4 names and 8 palette entries, collisions are unlikely
	assert.GreaterOrEqual(t, len(colors), 2, "different managers should generally get different colors")
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
