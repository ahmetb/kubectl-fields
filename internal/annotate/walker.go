package annotate

import (
	"encoding/json"
	"fmt"
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

		case "k":
			// Associative key prefix: yamlNode is a SequenceNode containing
			// MappingNodes. Parse the JSON key and find the matching item.
			assocKey, err := managed.ParseAssociativeKey(content)
			if err != nil || assocKey == nil {
				continue
			}
			item := findSequenceItemByKey(yamlNode, assocKey)
			if item == nil {
				continue
			}
			if isLeaf(val) {
				// Rare: k: item is a leaf itself.
				targets[item] = AnnotationTarget{
					KeyNode:   nil,
					ValueNode: item,
					Info:      info,
				}
			} else {
				// Non-leaf: recurse into the item's fields.
				// Pass nil for parentKeyNode since sequence items
				// don't have a key in the parent mapping sense.
				walkFieldsV1(item, nil, val, entry, targets)
			}

		case "v":
			// Set value prefix: yamlNode is a SequenceNode containing
			// ScalarNodes. Find the matching scalar by value.
			item := findSequenceItemByValue(yamlNode, content)
			if item == nil {
				continue
			}
			// v: items are always leaves.
			targets[item] = AnnotationTarget{
				KeyNode:   nil,
				ValueNode: item,
				Info:      info,
			}

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

// findSequenceItemByKey locates a MappingNode in a SequenceNode whose fields
// match all key-value pairs in assocKey (from a FieldsV1 k: prefix).
func findSequenceItemByKey(seq *yaml.Node, assocKey map[string]any) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if matchesAssociativeKey(item, assocKey) {
			return item
		}
	}
	return nil
}

// matchesAssociativeKey returns true if every key-value pair in assocKey has a
// matching field in the YAML MappingNode.
func matchesAssociativeKey(mapping *yaml.Node, assocKey map[string]any) bool {
	for field, jsonVal := range assocKey {
		_, valNode := findMappingField(mapping, field)
		if valNode == nil {
			return false
		}
		if !matchValue(valNode.Value, jsonVal) {
			return false
		}
	}
	return true
}

// matchValue compares a YAML scalar string value against a JSON-decoded value.
// Handles string, float64 (JSON numbers), and bool comparisons.
func matchValue(yamlVal string, jsonVal any) bool {
	switch v := jsonVal.(type) {
	case string:
		return yamlVal == v
	case float64:
		return yamlVal == fmt.Sprintf("%g", v)
	case bool:
		return yamlVal == fmt.Sprintf("%t", v)
	default:
		return false
	}
}

// findSequenceItemByValue locates a ScalarNode in a SequenceNode by its value.
// The content parameter is JSON-encoded (e.g., `"example.com/foo"`); it is
// decoded before comparison so that the quotes are stripped.
func findSequenceItemByValue(seq *yaml.Node, jsonContent string) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(jsonContent), &decoded); err != nil {
		return nil
	}

	str, ok := decoded.(string)
	if !ok {
		return nil
	}

	for _, item := range seq.Content {
		if item.Kind == yaml.ScalarNode && item.Value == str {
			return item
		}
	}
	return nil
}

// annotationFrom creates an AnnotationInfo from a ManagedFieldsEntry.
func annotationFrom(entry managed.ManagedFieldsEntry) AnnotationInfo {
	return AnnotationInfo{
		Manager:     entry.Manager,
		Subresource: entry.Subresource,
		Time:        entry.Time,
	}
}
