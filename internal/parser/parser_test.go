package parser

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// ---------------------------------------------------------------------------
// ParseDocuments tests
// ---------------------------------------------------------------------------

func TestParseDocuments_SingleDoc(t *testing.T) {
	input := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n"
	docs, err := ParseDocuments(strings.NewReader(input))
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, yaml.DocumentNode, docs[0].Kind)
	assert.Equal(t, yaml.MappingNode, docs[0].Content[0].Kind)
}

func TestParseDocuments_MultiDoc(t *testing.T) {
	data, err := os.ReadFile("../../testdata/roundtrip/multidoc.yaml")
	require.NoError(t, err)

	docs, err := ParseDocuments(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	// Verify both are documents with mapping roots.
	for i, doc := range docs {
		assert.Equal(t, yaml.DocumentNode, doc.Kind, "doc %d should be DocumentNode", i)
		assert.Equal(t, yaml.MappingNode, doc.Content[0].Kind, "doc %d root should be MappingNode", i)
	}
}

func TestParseDocuments_EmptyInput(t *testing.T) {
	docs, err := ParseDocuments(strings.NewReader(""))
	require.NoError(t, err)
	assert.Len(t, docs, 0)
}

func TestParseDocuments_InvalidYAML(t *testing.T) {
	_, err := ParseDocuments(strings.NewReader(":\n  - :\n    -"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "YAML parse error")
}

// ---------------------------------------------------------------------------
// UnwrapListKind tests
// ---------------------------------------------------------------------------

func TestUnwrapListKind_NotAList(t *testing.T) {
	input := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n"
	docs, err := ParseDocuments(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	result := UnwrapListKind(docs[0])
	assert.Len(t, result, 1)
	// Should be the same document object (pointer equality).
	assert.Same(t, docs[0], result[0])
}

func TestUnwrapListKind_ListWithItems(t *testing.T) {
	data, err := os.ReadFile("../../testdata/roundtrip/list_kind.yaml")
	require.NoError(t, err)

	docs, err := ParseDocuments(bytes.NewReader(data))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	unwrapped := UnwrapListKind(docs[0])
	assert.Len(t, unwrapped, 2)

	// Each unwrapped item should be a DocumentNode wrapping a MappingNode.
	for i, doc := range unwrapped {
		assert.Equal(t, yaml.DocumentNode, doc.Kind, "unwrapped item %d should be DocumentNode", i)
		require.NotEmpty(t, doc.Content, "unwrapped item %d should have content", i)
		assert.Equal(t, yaml.MappingNode, doc.Content[0].Kind, "unwrapped item %d root should be MappingNode", i)
	}

	// Verify first item is cm-one.
	name1, ok := getMapValue(unwrapped[0].Content[0], "kind")
	require.True(t, ok)
	assert.Equal(t, "ConfigMap", name1)

	// Verify second item is cm-two by checking metadata.name.
	metaNode, ok := getMapValueNode(unwrapped[1].Content[0], "metadata")
	require.True(t, ok)
	nameVal, ok := getMapValue(metaNode, "name")
	require.True(t, ok)
	assert.Equal(t, "cm-two", nameVal)
}

// ---------------------------------------------------------------------------
// Round-trip fidelity tests
// ---------------------------------------------------------------------------

func roundTripTest(t *testing.T, fixturePath string) {
	t.Helper()

	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "failed to read fixture: %s", fixturePath)

	docs, err := ParseDocuments(bytes.NewReader(data))
	require.NoError(t, err, "ParseDocuments failed for %s", fixturePath)

	var buf bytes.Buffer
	err = EncodeDocuments(&buf, docs)
	require.NoError(t, err, "EncodeDocuments failed for %s", fixturePath)

	expected := strings.TrimRight(string(data), "\n")
	actual := strings.TrimRight(buf.String(), "\n")

	assert.Equal(t, expected, actual,
		"round-trip fidelity failed for %s", fixturePath)
}

func TestRoundTrip_Deployment(t *testing.T) {
	// This is the #1 validation test for the entire phase.
	// If this fails, the round-trip approach needs rethinking.
	roundTripTest(t, "../../testdata/roundtrip/deployment.yaml")
}

func TestRoundTrip_ConfigMap(t *testing.T) {
	// Validates that quoted values like "yes", "true", "null" survive round-trip.
	// Also validates literal block scalar (|) preservation and flow-style empty map.
	roundTripTest(t, "../../testdata/roundtrip/configmap.yaml")
}

func TestRoundTrip_Service(t *testing.T) {
	// Validates compact sequence indent for the ports list.
	roundTripTest(t, "../../testdata/roundtrip/service.yaml")
}

func TestRoundTrip_MultiDoc(t *testing.T) {
	// Validates that --- separators are preserved between documents.
	roundTripTest(t, "../../testdata/roundtrip/multidoc.yaml")
}

func TestRoundTrip_ListKind(t *testing.T) {
	// Read the list_kind.yaml, parse it, unwrap the List, encode the
	// unwrapped items, and verify the output has 2 separate documents.
	data, err := os.ReadFile("../../testdata/roundtrip/list_kind.yaml")
	require.NoError(t, err)

	docs, err := ParseDocuments(bytes.NewReader(data))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	var allDocs []*yaml.Node
	for _, doc := range docs {
		allDocs = append(allDocs, UnwrapListKind(doc)...)
	}
	assert.Len(t, allDocs, 2)

	var buf bytes.Buffer
	err = EncodeDocuments(&buf, allDocs)
	require.NoError(t, err)

	output := strings.TrimRight(buf.String(), "\n")

	// Verify it contains two documents.
	docParts := strings.Split(output, "---\n")
	// The first document has no leading ---, subsequent ones do.
	// So splitting by "---\n" gives: [first-doc, second-doc].
	assert.Len(t, docParts, 2, "expected 2 documents in output after List unwrap")

	// Verify first document contains cm-one.
	assert.Contains(t, docParts[0], "name: cm-one")

	// Verify second document contains cm-two.
	assert.Contains(t, docParts[1], "name: cm-two")
}
