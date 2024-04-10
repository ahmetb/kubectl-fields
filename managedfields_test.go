package main

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v4/value"
)

const exampleObj = `apiVersion: v1
kind: Foo
metadata:
  managedFields:
  - apiVersion: apps/v1
    fieldsType: FieldsV1
    manager: manager1
    operation: Apply
    fieldsV1:
      f:metadata:
        f:annotations:
          .: {}
          f:kubectl.kubernetes.io/last-applied-configuration: {}
        f:labels:
          .: {}
          f:app: {}
      f:spec:
        f:template:
          f:spec:
            f:containers:
              k:{"name":"nginx"}:
                .: {}
                f:ports:
                  .: {}
                  k:{"containerPort":80,"protocol":"TCP"}:
                    .: {}
                    f:containerPort: {}
                    f:protocol: {}
  - apiVersion: apps/v1
    fieldsType: FieldsV1
    fieldsV1:
      f:metadata:
        f:finalizers:
          v:"my-finalizer": {}
      f:spec:
        f:template:
          f:spec:
            f:containers:
              k:{"name":"nginx"}:
                f:env:
                  .: {}
                  k:{"foo":"bar"}:
                    .: {}
                    f:name: {}
                    f:value: {}
    manager: manager2
    operation: Update
    subresource: status
    time: "2024-04-10T00:35:11Z"`

var P = fieldpath.MakePathOrDie
var KV = fieldpath.KeyByFields

func TestGetManagedFields(t *testing.T) {
	t.Run("not kubernetes object", func(t *testing.T) {
		_, err := getManagedFields([]byte(`foo: bar`))
		require.Error(t, err)
	})
	t.Run("no metadata", func(t *testing.T) {
		got, err := getManagedFields([]byte(`apiVersion: v1
kind: Foo`))
		require.NoError(t, err)
		require.Empty(t, got)
	})
	t.Run("invalid managedFields", func(t *testing.T) {
		_, err := getManagedFields([]byte(`apiVersion: v1
kind: Foo
metadata:
  managedFields:
  - operation: Apply`))
		require.NoError(t, err)
	})
	t.Run("valid obj", func(t *testing.T) {
		got, err := getManagedFields([]byte(exampleObj))
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "manager1", got[0].Manager)
		require.Equal(t, "manager2", got[1].Manager)
	})
}

func TestExtractManagedFieldSet(t *testing.T) {
	got, err := getManagedFields([]byte(exampleObj))
	require.NoError(t, err)
	require.Len(t, got, 2)

	mgr1 := got[0]
	got1, err := extractManagedFieldSet(mgr1)
	require.NoError(t, err)
	mgrEntry1 := managerEntry{
		Name: "manager1",
	}
	expected1 := []managedField{
		{Manager: mgrEntry1, Path: P("metadata", "annotations")},
		{Manager: mgrEntry1, Path: P("metadata", "labels")},
		{Manager: mgrEntry1, Path: P("metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")},
		{Manager: mgrEntry1, Path: P("metadata", "labels", "app")},
		{Manager: mgrEntry1, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"))},
		{Manager: mgrEntry1, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "ports")},
		{Manager: mgrEntry1, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "ports", KV("containerPort", float64(80), "protocol", "TCP"))},
		{Manager: mgrEntry1, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "ports", KV("containerPort", float64(80), "protocol", "TCP"), "containerPort")},
		{Manager: mgrEntry1, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "ports", KV("containerPort", float64(80), "protocol", "TCP"), "protocol")},
	}
	assert.Empty(t, cmp.Diff(expected1, got1), "-want +got")

	mgrEntry2 := managerEntry{
		Name:        "manager2",
		Subresource: "status",
		Time:        time.Date(2024, 4, 10, 0, 35, 11, 0, time.UTC),
	}
	got2, err := extractManagedFieldSet(got[1])
	require.NoError(t, err)
	expected2 := []managedField{
		{Manager: mgrEntry2, Path: P("metadata", "finalizers", value.NewValueInterface("my-finalizer"))},
		{Manager: mgrEntry2, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "env")},
		{Manager: mgrEntry2, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "env", KV("foo", "bar"))},
		{Manager: mgrEntry2, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "env", KV("foo", "bar"), "name")},
		{Manager: mgrEntry2, Path: P("spec", "template", "spec", "containers", KV("name", "nginx"), "env", KV("foo", "bar"), "value")},
	}
	assert.Empty(t, cmp.Diff(expected2, got2), "-want +got")
}

func TestExtractManagedFields(t *testing.T) {
	set := fieldpath.NewSet(
		P("metadata", "annotations"),
		P("metadata", "annotations", "my-annotation"),
		P("spec", "env", KV("name", "MY_ENV")),
		P("spec", "finalizers", 1),
		P("metadata", "finalizers", value.NewValueInterface("my-finalizer")),
	)
	got := extractManagedFields(set)
	require.Equal(t, 5, got.Size())
}
