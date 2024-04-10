package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/hako/durafmt"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

var (
	flPosition *string
)

func main() {
	klog.InitFlags(flag.CommandLine)
	defer klog.Flush()

	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		pflag.CommandLine.AddGoFlag(f)
	})
	flPosition = pflag.StringP("position", "p", "inline", "comment position on the yaml (inline|above)")
	pflag.Parse()

	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		klog.Fatalf("error reading input: %v", err)
	}

	// Parse the input as a YAML document
	var doc yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(in)).Decode(&doc); err != nil {
		klog.Fatalf("error reading input as YAML document: %v", err)
	}
	if err := validateDocumentIsSingleKubernetesObject(&doc); err != nil {
		klog.Fatalf("error validating object: %v", err)
	}
	rootNode := doc.Content[0] // this is our Kubernetes object as YAML
	klog.V(1).Info("parsed input as a single Kubernetes object")

	managedFieldEntries, err := getManagedFields(in)
	if err != nil {
		klog.Fatalf("error getting managed fields: %v", err)
	}
	klog.V(1).Infof("found %d managed field entries", len(managedFieldEntries))

	// TODO make this a nicely typed map that works with fieldpath.Path.
	var allManagedFields []managedField

	for _, managedFieldsEntry := range managedFieldEntries {
		fields, err := extractManagedFieldSet(managedFieldsEntry)
		if err != nil {
			klog.Fatalf("error extracting managed fields: %v", err)
		}
		klog.V(1).Infof("found %d managed fields for manager %s", len(fields), managedFieldsEntry.Manager)
		allManagedFields = append(allManagedFields, fields...)
	}
	klog.V(1).Infof("total %d managed fields from %d managers", len(allManagedFields), len(managedFieldEntries))

	// Delete the metadata.managedFields from the original object
	stripManagedFields(rootNode)

	// Annotate each managed field on the YAML document
	for i := range allManagedFields {
		klog.V(3).InfoS("call annotating field", "path", allManagedFields[i].Path)
		if err := annotateManagedField(rootNode, &allManagedFields[i]); err != nil {
			klog.Fatalf("error annotating field %s: %v", allManagedFields[i].Path, err)
		}
	}

	if err := yaml.NewEncoder(os.Stdout).Encode(rootNode); err != nil {
		klog.Fatalf("error marshaling the resulting object back to yaml: %v", err)
	}
	for _, v := range allManagedFields {
		if !v.Used {
			klog.Warningf("managed field %s is not annotated on the resulting output (probably a bug, please report it)", v.Path)
		}
	}
	klog.V(1).Info("done")
}

// annotateManagedField annotates the given managed field entry in node.
func annotateManagedField(node *yaml.Node, entry *managedField) error {
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
	annotateYAMLNode(node, entry)

	return nil
}

func annotateYAMLNode(node *yaml.Node, entry *managedField) {
	entry.Used = true

	comment := fmt.Sprintf("%s", entry.Manager.Name)
	if entry.Manager.Subresource != "" {
		comment += fmt.Sprintf(" (/%s)", entry.Manager.Subresource)
	}
	if !entry.Manager.Time.IsZero() {
		comment += fmt.Sprintf(" (%s)", timeFmt(entry.Manager.Time))
	}

	if *flPosition == "above" {
		node.HeadComment = comment
	} else {
		node.LineComment = comment
	}
	klog.V(3).Info("annotated node")
}

func timeFmt(t time.Time) string {
	s, _ := durafmt.ParseStringShort(time.Since(t).Truncate(time.Second).String())
	units, _ := durafmt.DefaultUnitsCoder.Decode("yr:yr,wk:wk,d:d,h:h,m:m,s:s,ms:ms,µs:µs")
	return strings.ReplaceAll(s.LimitFirstN(2).Format(units), " ", "") + " ago"
}
