package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	kyaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
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

	rootNode, err := yamlRootNode(in)
	if err != nil {
		klog.Fatalf("error parsing input: %v", err)
	}

	managedFieldEntries, err := getManagedFields(in)
	if err != nil {
		klog.Fatalf("error getting managed fields: %v", err)
	}

	if len(managedFieldEntries) == 0 {
		klog.Fatal(`no metadata.managedFields found on the original object.` +
			` use "kubectl get --show-managed-fields -o=yaml"` +
			` to get the resource, and pipe its output to this program`)
	}

	// TODO make this a nicely typed map that works with fieldpath.Path.
	var allManagedFields []managedFieldEntry

	for _, managedFieldsEntry := range managedFieldEntries {
		fieldsJSON := bytes.NewReader(managedFieldsEntry.FieldsV1.Raw)
		fset := fieldpath.NewSet()
		if err := fset.FromJSON(fieldsJSON); err != nil {
			klog.Fatalf("error unmarshaling managed fields: %v", err)
		}
		extractManagedFields(fset).Iterate(func(p fieldpath.Path) {
			klog.V(1).InfoS("managed field", "manager", managedFieldsEntry.Manager, "path", p.String())
			allManagedFields = append(allManagedFields, managedFieldEntry{
				path:        clone(p), // for whatever reason the p is reused
				manager:     managedFieldsEntry.Manager,
				subresource: managedFieldsEntry.Subresource,
				time:        ptr.Deref(managedFieldsEntry.Time, metav1.Time{}),
			})
		})
		// Delete the metadata.managedFields from the original object
	}

	// Delete the metadata.managedFields from the original object
	stripManagedFields(rootNode)

	// Annotate each managed field on the YAML document
	for i := range allManagedFields {
		klog.V(3).InfoS("call annotating field", "path", allManagedFields[i].path)
		if err := annotateManagedField(rootNode, &allManagedFields[i]); err != nil {
			klog.Fatalf("error annotating field %s: %v", allManagedFields[i].path, err)
		}
	}

	output, err := yaml.Marshal(&rootNode)
	if err != nil {
		klog.Fatalf("error marshaling the resulting object back to yaml: %v", err)
	}
	fmt.Print(string(output))

	for _, v := range allManagedFields {
		if !v.used {
			klog.Warningf("managed field=%s is not annotated on the resulting yaml", v.path)
		}
	}
}

// annotateManagedField annotates the given yaml node in the document with the given
// managed field entry at path.
func annotateManagedField(node *yaml.Node, entry *managedFieldEntry) error {
	fullPath := entry.path
	path := clone(entry.path)

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
				return fmt.Errorf("expected a sequence node %s (full path: %s), got %v", entry.path, fullPath, yamlNodeKind[node.Kind])
			}
			if *cur.Index >= len(node.Content) {
				return fmt.Errorf("index %d out of range in sequence node %s (full path: %s)", *cur.Index, path, fullPath)
			}

			node = node.Content[*cur.Index]

		case cur.Value != nil: // v:value entry in a sequence node
			if node.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected a sequence node %s (full path: %s), got %v", entry.path, fullPath, yamlNodeKind[node.Kind])
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
				return fmt.Errorf("expected a sequence node at %s (full path: %s), got %v", entry.path, fullPath, yamlNodeKind[node.Kind])
			}

			listElems := make([]map[string]any, len(node.Content))
			for i, child := range node.Content {
				m, err := mappingNodeAsMap(child)
				if err != nil {
					return fmt.Errorf("error converting child node at %s[%d] to map: %w", entry.path, i, err)
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

func annotateYAMLNode(node *yaml.Node, entry *managedFieldEntry) {
	entry.used = true

	comment := fmt.Sprintf("%s", entry.manager)
	if entry.subresource != "" {
		comment += fmt.Sprintf(" (/%s)", entry.subresource)
	}
	if !entry.time.IsZero() {
		comment += fmt.Sprintf(" (%s)", timeFmt(entry.time.Time))
	}

	if *flPosition == "above" {
		node.HeadComment = comment
	} else {
		node.LineComment = comment
	}
	klog.V(3).Info("annotated node")
}

func timeFmt(t time.Time) string {
	s := time.Since(t).Round(time.Minute).String()
	return strings.Replace(s, "m0s", "m", 1) + " ago" // 13m0s --> 13m
}

type managedFieldEntry struct {
	path        fieldpath.Path
	manager     string
	subresource string
	time        metav1.Time

	used bool
}

func yamlRootNode(in []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(in))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("error reading input as YAML document: %v", err)
	}
	if err := validateDocumentIsKubernetesObject(&doc); err != nil {
		return nil, fmt.Errorf("error validating document: %v", err)
	}
	return doc.Content[0], nil
}

func validateDocumentIsKubernetesObject(doc *yaml.Node) error {
	if doc.Kind != yaml.DocumentNode {
		return errors.New("only single object yaml documents are supported as input")
	}

	// Ensure the document node contains a mapping node as its content.
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid document structure (yaml.Kind=%v, content_len=%d)", doc.Content[0].Kind, len(doc.Content))
	}

	// TODO validate `metadata.managedField` exists

	rootDoc := doc.Content[0]
	_ = rootDoc
	return nil
}

// getManagedFields parses given object and returns its validated
// ManagedFieldEntries.
func getManagedFields(in []byte) ([]metav1.ManagedFieldsEntry, error) {
	emptyScheme := runtime.NewScheme()
	var u unstructured.Unstructured
	serializer := kyaml.NewDecodingSerializer(json.NewSerializerWithOptions(
		json.DefaultMetaFactory, emptyScheme, emptyScheme, json.SerializerOptions{}))

	obj, gvk, err := serializer.Decode(in, nil, &u)
	if err != nil {
		return nil, fmt.Errorf("failed to decode input as a Kubernetes resource: %w", err)
	}
	klog.V(1).InfoS("decoded object", "gvk", gvk)

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("error getting object metadata: %w", err)
	}
	mf := objMeta.GetManagedFields()
	if err := managedfields.ValidateManagedFields(mf); err != nil {
		return nil, fmt.Errorf("error validating managed fields on the object: %w", err)
	}
	return mf, nil
}

// extractManagedFields extracts the set of managed fields from the given
// fieldpath set. These elements are basically those appear in metadata.managedFields
// with a value of `{}`.
func extractManagedFields(fs *fieldpath.Set) *fieldpath.Set {
	var out []fieldpath.Path
	f := func(p fieldpath.Path) {
		out = append(out, p)
	}
	extractRecurse(fs, nil, f)
	return fieldpath.NewSet(out...)
}

func extractRecurse(fs *fieldpath.Set, prefix fieldpath.Path, f func(fieldpath.Path)) {
	// members are the set of managed fields on the entry that appear like `"f:foo": {}`
	fs.Members.Iterate(func(pe fieldpath.PathElement) {
		path := append(clone(prefix), pe)
		f(path)
	})

	// children are the sub-paths where more members are stored
	fs.Children.Iterate(func(pe fieldpath.PathElement) {
		path := append(clone(prefix), pe)
		ss, ok := fs.Children.Get(pe)
		if !ok {
			return
		}
		extractRecurse(ss, path, f)
	})
}

func clone[T any](v []T) []T {
	return append([]T(nil), v...)
}

// mappingNodeAsMap converts a given yaml object (kind=MappingNode) into
// an unstructured Go map
func mappingNodeAsMap(node *yaml.Node) (map[string]any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", node.Kind)
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
		klog.Warning("metadata not found in the object")
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

var yamlNodeKind = map[yaml.Kind]string{
	yaml.DocumentNode: "DocumentNode",
	yaml.SequenceNode: "SequenceNode",
	yaml.MappingNode:  "MappingNode",
	yaml.ScalarNode:   "ScalarNode",
	yaml.AliasNode:    "AliasNode",
}
