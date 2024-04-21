// Copyright 2024 Ahmet Alp Balkan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v4/value"
)

func TestTimeFmt(t *testing.T) {
	now := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	c := testingclock.NewFakePassiveClock(now)
	cases := []struct {
		fmt      timeFormat
		offset   time.Duration
		expected string
	}{
		{Relative, 0, "0s ago"},
		{Relative, time.Second, "1s ago"},
		{Relative, time.Second*70 + time.Millisecond*999, "1m10s ago"},
		{Relative, time.Minute*70 + time.Second, "1h10m ago"},
		{Relative, time.Hour*25 + time.Minute*31, "1d1h ago"},
		{Relative, time.Hour*24*365*2 + time.Hour*73, "2yr3d ago"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("offset %s", tc.offset), func(t *testing.T) {
			assert.Equal(t, tc.expected, timeFmt(now.Add(-tc.offset), annotationOptions{
				Clock:   c,
				TimeFmt: tc.fmt,
			}))
		})
	}
}

func TestAnnotation(t *testing.T) {
	now := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	c := testingclock.NewFakePassiveClock(now)

	assert.Equal(t, "test", annotation(
		managerEntry{Name: "test"},
		annotationOptions{Clock: c},
	))
	assert.Equal(t, "test (/scale)", annotation(
		managerEntry{Name: "test", Subresource: "scale"},
		annotationOptions{Clock: c},
	))
	assert.Equal(t, "test (/scale) (1h ago)", annotation(
		managerEntry{Name: "test", Subresource: "scale", Time: now.Add(-time.Hour)},
		annotationOptions{Clock: c},
	))
	assert.Equal(t, "test (/scale) (2000-01-01T00:00:00Z)", annotation(
		managerEntry{Name: "test", Subresource: "scale", Time: now.Add(-time.Hour)},
		annotationOptions{Clock: c, TimeFmt: Absolute},
	))
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
	annotateYAMLNode(node.Content[4], &managedField{Manager: managerEntry{Name: "field3mgr"}}, annotationOptions{Position: Above})
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
