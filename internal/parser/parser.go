package parser

import (
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

// ParseDocuments decodes all YAML documents from the reader and returns
// them as a slice of yaml.Node pointers. Each node has Kind == DocumentNode.
func ParseDocuments(r io.Reader) ([]*yaml.Node, error) {
	decoder := yaml.NewDecoder(r)
	var docs []*yaml.Node

	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("YAML parse error: %w", err)
		}
		docs = append(docs, &doc)
	}

	return docs, nil
}

// UnwrapListKind checks if a document is a Kubernetes List kind and, if so,
// unwraps its items into individual DocumentNode entries. Non-List documents
// are returned as-is in a single-element slice.
func UnwrapListKind(doc *yaml.Node) []*yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return []*yaml.Node{doc}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return []*yaml.Node{doc}
	}

	kind, ok := getMapValue(root, "kind")
	if !ok || kind != "List" {
		return []*yaml.Node{doc}
	}

	itemsNode, ok := getMapValueNode(root, "items")
	if !ok || itemsNode.Kind != yaml.SequenceNode {
		return []*yaml.Node{doc}
	}

	var result []*yaml.Node
	for _, item := range itemsNode.Content {
		docNode := &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{item},
		}
		result = append(result, docNode)
	}

	return result
}

// EncodeDocuments writes all YAML documents to the writer with kubectl-compatible
// formatting: 2-space indent and compact sequence indent style.
func EncodeDocuments(w io.Writer, docs []*yaml.Node) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()

	enc.SetIndent(2)
	enc.CompactSeqIndent()

	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return fmt.Errorf("YAML encode error: %w", err)
		}
	}

	return nil
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
