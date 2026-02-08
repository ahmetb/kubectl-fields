package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/annotate"
	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"github.com/rewanthtammana/kubectl-fields/internal/parser"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kubectl-fields",
		Short: "Annotate Kubernetes YAML with field ownership information",
		Long: `kubectl-fields reads Kubernetes resource YAML from stdin, annotates each
managed field with its owner (manager name and timestamp), and writes the
annotated YAML to stdout.

Usage:
  kubectl get deploy nginx -o yaml --show-managed-fields | kubectl-fields
  kubectl get pods -o yaml --show-managed-fields | kubectl-fields
  kubectl get deploy -o yaml --show-managed-fields | kubectl-fields --above

The tool processes managedFields metadata to show who owns each field
and when it was last updated, making field ownership visible without
reading raw managedFields JSON.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			aboveMode, _ := cmd.Flags().GetBool("above")

			docs, err := parser.ParseDocuments(os.Stdin)
			if err != nil {
				return err
			}

			// Unwrap any List kind documents into individual items.
			var allDocs []*yaml.Node
			for _, doc := range docs {
				allDocs = append(allDocs, parser.UnwrapListKind(doc)...)
			}

			// Extract managedFields, annotate fields, then strip managedFields.
			foundManagedFields := false
			for _, doc := range allDocs {
				if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
					continue
				}
				root := doc.Content[0]

				entries, err := managed.ExtractManagedFields(root)
				if err != nil {
					return fmt.Errorf("extracting managedFields: %w", err)
				}
				if len(entries) > 0 {
					foundManagedFields = true
				}

				// Annotate owned fields with ownership comments.
				if len(entries) > 0 {
					annotate.Annotate(root, entries, annotate.Options{
						Above: aboveMode,
						Now:   time.Now(),
					})
				}

				// Strip managedFields from the YAML tree.
				managed.StripManagedFields(root)
			}

			if !foundManagedFields {
				fmt.Fprintln(os.Stderr, "Warning: no managedFields found. Did you use --show-managed-fields?")
			}

			if err := parser.EncodeDocuments(os.Stdout, allDocs); err != nil {
				return err
			}

			return nil
		},
	}

	rootCmd.Flags().Bool("above", false, "Place annotations on the line above each field instead of inline")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
