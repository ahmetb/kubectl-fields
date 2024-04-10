/*
 .o88o.
 888 `"
o888oo  oooo d8b  .ooooo.   .ooooo.
 888    `888""8P d88' `88b d88' `88b
 888     888     888ooo888 888ooo888
 888     888     888    .o 888    .o
o888o   d888b    `Y8bod8P' `Y8bod8P'
                     oooo                         .    o8o
                     `888                       .o8    `"'
oo.ooooo.   .oooo.    888   .ooooo.   .oooo.o .o888oo oooo  ooo. .oo.    .ooooo.
 888' `88b `P  )88b   888  d88' `88b d88(  "8   888   `888  `888P"Y88b  d88' `88b
 888   888  .oP"888   888  888ooo888 `"Y88b.    888    888   888   888  888ooo888
 888   888 d8(  888   888  888    .o o.  )88b   888 .  888   888   888  888    .o
 888bod8P' `Y888""8o o888o `Y8bod8P' 8d"888P'   "888" o888o o888o o888o `Y8bod8P'
 888
o888o

*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
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

	if err := run(in, os.Stdout, annotationOptions{
		clock:    clock.RealClock{},
		position: pos}); err != nil {
		klog.Fatal(err)
	}

	klog.V(1).Info("done")
}

func run(in []byte, w io.Writer, opts annotationOptions) error {
	// Parse the input as a YAML document
	var doc yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(in)).Decode(&doc); err != nil {
		return fmt.Errorf("error reading input as YAML document: %v", err)
	}
	if err := validateDocumentIsSingleKubernetesObject(&doc); err != nil {
		return fmt.Errorf("error validating object: %v", err)
	}
	rootNode := doc.Content[0] // this is our Kubernetes object as YAML
	klog.V(1).Info("parsed input as a single Kubernetes object")

	managedFieldEntries, err := getManagedFields(in)
	if err != nil {
		return fmt.Errorf("error getting managed fields: %v", err)
	}
	klog.V(1).Infof("found %d managed field entries", len(managedFieldEntries))

	// TODO make this a nicely typed map that works with fieldpath.Path.
	var allManagedFields []managedField

	for _, managedFieldsEntry := range managedFieldEntries {
		fields, err := extractManagedFieldSet(managedFieldsEntry)
		if err != nil {
			return fmt.Errorf("error extracting managed fields: %v", err)
		}
		klog.V(1).Infof("found %d managed fields for manager %s", len(fields), managedFieldsEntry.Manager)
		allManagedFields = append(allManagedFields, fields...)
	}
	klog.V(1).Infof("total %d managed fields from %d managers", len(allManagedFields), len(managedFieldEntries))

	// Delete the metadata.managedFields from the original object
	if !stripManagedFields(rootNode) {
		klog.Warning("metadata.managedFields could not be stripped off from the object (probably a bug, please report it)")
	}

	// Annotate each managed field on the YAML document
	for i := range allManagedFields {
		klog.V(3).InfoS("call annotating field", "path", allManagedFields[i].Path)
		if err := annotateManagedField(rootNode, &allManagedFields[i], opts); err != nil {
			return fmt.Errorf("error annotating field %s: %v", allManagedFields[i].Path, err)
		}
	}

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(rootNode); err != nil {
		return fmt.Errorf("error marshaling the resulting object back to yaml: %v", err)
	}
	for _, v := range allManagedFields {
		if !v.Used {
			klog.Warningf("managed field %s is not annotated on the resulting output (probably a bug, please report it)", v.Path)
		}
	}
	return nil
}
