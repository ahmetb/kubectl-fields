package managed

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

// ManagedFieldsEntry represents a single managedFields entry from a
// Kubernetes resource's metadata.
type ManagedFieldsEntry struct {
	Manager     string
	Operation   string
	Subresource string
	Time        time.Time
	APIVersion  string
	FieldsV1    *yaml.Node // Raw YAML MappingNode of the ownership tree
}

// ExtractManagedFields finds and parses managedFields entries from a
// Kubernetes resource root MappingNode. Returns nil, nil if metadata or
// managedFields are not present (not an error).
func ExtractManagedFields(root *yaml.Node) ([]ManagedFieldsEntry, error) {
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected MappingNode, got kind %d", root.Kind)
	}

	metadataNode, ok := getMapValueNode(root, "metadata")
	if !ok {
		return nil, nil
	}
	if metadataNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	managedNode, ok := getMapValueNode(metadataNode, "managedFields")
	if !ok {
		return nil, nil
	}
	if managedNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("managedFields is not a sequence (kind %d)", managedNode.Kind)
	}

	var entries []ManagedFieldsEntry
	for _, item := range managedNode.Content {
		entry, err := parseManagedFieldEntry(item)
		if err != nil {
			return nil, fmt.Errorf("parsing managedFields entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// parseManagedFieldEntry parses a single managedFields entry MappingNode.
func parseManagedFieldEntry(node *yaml.Node) (ManagedFieldsEntry, error) {
	if node.Kind != yaml.MappingNode {
		return ManagedFieldsEntry{}, fmt.Errorf("expected MappingNode for entry, got kind %d", node.Kind)
	}

	var entry ManagedFieldsEntry

	if v, ok := getMapValue(node, "manager"); ok {
		entry.Manager = v
	}
	if v, ok := getMapValue(node, "operation"); ok {
		entry.Operation = v
	}
	if v, ok := getMapValue(node, "subresource"); ok {
		entry.Subresource = v
	}
	if v, ok := getMapValue(node, "apiVersion"); ok {
		entry.APIVersion = v
	}
	if v, ok := getMapValue(node, "time"); ok {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return ManagedFieldsEntry{}, fmt.Errorf("parsing time %q: %w", v, err)
		}
		entry.Time = t
	}
	if n, ok := getMapValueNode(node, "fieldsV1"); ok {
		entry.FieldsV1 = n
	}

	return entry, nil
}

// getMapValue finds a key in a MappingNode and returns its string value.
func getMapValue(mapping *yaml.Node, key string) (string, bool) {
	if mapping.Kind != yaml.MappingNode {
		return "", false
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1].Value, true
		}
	}
	return "", false
}

// getMapValueNode finds a key in a MappingNode and returns the value Node.
func getMapValueNode(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}
