package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hako/durafmt"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

type annotationPosition int

const (
	Inline annotationPosition = iota
	Above
)

type annotationOptions struct {
	position annotationPosition
}

// annotateManagedField annotates the given managed field entry in node.
func annotateManagedField(node *yaml.Node, entry *managedField, opts annotationOptions) error {
	fullPath := entry.Path
	path := slices.Clone(entry.Path)

	klog.V(1).InfoS("start annotating", "path", fullPath.String())
	for len(path) > 0 {
		cur := path[0] // depending on the first path segment, traverse into the node
		klog.V(1).InfoS("traversing segment", "cur", cur)
		// formats can be seen at https://github.com/kubernetes/kubernetes/blob/9791f0d1f39f3f1e0796add7833c1059325d5098/staging/src/k8s.io/apimachinery/pkg/util/managedfields/internal/pathelement_test.go#L21-L37
		switch {
		case cur.FieldName != nil: // f:fieldName entry in a mapping node
			if node.Kind != yaml.MappingNode {
				return fmt.Errorf("expected a mapping node on key %s (full path: %s), got %v", cur, fullPath, yamlNodeKind[node.Kind])
			}

			// find the key in the mapping node
			var found bool
			for i, elem := range node.Content {
				if elem.Value == *cur.FieldName {
					found = true

					keyNode := node.Content[i]     // current element is the key
					valueNode := node.Content[i+1] // adjacent element is the value

					if len(path) == 1 {
						// we're at the end of the path, annotate the value node
						node = keyNode
					} else {
						// we're not at the end of the path, traverse into the value node
						node = valueNode
					}

					break
				}
			}
			if !found {
				return fmt.Errorf("field %q not found in the mapping node %s", *cur.FieldName, path)
			}
		case cur.Index != nil: // i:0 entry in a sequence node
			if node.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected a sequence node %s (full path: %s), got %v", entry.Path, fullPath, yamlNodeKind[node.Kind])
			}
			if *cur.Index >= len(node.Content) {
				return fmt.Errorf("index %d out of range in sequence node %s (full path: %s)", *cur.Index, path, fullPath)
			}

			node = node.Content[*cur.Index]

		case cur.Value != nil: // v:value entry in a sequence node
			if node.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected a sequence node %s (full path: %s), got %v", entry.Path, fullPath, yamlNodeKind[node.Kind])
			}
			val := *cur.Value
			if !val.IsString() {
				return fmt.Errorf("managed field %s (with value %v) is not a string (not yet supported)", fullPath, val)
			}
			var found bool
			for _, ch := range node.Content {
				if ch.Value == val.AsString() {
					node = ch
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("value %s not found in the sequence node", cur)
			}
		case cur.Key != nil: // k:{"key": val} in a sequence node
			// examples include:
			// - k:{"name":"my-container"}
			// - k:{"jsonField":{"A":1,"B":null,"C":"D","E":{"F":"G"}}}
			// - k:{"port":"8080","protocol":"TCP"}
			// - k:{"listField":["1","2","3"]}
			//
			// we're trying to find the objects in a list that match all given requirements
			if node.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected a sequence node at %s (full path: %s), got %v", entry.Path, fullPath, yamlNodeKind[node.Kind])
			}

			listElems := make([]map[string]any, len(node.Content))
			for i, child := range node.Content {
				m, err := mappingNodeAsMap(child)
				if err != nil {
					return fmt.Errorf("error converting child node at %s[%d] to map: %w", entry.Path, i, err)
				}
				listElems[i] = m
			}

			// see if any list nodes match the key requirements
			var found bool
			for i, child := range listElems {
				var meetsRequirements bool
				for _, requirement := range *cur.Key {
					needKey := requirement.Name
					needVal := requirement.Value.Unstructured()

					v, ok := child[needKey]
					meetsRequirements = meetsRequirements || (ok && v == needVal)
				}
				if meetsRequirements {
					node = node.Content[i]
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("no elements found in %s matching the requirements %v", fullPath, *cur.Key)
			}
		default:
			return fmt.Errorf("unsupported path element %#v", cur)
		}

		path = path[1:] // move to the next path segment
	}

	// when we popped all path segments, we should have the node we need to annotate
	annotateYAMLNode(node, entry, opts)

	return nil
}

func annotateYAMLNode(node *yaml.Node, entry *managedField, opts annotationOptions) {
	entry.Used = true

	comment := annotation(entry.Manager)

	if opts.position == Above {
		node.HeadComment = comment
	} else {
		node.LineComment = comment
	}
	klog.V(3).Info("annotated node")
}

func annotation(mgr managerEntry) string {
	comment := fmt.Sprintf("%s", mgr.Name)
	if mgr.Subresource != "" {
		comment += fmt.Sprintf(" (/%s)", mgr.Subresource)
	}
	if !mgr.Time.IsZero() {
		comment += fmt.Sprintf(" (%s)", timeFmt(mgr.Time))
	}
	return comment
}

func timeFmt(t time.Time) string {
	since := time.Since(t)
	d, _ := durafmt.ParseStringShort(since.Truncate(time.Second).String())
	units, _ := durafmt.DefaultUnitsCoder.Decode("yr:yr,wk:wk,d:d,h:h,m:m,s:s,ms:ms,µs:µs")
	return strings.ReplaceAll(d.LimitFirstN(2).Format(units), " ", "") + " ago"

}
