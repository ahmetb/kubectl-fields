package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v4/value"
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
	node := rootNode(t, yamlDoc(t, []byte(`
field1: value1
field2:
    - 1
    - 2
field3Above:
    field3field1: null`)))

	annotateYAMLNode(node.Content[0], &managedField{Manager: managerEntry{Name: "field1mgr"}}, annotationOptions{})
	annotateYAMLNode(node.Content[2], &managedField{Manager: managerEntry{Name: "field2mgr"}}, annotationOptions{})
	annotateYAMLNode(node.Content[4], &managedField{Manager: managerEntry{Name: "field3mgr"}}, annotationOptions{position: Above})
	annotateYAMLNode(node.Content[5].Content[0], &managedField{Manager: managerEntry{Name: "field3field1mgr"}}, annotationOptions{})

	var b bytes.Buffer
	require.NoError(t, yaml.NewEncoder(&b).Encode(node))

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

func TestFindValueAtIndex(t *testing.T) {
	t.Run("not list node", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`field1: value1`)))
		_, err := findValueAtIndex(n, 0)
		require.Error(t, err)
	})

	t.Run("various indices", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`["a", "b"]`)))
		v0, err := findValueAtIndex(n, 0)
		require.NoError(t, err)
		require.Equal(t, "a", v0.Value)

		v1, err := findValueAtIndex(n, 1)
		require.NoError(t, err)
		require.Equal(t, "b", v1.Value)

		_, err = findValueAtIndex(n, 2)
		require.Error(t, err)
	})
}

func TestFindValueNode(t *testing.T) {
	t.Run("not list node", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`field1: value1`)))
		_, err := findValueNode(n, value.NewValueInterface("foo"))
		require.Error(t, err)
	})
	t.Run("not list of scalars", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`
- foo: bar
- baz: quux`)))
		_, err := findValueNode(n, value.NewValueInterface("some-value"))
		require.Error(t, err)
	})

	n := rootNode(t, yamlDoc(t, []byte(`["a", "b"]`)))
	t.Run("unsupported lookup types", func(t *testing.T) {
		_, err := findValueNode(n, value.NewValueInterface(0))
		require.Error(t, err)
		_, err = findValueNode(n, value.NewValueInterface(true))
		require.Error(t, err)
		_, err = findValueNode(n, value.NewValueInterface(nil))
		require.Error(t, err)
	})

	t.Run("lookup", func(t *testing.T) {
		_, err := findValueNode(n, value.NewValueInterface("a"))
		require.NoError(t, err)
		_, err = findValueNode(n, value.NewValueInterface("b"))
		require.NoError(t, err)
		_, err = findValueNode(n, value.NewValueInterface("c"))
		require.Error(t, err)
	})
}

func TestFindFieldInMappingNode(t *testing.T) {
	t.Run("not mapping node", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`- foo: bar`)))
		_, err := findFieldInMappingNode(n, "foo", false)
		require.Error(t, err)
	})

	n := rootNode(t, yamlDoc(t, []byte(`foo: bar`)))
	t.Run("not found", func(t *testing.T) {
		_, err := findFieldInMappingNode(n, "baz", false)
		require.Error(t, err)
	})

	t.Run("found: not leaf", func(t *testing.T) {
		v, err := findFieldInMappingNode(n, "foo", false)
		require.NoError(t, err)
		require.Equal(t, "bar", v.Value)
	})
	t.Run("found: leaf", func(t *testing.T) {
		v, err := findFieldInMappingNode(n, "foo", true)
		require.NoError(t, err)
		require.Equal(t, "foo", v.Value)
	})
}

func TestFindAssociativeListNode(t *testing.T) {
	t.Run("not list node", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`foo: bar`)))
		_, err := findAssociativeListNode(n, value.FieldList{{Name: "foo", Value: value.NewValueInterface("bar")}})
		require.Error(t, err)
	})
	t.Run("list elem is not mapping node", func(t *testing.T) {
		n := rootNode(t, yamlDoc(t, []byte(`
- [1,2]
- [3,4]`)))
		_, err := findAssociativeListNode(n, value.FieldList{{Name: "foo", Value: value.NewValueInterface("bar")}})
		require.Error(t, err)
	})

	t.Run("lookup", func(t *testing.T) {
		node := rootNode(t, yamlDoc(t, []byte(`
- k1: v1

- k2: v2
  100: 200

- k4: v4
  k5: true

- k6: false

- k7:
  - a
  - b
`)))

		cases := []struct {
			key       value.FieldList
			expectErr require.ErrorAssertionFunc
		}{
			{value.FieldList{{Name: "k1", Value: value.NewValueInterface("v1")}}, require.NoError},
			{value.FieldList{{Name: "k2", Value: value.NewValueInterface("")}}, require.Error},
			{value.FieldList{{Name: "k2", Value: value.NewValueInterface("v2")}}, require.NoError},
			{value.FieldList{
				{Name: "k2", Value: value.NewValueInterface("v2")},
				{Name: "100", Value: value.NewValueInterface(200)},
			}, require.NoError},
			{value.FieldList{
				{Name: "k4", Value: value.NewValueInterface("v4")},
				{Name: "k5", Value: value.NewValueInterface(true)},
			}, require.NoError},
			{value.FieldList{{Name: "k6", Value: value.NewValueInterface(true)}}, require.Error},
			{value.FieldList{{Name: "k6", Value: value.NewValueInterface(false)}}, require.NoError},
			{value.FieldList{{Name: "k7", Value: value.NewValueInterface([]any{"a"})}}, require.Error},
			{value.FieldList{{Name: "k7", Value: value.NewValueInterface([]any{"a", "b"})}}, require.NoError},
		}

		for i, c := range cases {
			t.Run(fmt.Sprintf("test=%d key=%v", i, c.key), func(t *testing.T) {
				got, err := findAssociativeListNode(node, c.key)
				c.expectErr(t, err)
				if err == nil {
					assert.NotNil(t, got)
				}
			})
		}
	})
}

func TestAnnotateManagedField(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		paths     []fieldpath.Path
		expected  string
		expectErr bool
	}{
		{
			name: "invalid pathelement",
			in:   `field1: value1`,
			paths: []fieldpath.Path{
				{fieldpath.PathElement{ /* has no fields */ }},
			},
			expectErr: true,
		},
		{
			name:      "field name not found",
			in:        `field1: value1`,
			paths:     []fieldpath.Path{fieldpath.MakePathOrDie("field2")},
			expectErr: true,
		},
		{
			name: "field index not found",
			in: `
field1:
- a`,
			paths:     []fieldpath.Path{fieldpath.MakePathOrDie("field1", 2)},
			expectErr: true,
		},
		{
			name: "field value not found",
			in: `
field1:
- a`,
			paths:     []fieldpath.Path{fieldpath.MakePathOrDie("field1", value.NewValueInterface("b"))},
			expectErr: true,
		},
		{
			name: "associative list element not found",
			in: `
field1:
- k1: v1`,
			paths:     []fieldpath.Path{fieldpath.MakePathOrDie("field1", KV("k2", "v2"))},
			expectErr: true,
		},
		{
			name:     "path at root",
			in:       `field1: value1`,
			paths:    []fieldpath.Path{fieldpath.MakePathOrDie("field1")},
			expected: `field1: value1 # V`,
		},
		{
			name: "nested map path",
			in: `
field1:
    field2:
      field3: true`,
			paths: []fieldpath.Path{
				fieldpath.MakePathOrDie("field1", "field2"),
				fieldpath.MakePathOrDie("field1", "field2", "field3"),
			},
			expected: `
field1:
    field2: # V
        field3: true # V`,
		},
		{
			name: "all in one",
			in: `
root:
    associativeArray:
    - k1: v1
    - k2: v2
    indexedArray:
    - 0
    - 1
    - 2
    valueArray:
    - a
    - b
    - c`,

			paths: []fieldpath.Path{
				P("root"),
				P("root", "associativeArray", KV("k2", "v2"), "k2"),
				P("root", "indexedArray", 1),
				P("root", "valueArray"),
				P("root", "valueArray", value.NewValueInterface("b")),
			},
			expected: `
root: # V
    associativeArray:
        - k1: v1
        - k2: v2 # V
    indexedArray:
        - 0
        - 1 # V
        - 2
    valueArray: # V
        - a
        - b # V
        - c`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node := rootNode(t, yamlDoc(t, []byte(c.in)))
			for _, p := range c.paths {
				err := annotateManagedField(node, &managedField{
					Manager: managerEntry{Name: "V"},
					Path:    p}, annotationOptions{})
				if c.expectErr {
					require.Error(t, err)
					return
				} else {
					require.NoError(t, err)
				}
			}
			var b bytes.Buffer
			require.NoError(t, yaml.NewEncoder(&b).Encode(node))
			assert.Equal(t, strings.TrimSpace(c.expected), strings.TrimSpace(b.String()))
		})
	}
}
