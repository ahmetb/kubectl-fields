package annotate

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"github.com/rewanthtammana/kubectl-fields/internal/parser"
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
	got := formatComment(info, testNow, MtimeRelative, false)
	assert.Equal(t, "kubectl-client-side-apply (50m ago)", got)
}

func TestFormatComment_WithSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager:     "kube-controller-manager",
		Subresource: "status",
		Time:        testNow.Add(-1 * time.Hour),
	}
	got := formatComment(info, testNow, MtimeRelative, false)
	// New format: space + slash, no parentheses around subresource
	assert.Equal(t, "kube-controller-manager /status (1h ago)", got)
}

func TestFormatComment_NoSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager: "helm",
		Time:    testNow.Add(-3 * time.Hour),
	}
	got := formatComment(info, testNow, MtimeRelative, false)
	assert.Equal(t, "helm (3h ago)", got)
	assert.NotContains(t, got, "/")
}

func TestFormatComment_AbsoluteMode(t *testing.T) {
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	info := AnnotationInfo{
		Manager: "kubectl-apply",
		Time:    ts,
	}
	got := formatComment(info, testNow, MtimeAbsolute, false)
	assert.Equal(t, "kubectl-apply (2026-02-07T12:00:00Z)", got)
}

func TestFormatComment_AbsoluteModeWithSubresource(t *testing.T) {
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	info := AnnotationInfo{
		Manager:     "kube-controller-manager",
		Subresource: "status",
		Time:        ts,
	}
	got := formatComment(info, testNow, MtimeAbsolute, false)
	assert.Equal(t, "kube-controller-manager /status (2026-02-07T12:00:00Z)", got)
}

func TestFormatComment_HideMode(t *testing.T) {
	info := AnnotationInfo{
		Manager: "kubectl-apply",
		Time:    testNow.Add(-5 * time.Minute),
	}
	got := formatComment(info, testNow, MtimeHide, false)
	assert.Equal(t, "kubectl-apply", got)
}

func TestFormatComment_HideModeWithSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager:     "kube-controller-manager",
		Subresource: "status",
		Time:        testNow.Add(-5 * time.Minute),
	}
	got := formatComment(info, testNow, MtimeHide, false)
	assert.Equal(t, "kube-controller-manager /status", got)
}

func TestFormatComment_EmptyMtimeDefaultsToRelative(t *testing.T) {
	info := AnnotationInfo{
		Manager: "helm",
		Time:    testNow.Add(-2 * time.Hour),
	}
	// Empty string for mtime should behave as relative
	got := formatComment(info, testNow, "", false)
	assert.Equal(t, "helm (2h ago)", got)
}

// --- formatComment showOperation tests ---

func TestFormatComment_ShowOperation_Relative(t *testing.T) {
	info := AnnotationInfo{
		Manager:   "kubectl-apply",
		Operation: "Update",
		Time:      testNow.Add(-50 * time.Minute),
	}
	got := formatComment(info, testNow, MtimeRelative, true)
	assert.Equal(t, "kubectl-apply (50m ago, update)", got)
}

func TestFormatComment_ShowOperation_Absolute(t *testing.T) {
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	info := AnnotationInfo{
		Manager:   "kubectl-apply",
		Operation: "Apply",
		Time:      ts,
	}
	got := formatComment(info, testNow, MtimeAbsolute, true)
	assert.Equal(t, "kubectl-apply (2026-02-07T12:00:00Z, apply)", got)
}

func TestFormatComment_ShowOperation_Hide(t *testing.T) {
	info := AnnotationInfo{
		Manager:   "kubectl-apply",
		Operation: "Update",
		Time:      testNow,
	}
	got := formatComment(info, testNow, MtimeHide, true)
	assert.Equal(t, "kubectl-apply (update)", got)
}

func TestFormatComment_ShowOperation_WithSubresource(t *testing.T) {
	info := AnnotationInfo{
		Manager:     "kube-controller-manager",
		Operation:   "Update",
		Subresource: "status",
		Time:        testNow.Add(-1 * time.Hour),
	}
	got := formatComment(info, testNow, MtimeRelative, true)
	assert.Equal(t, "kube-controller-manager /status (1h ago, update)", got)
}

func TestFormatComment_ShowOperation_EmptyOperation(t *testing.T) {
	info := AnnotationInfo{
		Manager:   "kubectl-apply",
		Operation: "",
		Time:      testNow.Add(-50 * time.Minute),
	}
	got := formatComment(info, testNow, MtimeRelative, true)
	// Empty operation should produce same output as showOperation=false
	assert.Equal(t, "kubectl-apply (50m ago)", got)
}

func TestFormatComment_ShowOperation_False(t *testing.T) {
	info := AnnotationInfo{
		Manager:   "kubectl-apply",
		Operation: "Update",
		Time:      testNow.Add(-50 * time.Minute),
	}
	got := formatComment(info, testNow, MtimeRelative, false)
	// showOperation=false must produce byte-identical output to existing behavior
	assert.Equal(t, "kubectl-apply (50m ago)", got)
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

	// New format: space + slash, no parentheses around subresource
	assert.Contains(t, output, "/status")
	assert.Contains(t, output, "kube-controller-manager /status (1h ago)")
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

// --- k: (associative key) annotation tests ---

func TestAnnotate_InlineListItemByKey(t *testing.T) {
	// containers sequence with one item containing name and image
	root := parseYAML(t, `containers:
- name: nginx
  image: nginx:1.14
`)

	entries := []managed.ManagedFieldsEntry{
		{
			Manager: "kubectl-apply",
			Time:    testNow.Add(-30 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{
				"f:containers": {
					"k:{\"name\":\"nginx\"}": {
						".": {},
						"f:image": {}
					}
				}
			}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	// k: item dot marker: HeadComment on first key renders as "- # comment"
	assert.Contains(t, output, "# kubectl-apply (30m ago)")
	// f:image within the item should have inline comment
	assert.Contains(t, output, "image: nginx:1.14 # kubectl-apply (30m ago)")
}

func TestAnnotate_InlineSetValue(t *testing.T) {
	// finalizers sequence with one scalar value
	root := parseYAML(t, `finalizers:
- example.com/foo
`)

	entries := []managed.ManagedFieldsEntry{
		{
			Manager: "finalizerpatcher",
			Time:    testNow.Add(-10 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{
				"f:finalizers": {
					".": {},
					"v:\"example.com/foo\"": {}
				}
			}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	// v: set value inline: LineComment on the scalar
	assert.Contains(t, output, "- example.com/foo # finalizerpatcher (10m ago)")
	// Container dot marker on finalizers key
	assert.Contains(t, output, "finalizers: # finalizerpatcher (10m ago)")
}

func TestAnnotate_AboveListItemByKey(t *testing.T) {
	root := parseYAML(t, `containers:
- name: nginx
  image: nginx:1.14
`)

	entries := []managed.ManagedFieldsEntry{
		{
			Manager: "kubectl-apply",
			Time:    testNow.Add(-30 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{
				"f:containers": {
					"k:{\"name\":\"nginx\"}": {
						".": {},
						"f:image": {}
					}
				}
			}`),
		},
	}

	Annotate(root, entries, Options{Above: true, Now: testNow})
	output := encodeYAML(t, root)

	// Above mode: HeadComment before the item (on the MappingNode)
	assert.Contains(t, output, "# kubectl-apply (30m ago)")
	// The comment should appear before the item's first field
	lines := strings.Split(output, "\n")
	foundItemComment := false
	foundImageComment := false
	for i, line := range lines {
		if strings.Contains(line, "# kubectl-apply") && i+1 < len(lines) {
			nextLine := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(nextLine, "- ") || strings.HasPrefix(nextLine, "name:") {
				foundItemComment = true
			}
			if strings.HasPrefix(nextLine, "image:") {
				foundImageComment = true
			}
		}
	}
	assert.True(t, foundItemComment, "k: item should have above comment")
	assert.True(t, foundImageComment, "image field should have above comment")
}

// --- MtimeMode integration tests ---

func TestAnnotate_MtimeAbsolute(t *testing.T) {
	root := parseYAML(t, "replicas: 3\n")

	fieldTime := time.Date(2026, 2, 7, 12, 30, 0, 0, time.UTC)
	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     fieldTime,
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow, Mtime: MtimeAbsolute})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "replicas: 3 # kubectl-apply (2026-02-07T12:30:00Z)")
}

func TestAnnotate_MtimeHide(t *testing.T) {
	root := parseYAML(t, "replicas: 3\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-5 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
	}

	Annotate(root, entries, Options{Now: testNow, Mtime: MtimeHide})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "replicas: 3 # kubectl-apply")
	// Should not contain any age or timestamp in parentheses
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "replicas") {
			assert.NotContains(t, line, "(")
		}
	}
}

func TestAnnotate_MtimeEmptyDefaultsRelative(t *testing.T) {
	root := parseYAML(t, "replicas: 3\n")

	entries := []managed.ManagedFieldsEntry{
		{
			Manager:  "kubectl-apply",
			Time:     testNow.Add(-10 * time.Minute),
			FieldsV1: buildFieldsV1(t, `{"f:replicas":{}}`),
		},
	}

	// Empty Mtime should default to relative
	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	assert.Contains(t, output, "replicas: 3 # kubectl-apply (10m ago)")
}

// --- Golden file tests ---

// processDeploymentFixture reads the deployment YAML, parses, extracts managedFields,
// annotates, strips managedFields, and encodes. Returns the output string.
func processDeploymentFixture(t *testing.T, above bool, showOperation bool, fixedNow time.Time) string {
	t.Helper()

	inputData, err := os.ReadFile("../../testdata/1_deployment.yaml")
	require.NoError(t, err, "reading deployment fixture")

	docs, err := parser.ParseDocuments(bytes.NewReader(inputData))
	require.NoError(t, err, "parsing deployment fixture")
	require.Len(t, docs, 1, "expected 1 document")

	doc := docs[0]
	require.Equal(t, yaml.DocumentNode, doc.Kind)
	require.NotEmpty(t, doc.Content)
	root := doc.Content[0]

	entries, err := managed.ExtractManagedFields(root)
	require.NoError(t, err, "extracting managedFields")
	require.NotEmpty(t, entries, "expected managedFields entries")

	Annotate(root, entries, Options{
		Above:         above,
		Now:           fixedNow,
		ShowOperation: showOperation,
	})

	managed.StripManagedFields(root)

	var buf bytes.Buffer
	err = parser.EncodeDocuments(&buf, []*yaml.Node{doc})
	require.NoError(t, err, "encoding annotated YAML")

	return buf.String()
}

var updateGolden = os.Getenv("UPDATE_GOLDEN") != ""

func TestAnnotate_GoldenInline(t *testing.T) {
	// fixedNow chosen so timestamps match the golden file:
	// kubectl-client-side-apply: 2024-04-10T00:44:50Z -> 50m ago
	// envpatcher/kube-controller-manager: 2024-04-10T00:34:50Z -> 1h ago
	// finalizerpatcher: 2024-04-10T00:35:29Z -> 59m21s ago
	fixedNow := time.Date(2024, 4, 10, 1, 34, 50, 0, time.UTC)

	got := processDeploymentFixture(t, false, false, fixedNow)

	goldenPath := "../../testdata/1_deployment_inline.out"
	if updateGolden {
		err := os.WriteFile(goldenPath, []byte(got), 0644)
		require.NoError(t, err, "updating inline golden file")
		t.Log("Updated inline golden file")
		return
	}

	expectedData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading inline golden file")
	expected := string(expectedData)

	assert.Equal(t, expected, got, "inline golden file mismatch")
}

func TestAnnotate_GoldenAbove(t *testing.T) {
	// fixedNow chosen so timestamps match the golden file:
	// kubectl-client-side-apply: 2024-04-10T00:44:50Z -> 16h55m ago
	// envpatcher/kube-controller-manager: 2024-04-10T00:34:50Z -> 17h5m ago
	// finalizerpatcher: 2024-04-10T00:35:29Z -> 17h4m ago (seconds dropped in hours range)
	fixedNow := time.Date(2024, 4, 10, 17, 39, 50, 0, time.UTC)

	got := processDeploymentFixture(t, true, false, fixedNow)

	goldenPath := "../../testdata/1_deployment_above.out"
	if updateGolden {
		err := os.WriteFile(goldenPath, []byte(got), 0644)
		require.NoError(t, err, "updating above golden file")
		t.Log("Updated above golden file")
		return
	}

	expectedData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading above golden file")
	expected := string(expectedData)

	assert.Equal(t, expected, got, "above golden file mismatch")
}

func TestAnnotate_GoldenInlineOperation(t *testing.T) {
	// Same fixedNow as TestAnnotate_GoldenInline
	fixedNow := time.Date(2024, 4, 10, 1, 34, 50, 0, time.UTC)

	got := processDeploymentFixture(t, false, true, fixedNow)

	goldenPath := "../../testdata/1_deployment_inline_operation.out"
	if updateGolden {
		err := os.WriteFile(goldenPath, []byte(got), 0644)
		require.NoError(t, err, "updating inline operation golden file")
		t.Log("Updated inline operation golden file")
		return
	}

	expectedData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading inline operation golden file")
	expected := string(expectedData)

	assert.Equal(t, expected, got, "inline operation golden file mismatch")
}

func TestAnnotate_GoldenAboveOperation(t *testing.T) {
	// Same fixedNow as TestAnnotate_GoldenAbove
	fixedNow := time.Date(2024, 4, 10, 17, 39, 50, 0, time.UTC)

	got := processDeploymentFixture(t, true, true, fixedNow)

	goldenPath := "../../testdata/1_deployment_above_operation.out"
	if updateGolden {
		err := os.WriteFile(goldenPath, []byte(got), 0644)
		require.NoError(t, err, "updating above operation golden file")
		t.Log("Updated above operation golden file")
		return
	}

	expectedData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading above operation golden file")
	expected := string(expectedData)

	assert.Equal(t, expected, got, "above operation golden file mismatch")
}

func TestAnnotate_NoManagedFields(t *testing.T) {
	root := parseYAML(t, `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value
`)

	entries := []managed.ManagedFieldsEntry{}

	Annotate(root, entries, Options{Now: testNow})
	output := encodeYAML(t, root)

	// No managedFields means no annotations at all
	assert.NotContains(t, output, "#", "no comments should be present when there are no managedFields")
	assert.Contains(t, output, "key: value", "original data should be preserved")
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
