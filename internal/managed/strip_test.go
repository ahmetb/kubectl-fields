package managed

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func encodeNode(t *testing.T, doc *yaml.Node) string {
	t.Helper()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	enc.CompactSeqIndent()
	require.NoError(t, enc.Encode(doc))
	require.NoError(t, enc.Close())
	return buf.String()
}

func TestStripManagedFields_Deployment(t *testing.T) {
	data, err := os.ReadFile("../../testdata/1_deployment.yaml")
	require.NoError(t, err)

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	root := doc.Content[0]

	// Verify managedFields exists before stripping
	assert.True(t, strings.Contains(string(data), "managedFields"))

	removed := StripManagedFields(root)
	assert.True(t, removed, "StripManagedFields should return true when managedFields exists")

	// Re-encode and verify managedFields is gone
	output := encodeNode(t, &doc)
	assert.False(t, strings.Contains(output, "managedFields"), "output should not contain managedFields")

	// Verify metadata key still exists with remaining fields
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "name: nginx-deployment")
	assert.Contains(t, output, "namespace: default")
	assert.Contains(t, output, "app: nginx")

	// Verify spec and status sections are unchanged
	assert.Contains(t, output, "spec:")
	assert.Contains(t, output, "replicas: 3")
	assert.Contains(t, output, "status:")
	assert.Contains(t, output, "availableReplicas: 3")
}

func TestStripManagedFields_NoManagedFields(t *testing.T) {
	data, err := os.ReadFile("../../testdata/0_no_managedFields.yaml")
	require.NoError(t, err)

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	root := doc.Content[0]

	removed := StripManagedFields(root)
	assert.False(t, removed, "StripManagedFields should return false when no managedFields")

	// Output should be unchanged
	output := encodeNode(t, &doc)
	// Re-parse original for comparison
	var origDoc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &origDoc))
	origOutput := encodeNode(t, &origDoc)
	assert.Equal(t, origOutput, output, "output should equal input when no managedFields to strip")
}

func TestStripManagedFields_RoundTripPreservation(t *testing.T) {
	data, err := os.ReadFile("../../testdata/1_deployment.yaml")
	require.NoError(t, err)

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	root := doc.Content[0]

	StripManagedFields(root)

	// Re-encode
	output := encodeNode(t, &doc)

	// Verify output is valid YAML (re-parseable)
	var reparsed yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(output), &reparsed), "output must be valid YAML")

	// Verify key preserved fields
	// Annotation with literal block scalar (|)
	assert.Contains(t, output, "kubectl.kubernetes.io/last-applied-configuration: |")

	// Quoted timestamp values
	assert.Contains(t, output, `"2024-04-10T00:34:50Z"`)

	// Compact sequence indentation for containers/ports
	assert.Contains(t, output, "containers:\n      - env:")
	assert.Contains(t, output, "ports:\n        - containerPort: 80")

	// All non-managedFields metadata fields preserved
	assert.Contains(t, output, `deployment.kubernetes.io/revision: "2"`)
	assert.Contains(t, output, "generation: 2")
	assert.Contains(t, output, "resourceVersion:")
	assert.Contains(t, output, "uid:")
	assert.Contains(t, output, "finalizers:")
	assert.Contains(t, output, "- example.com/foo")
}
