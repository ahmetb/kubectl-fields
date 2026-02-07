package managed

import (
	"go.yaml.in/yaml/v3"
)

// StripManagedFields removes the managedFields key from the metadata
// section of a Kubernetes resource root MappingNode. Returns true if
// managedFields was found and removed, false otherwise.
func StripManagedFields(root *yaml.Node) bool {
	if root.Kind != yaml.MappingNode {
		return false
	}

	metadataNode, ok := getMapValueNode(root, "metadata")
	if !ok || metadataNode.Kind != yaml.MappingNode {
		return false
	}

	return removeMapKey(metadataNode, "managedFields")
}

// removeMapKey removes a key-value pair from a MappingNode by key name.
// Returns true if the key was found and removed.
func removeMapKey(mapping *yaml.Node, key string) bool {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}
