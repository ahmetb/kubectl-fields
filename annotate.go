package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/hako/durafmt"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"sigs.k8s.io/structured-merge-diff/v4/value"
)

type annotationPosition int

const (
	Inline annotationPosition = iota
	Above
)

type annotationOptions struct {
	clock    clock.PassiveClock
	position annotationPosition
}

// annotateManagedField annotates the given managed field entry in node.
func annotateManagedField(node *yaml.Node, entry *managedField, opts annotationOptions) error {
	fullPath := entry.Path
	path := slices.Clone(entry.Path)

	klog.V(2).InfoS("start annotating", "path", fullPath.String())

	for len(path) > 0 {
		cur := path[0]
		klog.V(2).InfoS("traversing segment", "cur", cur)

		var next *yaml.Node
		var err error

		switch { // possible formats can be seen at https://github.com/kubernetes/kubernetes/blob/9791f0d1f39f3f1e0796add7833c1059325d5098/staging/src/k8s.io/apimachinery/pkg/util/managedfields/internal/pathelement_test.go#L21-L37
		case cur.FieldName != nil: // f:fieldName entry in a mapping node
			isLeaf := (len(path) == 1)
			next, err = findFieldInMappingNode(node, *cur.FieldName, isLeaf)
			if err != nil {
				return fmt.Errorf("failed to find field %s in %s: %w", *cur.FieldName, fullPath, err)
			}
		case cur.Index != nil: // i:0 entry in a sequence node
			next, err = findValueAtIndex(node, *cur.Index)
			if err != nil {
				return fmt.Errorf("failed to find list element at index %s in %s: %w", cur.String(), fullPath, err)
			}
		case cur.Value != nil: // v:value entry in a sequence node
			next, err = findValueNode(node, *cur.Value)
			if err != nil {
				return fmt.Errorf("failed to find list element with %s in %s: %w", cur.String(), fullPath, err)
			}
		case cur.Key != nil: // k:{"key": val} in a sequence node
			next, err = findAssociativeListNode(node, *cur.Key)
			if err != nil {
				return fmt.Errorf("failed to get element from associative list requirements %s in %s: %w", cur.String(), fullPath, err)
			}
		default:
			return fmt.Errorf("unsupported path element %#v", cur)
		}

		node = next
		path = path[1:] // move to the next path segment
	}

	// when we popped all path segments, we should have the node we need to annotate
	annotateYAMLNode(node, entry, opts)

	return nil
}

// findFieldInMappingNode takes a mapping node and tries to find the field with the given name.
// If the isLeaf flag is set, the function returns the key node; otherwise it returns the
// value node to traverse into.
func findFieldInMappingNode(node *yaml.Node, fieldName string, isLeaf bool) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node; got %v", yamlNodeKind[node.Kind])
	}

	// find the key in the mapping node
	for i, elem := range node.Content {
		if elem.Value == fieldName {
			keyNode := node.Content[i]     // current element is the key
			valueNode := node.Content[i+1] // adjacent element is the value

			if isLeaf {
				// we're at the end of the path, annotate the value node
				return keyNode, nil
			} else {
				// we're not at the end of the path, traverse into the value node
				return valueNode, nil
			}
		}
	}
	return nil, fmt.Errorf("field %q not found in the mapping node", fieldName)
}

// findValueAtIndex takes a sequence node and tries to find the element at the given index.
func findValueAtIndex(node *yaml.Node, index int) (*yaml.Node, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a sequence node, got %v", yamlNodeKind[node.Kind])
	}
	if index >= len(node.Content) {
		return nil, fmt.Errorf("index %d out of range for sequence node (has %d elems)", index, len(node.Content))
	}
	return node.Content[index], nil
}

// findValueNode takes a sequence node of scalar values and tries to find the element that matches the given value.
func findValueNode(node *yaml.Node, val value.Value) (*yaml.Node, error) {
	if !val.IsString() {
		return nil, fmt.Errorf("managed field v:%v is not a string (not yet supported)", val)
	}

	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a sequence node, got %v", yamlNodeKind[node.Kind])
	}

	for _, ch := range node.Content {
		if ch.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("expected a scalar node in the sequence node, but the elements are of type %v", yamlNodeKind[ch.Kind])
		}
		if ch.Value == val.AsString() {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("value %v not found in the sequence node", val)
}

// findAssociativeListNode takes a list (SequenceNode) and tries to find the element
// that matches given criteria. The criteria are given as a set of key-value pairs.
//
// Examples for key (k: prefix appears on the Kubernetes object):
//   - k:{"name":"my-container"}
//   - k:{"jsonField":{"A":1,"B":null,"C":"D","E":{"F":"G"}}}
//   - k:{"port":"8080","protocol":"TCP"}
//   - k:{"listField":["1","2","3"]}
func findAssociativeListNode(node *yaml.Node, key value.FieldList) (*yaml.Node, error) {
	// we're trying to find the objects in a list that match all given requirements
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a sequence node, got %v", yamlNodeKind[node.Kind])
	}

	listElems := make([]map[string]any, len(node.Content))
	for i, child := range node.Content {
		m, err := mappingNodeAsMap(child)
		if err != nil {
			return nil, fmt.Errorf("error converting child node at index[%d] to map: %w", i, err)
		}
		listElems[i] = m
	}

	// see if any list nodes match the key requirements
	for i, child := range listElems {
		var meetsRequirements bool
		for _, requirement := range key {
			needKey := requirement.Name
			needVal := requirement.Value.Unstructured()

			v, ok := child[needKey]
			meetsRequirements = meetsRequirements || (ok && reflect.DeepEqual(v, needVal))
		}
		if meetsRequirements {
			return node.Content[i], nil
		}
	}
	return nil, fmt.Errorf("no elements found in the list matching the requirements %v", key)
}

func annotateYAMLNode(node *yaml.Node, entry *managedField, opts annotationOptions) {
	entry.Used = true

	comment := annotation(entry.Manager, opts.clock)

	if opts.position == Above {
		node.HeadComment = comment
	} else {
		node.LineComment = comment
	}
	klog.V(3).Info("annotated node")
}

func annotation(mgr managerEntry, c clock.PassiveClock) string {
	comment := fmt.Sprintf("%s", mgr.Name)
	if mgr.Subresource != "" {
		comment += fmt.Sprintf(" (/%s)", mgr.Subresource)
	}
	if !mgr.Time.IsZero() {
		comment += fmt.Sprintf(" (%s)", timeFmt(c, mgr.Time))
	}
	return comment
}

func timeFmt(c clock.PassiveClock, t time.Time) string {
	since := c.Now().Sub(t)
	d, _ := durafmt.ParseStringShort(since.Truncate(time.Second).String())
	units, _ := durafmt.DefaultUnitsCoder.Decode("yr:yr,wk:wk,d:d,h:h,m:m,s:s,ms:ms,µs:µs")
	return strings.ReplaceAll(d.LimitFirstN(2).Format(units), " ", "") + " ago"

}
