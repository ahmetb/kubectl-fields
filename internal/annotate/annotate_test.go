package annotate

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

var testNow = time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC)

// --- formatComment tests ---

func TestFormatComment_Basic(t *testing.T) {
	info := AnnotationInfo{
		Manager: "kubectl-client-side-apply",
		Time:    testNow.Add(-50 * time.Minute),
	}
	got := formatComment(info, testNow)
	assert.Equal(t, "kubectl-client-side-apply (50m ago)", got)
}

func TestFormatComment_WithSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager:     "kube-controller-manager",
		Subresource: "status",
		Time:        testNow.Add(-1 * time.Hour),
	}
	got := formatComment(info, testNow)
	assert.Equal(t, "kube-controller-manager (/status) (1h ago)", got)
}

func TestFormatComment_NoSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager: "helm",
		Time:    testNow.Add(-3 * time.Hour),
	}
	got := formatComment(info, testNow)
	assert.Equal(t, "helm (3h ago)", got)
	assert.NotContains(t, got, "/")
}

// --- Annotate integration tests ---

// parseYAML is a test helper that parses YAML text and returns the root MappingNode.
func parseYAML(t *testing.T, input string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	err := yaml.Unmarshal([]byte(input), &doc)
	require.NoError(t, err)
	require.Equal(t, yaml.DocumentNode, doc.Kind)
	require.NotEmpty(t, doc.Content)
	return doc.Content[0]
}

// encodeYAML encodes a MappingNode back to YAML text with kubectl-compatible formatting.
func encodeYAML(t *testing.T, root *yaml.Node) string {
	t.Helper()
	doc := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	enc.CompactSeqIndent()
	err := enc.Encode(doc)
	require.NoError(t, err)
	err = enc.Close()
	require.NoError(t, err)
	return buf.String()
}

func TestAnnotate_InlineSimpleFields(t *testing.T) {
	root := parseYAML(t, "replicas: 3\nimage: nginx\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-30 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{},"f:image":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "replicas: 3 # kubectl-apply (30m ago)")
	assert.Contains(t, output, "image: nginx # kubectl-apply (30m ago)")
}

func TestAnnotate_InlineContainerField(t *testing.T) {
	root := parseYAML(t, "labels:\n  app: nginx\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-edit",
			Time:     testNow.Add(-2 * time.Hour),
			FieldsV1: buildFieldsV1(t, `{"f:labels":{".":{},"f:app":{}}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	// Container field: comment on the key "labels:"
	assert.Contains(t, output, "labels: # kubectl-edit (2h ago)")
	// Scalar field: comment on the value
	assert.Contains(t, output, "app: nginx # kubectl-edit (2h ago)")
}

func TestAnnotate_AboveMode(t *testing.T) {
	root := parseYAML(t, "replicas: 3\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-10 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
	}

	Annotate(root, entries, Options{Above: true, Now: testNow})
	output := encodeYAML(t, root)

	// Above mode: HeadComment before the key line
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected at least 2 lines in output")
	assert.Contains(t, lines[0], "# kubectl-apply (10m ago)")
	assert.Contains(t, lines[1], "replicas: 3")
}

func TestAnnotate_UnmanagedFieldBare(t *testing.T) {
	root := parseYAML(t, "replicas: 3\nimage: nginx\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-5 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	// replicas should have comment
	assert.Contains(t, output, "replicas: 3 # kubectl-apply")
	// image should NOT have any comment
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "image:") {
			assert.NotContains(t, line, "#", "unmanaged field 'image' should have no comment")
		}
	}
}

func TestAnnotate_SubresourceInComment(t *testing.T) {
	root := parseYAML(t, "conditions: []\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:     "kube-controller-manager",
			Subresource: "status",
			Time:        testNow.Add(-1 * time.Hour),
			FieldsV1:    buildFieldsV1(t, `{"f:conditions":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "(/status)")
	assert.Contains(t, output, "kube-controller-manager (/status) (1h ago)")
}

func TestAnnotate_MultipleManagers(t *testing.T) {
	root := parseYAML(t, "replicas: 3\nimage: nginx\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-10 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
		{
			Manager:  "helm",
			Time:     testNow.Add(-2 * time.Hour),
			FieldsV1: buildFieldsV1(t, `{"f:image":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "replicas: 3 # kubectl-apply (10m ago)")
	assert.Contains(t, output, "image: nginx # helm (2h ago)")
}

func TestAnnotate_NilFieldsV1Skipped(t *testing.T) {
	root := parseYAML(t, "replicas: 3\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-10 * time.Minute),
			FieldsV1: nil, // no FieldsV1
		},
	}

	// Should not panic or add any comments
	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)
	assert.NotContains(t, output, "#")
}

func TestAnnotate_AboveContainerField(t *testing.T) {
	root := parseYAML(t, "labels:\n  app: nginx\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-edit",
			Time:     testNow.Add(-5 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:labels":{".":{},"f:app":{}}}`),
		},
	}

	Annotate(root, entries, Options{Above: true, Now: testNow})
	output := encodeYAML(t, root)

	// In above mode, labels key should have a HeadComment
	assert.Contains(t, output, "# kubectl-edit (5m ago)")
	// And app key should also have a HeadComment
	lines := strings.Split(output, "\n")
	foundLabelsComment := false
	foundAppComment := false
	for i, line := range lines {
		if strings.Contains(line, "# kubectl-edit") && i+1 < len(lines) {
			nextLine := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(nextLine, "labels:") {
				foundLabelsComment = true
			}
			if strings.HasPrefix(nextLine, "app:") {
				foundAppComment = true
			}
		}
	}
	assert.True(t, foundLabelsComment, "labels should have above comment")
	assert.True(t, foundAppComment, "app should have above comment")
}

// --- helpers ---

// buildFieldsV1 parses a JSON FieldsV1 string into a yaml.Node MappingNode
// suitable for use in ManagedFieldsEntry.FieldsV1.
func buildFieldsV1(t *testing.T, jsonStr string) *yaml.Node {
	t.Helper()
	var node yaml.Node
	err := yaml.Unmarshal([]byte(jsonStr), &node)
	require.NoError(t, err)
	require.Equal(t, yaml.DocumentNode, node.Kind)
	require.NotEmpty(t, node.Content)
	return node.Content[0]
}
