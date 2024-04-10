package main

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
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
	if kind, ok := getValue(rootNode, "kind"); ok && kind == "List" {
		return errors.New("input is a meta/v1.List object, only single objects are supported")
	}

	// Ensure input has `metadata.managedField`
	metadata, ok := getValueNode(rootNode, "metadata")
	if !ok {
		return errors.New(".metadata not found in the object (is it a valid Kubernetes object?)")
	}
	_, ok = getValueNode(metadata, "managedFields")
	if !ok {
		return errors.New(".metadata.managedFields not found in the object, use `kubectl get --show-managed-fields -o=yaml` to get the resource")
	}
	return nil
}

// stripManagedFields removes the `metadata.managedFields` field from the given
// yaml document.
func stripManagedFields(rootDoc *yaml.Node) {
	var metadataNode *yaml.Node

	for i, c := range rootDoc.Content {
		if c.Value == "metadata" {
			metadataNode = rootDoc.Content[i+1]
			break
		}
	}
	if metadataNode == nil {
		return
	}

	for i, c := range metadataNode.Content {
		if c.Value == "managedFields" {
			// remove the key and the value adjacent to it
			metadataNode.Content = append(metadataNode.Content[:i], metadataNode.Content[i+2:]...)
			klog.V(3).Info("stripped managedFields from metadata")
			return
		}
	}
}
