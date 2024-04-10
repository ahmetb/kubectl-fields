package main

import (
	"bytes"
	"flag"
	"io"
	"os"

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

	var pos = map[string]annotationPosition{
		"inline": Inline,
		"above":  Above,
	}[*flPosition]

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
		if err := annotateManagedField(rootNode, &allManagedFields[i], annotationOptions{position: pos}); err != nil {
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
