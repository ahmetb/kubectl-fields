package managed

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func parseTestFile(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Equal(t, yaml.DocumentNode, doc.Kind)
	return doc.Content[0]
}

func TestExtractManagedFields_Deployment(t *testing.T) {
	root := parseTestFile(t, "../../testdata/1_deployment.yaml")

	entries, err := ExtractManagedFields(root)
	require.NoError(t, err)
	require.Len(t, entries, 4)

	// Entry 0: kubectl-client-side-apply
	assert.Equal(t, "kubectl-client-side-apply", entries[0].Manager)
	assert.Equal(t, "Update", entries[0].Operation)
	assert.Equal(t, "", entries[0].Subresource)
	assert.Equal(t, "apps/v1", entries[0].APIVersion)
	assert.False(t, entries[0].Time.IsZero())

	// Entry 1: envpatcher
	assert.Equal(t, "envpatcher", entries[1].Manager)
	assert.Equal(t, "Update", entries[1].Operation)

	// Entry 2: kube-controller-manager with subresource
	assert.Equal(t, "kube-controller-manager", entries[2].Manager)
	assert.Equal(t, "Update", entries[2].Operation)
	assert.Equal(t, "status", entries[2].Subresource)

	// Entry 3: finalizerpatcher
	assert.Equal(t, "finalizerpatcher", entries[3].Manager)

	// All entries have non-zero time
	for i, e := range entries {
		assert.False(t, e.Time.IsZero(), "entry %d has zero time", i)
	}

	// Entry 0 has FieldsV1 as a MappingNode
	require.NotNil(t, entries[0].FieldsV1)
	assert.Equal(t, yaml.MappingNode, entries[0].FieldsV1.Kind)
}

func TestExtractManagedFields_NoMetadata(t *testing.T) {
	// Minimal YAML with no metadata key
	data := []byte("apiVersion: v1\nkind: ConfigMap\ndata:\n  key: value\n")
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	root := doc.Content[0]

	entries, err := ExtractManagedFields(root)
	require.NoError(t, err)
	assert.Nil(t, entries)
}

func TestExtractManagedFields_NoManagedFields(t *testing.T) {
	root := parseTestFile(t, "../../testdata/0_no_managedFields.yaml")

	entries, err := ExtractManagedFields(root)
	require.NoError(t, err)
	assert.Nil(t, entries)
}
