package annotate

import (
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"go.yaml.in/yaml/v3"
)

// AnnotationInfo holds the ownership metadata for a single field annotation.
type AnnotationInfo struct {
	Manager     string
	Subresource string
	Time        time.Time
}

// AnnotationTarget pairs YAML key/value nodes with their ownership info.
// KeyNode is the mapping key (used for above-mode comments or inline on
// container fields). ValueNode is the mapping value (used for inline on
// scalar fields). For dot markers the KeyNode comes from the parent level.
type AnnotationTarget struct {
	KeyNode   *yaml.Node // key in mapping (may be nil at root level)
	ValueNode *yaml.Node // value in mapping (the owned node)
	Info      AnnotationInfo
}

// walkFieldsV1 descends the FieldsV1 ownership tree in parallel with the
// YAML document tree, collecting AnnotationTargets for every owned field.
//
// Parameters:
//   - yamlNode: the current YAML node being walked (MappingNode)
//   - parentKeyNode: the key node in the parent mapping that led to yamlNode
//     (nil when yamlNode is the document root)
//   - fieldsNode: the current FieldsV1 MappingNode containing ownership keys
//   - entry: the ManagedFieldsEntry providing manager/time metadata
//   - targets: accumulator map keyed by ValueNode pointer (last-writer-wins)
func walkFieldsV1(yamlNode *yaml.Node, parentKeyNode *yaml.Node, fieldsNode *yaml.Node, entry managed.ManagedFieldsEntry, targets map[*yaml.Node]AnnotationTarget) {
	if fieldsNode == nil || fieldsNode.Kind != yaml.MappingNode {
		return
	}

	info := annotationFrom(entry)

	for i := 0; i < len(fieldsNode.Content)-1; i += 2 {
		key := fieldsNode.Content[i].Value
		val := fieldsNode.Content[i+1]

		prefix, content := managed.ParseFieldsV1Key(key)

		switch prefix {
		case ".":
			// Dot marker: the current yamlNode itself is owned.
			// KeyNode comes from the parent level (may be nil at root).
			targets[yamlNode] = AnnotationTarget{
				KeyNode:   parentKeyNode,
				ValueNode: yamlNode,
				Info:      info,
			}

		case "f":
			// Field prefix: find the matching key-value pair in the YAML mapping.
			targetKey, targetVal := findMappingField(yamlNode, content)
			if targetKey == nil || targetVal == nil {
				continue
			}

			if isLeaf(val) {
				// Leaf field: store as annotation target.
				targets[targetVal] = AnnotationTarget{
					KeyNode:   targetKey,
					ValueNode: targetVal,
					Info:      info,
				}
			} else {
				// Non-leaf: recurse into the child mapping.
				walkFieldsV1(targetVal, targetKey, val, entry, targets)
			}

		case "k", "v":
			// TODO(02-02): list item matching handled in plan 02-02
			continue

		default:
			// Unknown prefix: skip
			continue
		}
	}
}

// findMappingField locates a key-value pair in a MappingNode by field name.
// Returns (keyNode, valueNode) or (nil, nil) if not found or node is not a mapping.
func findMappingField(mapping *yaml.Node, fieldName string) (*yaml.Node, *yaml.Node) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == fieldName {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// isLeaf returns true when a FieldsV1 value node is an empty MappingNode,
// which in the FieldsV1 encoding means "this field is a leaf" (owned directly,
// do not recurse further).
func isLeaf(node *yaml.Node) bool {
	return node.Kind == yaml.MappingNode && len(node.Content) == 0
}

// annotationFrom creates an AnnotationInfo from a ManagedFieldsEntry.
func annotationFrom(entry managed.ManagedFieldsEntry) AnnotationInfo {
	return AnnotationInfo{
		Manager:     entry.Manager,
		Subresource: entry.Subresource,
		Time:        entry.Time,
	}
}
