package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStripManagedFields(t *testing.T) {
	r := yamlNode(t, []byte(exampleObj))
	stripManagedFields(r.Content[0])

	var b bytes.Buffer
	require.NoError(t, yaml.NewEncoder(&b).Encode(r.Content[0]))
	require.NotContains(t, b.String(), "managedFields")
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
			node:     yamlNode(t, []byte("[1,2,3]")),
			errcheck: require.Error,
			errMsg:   "invalid document structure",
		},
		{
			name:     "List object",
			node:     yamlNode(t, []byte("apiVersion: v1\nkind: List\nitems: []")),
			errcheck: require.Error,
			errMsg:   "List object",
		},
		{
			name:     "no metadata",
			node:     yamlNode(t, []byte("apiVersion: v1")),
			errcheck: require.Error,
			errMsg:   "metadata not found",
		},
		{
			name:     "no managedFields",
			node:     yamlNode(t, []byte("metadata: {}")),
			errcheck: require.Error,
			errMsg:   "managedFields not found",
		},
		{
			name:     "valid object",
			node:     yamlNode(t, []byte(exampleObj)),
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

func yamlNode(t *testing.T, b []byte) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(b), &doc))
	return &doc
}
