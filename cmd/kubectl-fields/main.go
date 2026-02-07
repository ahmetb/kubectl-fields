package main

import (
	"fmt"
	"os"

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
  kubectl get deploy nginx -o yaml | kubectl-fields
  kubectl get pods -o yaml | kubectl-fields

The tool processes managedFields metadata to show who owns each field
and when it was last updated, making field ownership visible without
reading raw managedFields JSON.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			docs, err := parser.ParseDocuments(os.Stdin)
			if err != nil {
				return err
			}

			// Unwrap any List kind documents into individual items.
			var allDocs []*yaml.Node
			for _, doc := range docs {
				allDocs = append(allDocs, parser.UnwrapListKind(doc)...)
			}

			if err := parser.EncodeDocuments(os.Stdout, allDocs); err != nil {
				return err
			}

			return nil
		},
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
