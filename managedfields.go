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
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	kyaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// managedField is a struct that represents an individual field that has a manager.
type managedField struct {
	// Path is the full Path of the managed field.
	Path fieldpath.Path

	// Manager is the name of the Manager that last modified the field.
	Manager managerEntry
	// Used indicates if the managedField was annotated in the output.
	// It helps keep track of which managedField found its way to the output.
	Used bool
}

type managerEntry struct {
	// Name is the Name of the field Name.
	Name string

	// Subresource is the Subresource that the path was modified through, otherwise empty.
	Subresource string

	// Time is the Time at which the field was last modified (or zero value)
	Time time.Time
}

// getManagedFields parses given object and returns its validated
// ManagedFieldEntries.
func getManagedFields(in []byte) ([]metav1.ManagedFieldsEntry, error) {
	emptyScheme := runtime.NewScheme()
	var u unstructured.Unstructured
	serializer := kyaml.NewDecodingSerializer(json.NewSerializerWithOptions(
		json.DefaultMetaFactory, emptyScheme, emptyScheme, json.SerializerOptions{}))

	obj, gvk, err := serializer.Decode(in, nil, &u)
	if err != nil {
		return nil, fmt.Errorf("failed to decode input as a Kubernetes resource: %w", err)
	}
	klog.V(1).InfoS("decoded object", "gvk", gvk)

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("error getting object metadata: %w", err)
	}
	mf := objMeta.GetManagedFields()
	if err := managedfields.ValidateManagedFields(mf); err != nil {
		return nil, fmt.Errorf("error validating managed fields on the object: %w", err)
	}
	return mf, nil
}

func extractManagedFieldSet(managedFieldsEntry metav1.ManagedFieldsEntry) ([]managedField, error) {
	fieldsJSON := bytes.NewReader(managedFieldsEntry.FieldsV1.Raw)
	fset := fieldpath.NewSet()
	if err := fset.FromJSON(fieldsJSON); err != nil {
		return nil, fmt.Errorf("error unmarshaling managed fields: %w", err)
	}
	mgr := managerEntry{
		Name:        managedFieldsEntry.Manager,
		Time:        ptr.Deref(managedFieldsEntry.Time, metav1.Time{}).Time,
		Subresource: managedFieldsEntry.Subresource,
	}
	var out []managedField
	extractManagedFields(fset).Iterate(func(p fieldpath.Path) {
		klog.V(1).InfoS("managed field", "manager", managedFieldsEntry.Manager, "path", p.String())
		out = append(out, managedField{
			Path:    slices.Clone(p), // for whatever reason the p is reused, so cloning here.
			Manager: mgr,
		})
	})
	return out, nil
}

// extractManagedFields extracts the set of managed fields from the given
// fieldpath set. These elements are leaf nodes in the trie that represent
// metadata.managedFields entries (keys whose values are `{}`).
func extractManagedFields(fs *fieldpath.Set) *fieldpath.Set {
	var out []fieldpath.Path
	collect := func(p fieldpath.Path) {
		out = append(out, p)
	}

	var recurse func(fs *fieldpath.Set, prefix fieldpath.Path)
	recurse = func(fs *fieldpath.Set, prefix fieldpath.Path) {
		// Members store the actual values at this node in the path
		// (i.e. leaf nodes in the trie).
		fs.Members.Iterate(func(pe fieldpath.PathElement) {
			path := append(slices.Clone(prefix), pe)
			collect(path)
		})

		// Children are the sub-paths where more members are stored
		// (i.e. intermediate nodes in the trie).
		fs.Children.Iterate(func(pe fieldpath.PathElement) {
			path := append(slices.Clone(prefix), pe)
			ss, ok := fs.Children.Get(pe)
			if !ok {
				return
			}
			recurse(ss, path)
		})
	}

	recurse(fs, nil)

	return fieldpath.NewSet(out...)
}
