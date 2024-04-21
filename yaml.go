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
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

var (
	yamlNodeKind = map[yaml.Kind]string{
		yaml.DocumentNode: "DocumentNode",
		yaml.SequenceNode: "SequenceNode",
		yaml.MappingNode:  "MappingNode",
		yaml.ScalarNode:   "ScalarNode",
		yaml.AliasNode:    "AliasNode",
	}
)

// validateDocumentIsSingleKubernetesObject validates that the input document is
// a single Kubernetes object with a `metadata.managedFields` field.
func validateDocumentIsSingleKubernetesObject(doc *yaml.Node) error {
	if doc.Kind != yaml.DocumentNode {
		return fmt.Errorf("input object is not a YAML document (kind=%v)", yamlNodeKind[doc.Kind])
	}

	if len(doc.Content) != 1 {
		return errors.New("input document contains multiple YAML documents")
	}

	rootNode := doc.Content[0]
	// Ensure the document node contains a mapping node as its content.
	if rootNode.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid document structure, first object must be a yaml map (kind=%v)", yamlNodeKind[rootNode.Kind])
	}

	// make sure the doc is not a metav1.List
	if kind, ok := getMapValue(rootNode, "kind"); ok && kind == "List" {
		return errors.New("input is a meta/v1.List object, only single objects are supported")
	}

	// Ensure input has `metadata.managedField`
	metadata, ok := getMapValueNode(rootNode, "metadata")
	if !ok {
		return errors.New(".metadata not found in the object (is it a valid Kubernetes object?)")
	}
	_, ok = getMapValueNode(metadata, "managedFields")
	if !ok {
		return errors.New(".metadata.managedFields not found in the object, use `kubectl get --show-managed-fields -o=yaml` to get the resource")
	}
	return nil
}

// stripManagedFields removes the `metadata.managedFields` field from the given
// yaml document. Returns true if the field was found and removed.
func stripManagedFields(rootDoc *yaml.Node) bool {
	var metadataNode *yaml.Node

	for i, c := range rootDoc.Content {
		if c.Value == "metadata" {
			metadataNode = rootDoc.Content[i+1]
			break
		}
	}
	if metadataNode == nil {
		return false
	}

	for i, c := range metadataNode.Content {
		if c.Value == "managedFields" {
			// remove the key and the value adjacent to it
			metadataNode.Content = append(metadataNode.Content[:i], metadataNode.Content[i+2:]...)
			return true
		}
	}
	return false
}

// getMapValueNode returns the value node of a mapping entry in given node or returns
// false if the key is not found.
func getMapValueNode(mappingNode *yaml.Node, key string) (*yaml.Node, bool) {
	for i, content := range mappingNode.Content {
		if content.Value == key {
			return mappingNode.Content[i+1], true
		}
	}
	return nil, false
}

// getMapValue returns the string value of a mapping entry in given node or returns
// false if the value is not a scalar node or the key is not found.
func getMapValue(mappingNode *yaml.Node, key string) (string, bool) {
	valNode, ok := getMapValueNode(mappingNode, key)
	if !ok || valNode.Kind != yaml.ScalarNode {
		return "", false
	}
	return valNode.Value, true
}

// mappingNodeAsMap converts a given yaml object (kind=MappingNode) into
// an unstructured Go map
func mappingNodeAsMap(node *yaml.Node) (map[string]any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", yamlNodeKind[node.Kind])
	}
	// easiest way to do this is to round-trip
	var b bytes.Buffer
	if err := yaml.NewEncoder(&b).Encode(node); err != nil {
		return nil, fmt.Errorf("error encoding node: %w", err)
	}
	var out map[string]any
	if err := yaml.NewDecoder(&b).Decode(&out); err != nil {
		return nil, fmt.Errorf("error decoding node: %w", err)
	}
	return out, nil
}
