package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTimeFmt(t *testing.T) {
	cases := []struct {
		offset   time.Duration
		expected string
	}{
		{0, "0s ago"},
		{time.Second, "1s ago"},
		{time.Second*70 + time.Millisecond*999, "1m10s ago"},
		{time.Minute*70 + time.Second, "1h10m ago"},
		{time.Hour*25 + time.Minute*31, "1d1h ago"},
		{time.Hour*24*365*2 + time.Hour*73, "2yr3d ago"},
	}
	for _, v := range cases {
		t.Run(fmt.Sprintf("offset %s", v.offset), func(t *testing.T) {
			assert.Equal(t, v.expected, timeFmt(time.Now().Add(-v.offset)))
		})
	}
}

func TestAnnotation(t *testing.T) {
	assert.Equal(t, "test", annotation(managerEntry{Name: "test"}))
	assert.Equal(t, "test (/scale)", annotation(managerEntry{Name: "test", Subresource: "scale"}))
	assert.Equal(t, "test (/scale) (1h ago)", annotation(managerEntry{Name: "test", Subresource: "scale", Time: time.Now().Add(-time.Hour)}))
}

func TestAnnotateYAMLNode(t *testing.T) {
	doc := yamlDoc(t, []byte(`field1: value1
field2:
    - 1
    - 2
field3Above:
  field3field1: null`))
	node := doc.Content[0]

	annotateYAMLNode(node.Content[0], &managedField{Manager: managerEntry{Name: "field1mgr"}}, annotationOptions{})
	annotateYAMLNode(node.Content[2], &managedField{Manager: managerEntry{Name: "field2mgr"}}, annotationOptions{})
	annotateYAMLNode(node.Content[4], &managedField{Manager: managerEntry{Name: "field3mgr"}}, annotationOptions{position: Above})
	annotateYAMLNode(node.Content[5].Content[0], &managedField{Manager: managerEntry{Name: "field3field1mgr"}}, annotationOptions{})

	var b bytes.Buffer
	require.NoError(t, yaml.NewEncoder(&b).Encode(doc.Content[0]))

	expected := `field1: value1 # field1mgr
field2: # field2mgr
    - 1
    - 2
# field3mgr
field3Above:
    field3field1: null # field3field1mgr
`

	require.Equal(t, expected, b.String())

}
