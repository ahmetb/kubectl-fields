package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStripManagedFields(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := yamlDoc(t, []byte(exampleObj))
		require.True(t, stripManagedFields(r.Content[0]))

		var b bytes.Buffer
		require.NoError(t, yaml.NewEncoder(&b).Encode(r.Content[0]))
		require.NotContains(t, b.String(), "managedFields")
	})
	t.Run("no metadata", func(t *testing.T) {
		r := yamlDoc(t, []byte("apiVersion: v1"))
		require.False(t, stripManagedFields(r.Content[0]))
	})

	t.Run("no managedFields", func(t *testing.T) {
		r := yamlDoc(t, []byte(`metadata: {"a": "b"}`))
		require.False(t, stripManagedFields(r.Content[0]))
	})
}

func TestValidateDocumentIsSingleKubernetesObject(t *testing.T) {
	cases := []struct {
		name     string
		node     *yaml.Node
		errcheck require.ErrorAssertionFunc
		errMsg   string
	}{
		{
			name:     "doc is not object",
			node:     &yaml.Node{Kind: yaml.SequenceNode},
			errcheck: require.Error,
			errMsg:   "not a YAML document",
		},
		{
			name:     "root object is not mappingNode",
			node:     yamlDoc(t, []byte("[1,2,3]")),
			errcheck: require.Error,
			errMsg:   "invalid document structure",
		},
		{
			name:     "List object",
			node:     yamlDoc(t, []byte("apiVersion: v1\nkind: List\nitems: []")),
			errcheck: require.Error,
			errMsg:   "List object",
		},
		{
			name:     "no metadata",
			node:     yamlDoc(t, []byte("apiVersion: v1")),
			errcheck: require.Error,
			errMsg:   "metadata not found",
		},
		{
			name:     "no managedFields",
			node:     yamlDoc(t, []byte("metadata: {}")),
			errcheck: require.Error,
			errMsg:   "managedFields not found",
		},
		{
			name:     "valid object",
			node:     yamlDoc(t, []byte(exampleObj)),
			errcheck: require.NoError,
			errMsg:   "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDocumentIsSingleKubernetesObject(c.node)
			if c.errMsg != "" {
				require.ErrorContains(t, err, c.errMsg)
			}
		})
	}
}

func TestGetMapValueNode(t *testing.T) {
	in := `k1: v1
k2: {}`
	doc := yamlDoc(t, []byte(in))
	root := doc.Content[0]

	k1, ok := getMapValueNode(root, "k1")
	require.True(t, ok)
	assert.Equal(t, "v1", k1.Value)

	k2, ok := getMapValueNode(root, "k2")
	require.True(t, ok)
	assert.Equal(t, yaml.MappingNode, k2.Kind)

	_, ok = getMapValueNode(root, "k3")
	require.False(t, ok)
}

func TestGetMapValue(t *testing.T) {
	in := `k1: v1
k2: [1,2,3]`
	doc := yamlDoc(t, []byte(in))
	root := doc.Content[0]

	v1, ok := getMapValue(root, "k1")
	require.True(t, ok)
	assert.Equal(t, "v1", v1)

	_, ok = getMapValue(root, "k2")
	require.False(t, ok)

	_, ok = getMapValue(root, "k3")
	require.False(t, ok)
}

func TestMappingNodeAsMap(t *testing.T) {
	t.Run("not a mapping node", func(t *testing.T) {
		n := yamlDoc(t, []byte(`[1,2,3]`))
		_, err := mappingNodeAsMap(n.Content[0])
		require.Error(t, err)
	})
	t.Run("valid map", func(t *testing.T) {
		n := yamlDoc(t, []byte(`{"a": {"arr": [1,2,3]}, "b": "valB", "c": null, "d": true}`))
		got, err := mappingNodeAsMap(n.Content[0])
		require.NoError(t, err)
		expected := map[string]any{
			"a": map[string]any{
				"arr": []any{1, 2, 3},
			},
			"b": "valB",
			"c": nil,
			"d": true,
		}
		assert.Equal(t, expected, got)
	})
}

func yamlDoc(t *testing.T, b []byte) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(b), &doc))
	return &doc
}
