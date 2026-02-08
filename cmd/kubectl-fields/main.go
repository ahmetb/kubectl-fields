package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/ahmetb/kubectl-fields/internal/annotate"
	"github.com/ahmetb/kubectl-fields/internal/managed"
	"github.com/ahmetb/kubectl-fields/internal/output"
	"github.com/ahmetb/kubectl-fields/internal/parser"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
	"golang.org/x/term"
)

// colorFlag is a pflag.Value for the --color flag accepting auto|always|never.
type colorFlag string

func (f *colorFlag) String() string { return string(*f) }
func (f *colorFlag) Set(val string) error {
	switch val {
	case "auto", "always", "never":
		*f = colorFlag(val)
		return nil
	default:
		return fmt.Errorf("must be one of: auto, always, never")
	}
}
func (f *colorFlag) Type() string { return "string" }

// mtimeFlag is a pflag.Value for the --mtime flag accepting relative|absolute|hide.
type mtimeFlag string

func (f *mtimeFlag) String() string { return string(*f) }
func (f *mtimeFlag) Set(val string) error {
	switch val {
	case "relative", "absolute", "hide":
		*f = mtimeFlag(val)
		return nil
	default:
		return fmt.Errorf("must be one of: relative, absolute, hide")
	}
}
func (f *mtimeFlag) Type() string { return "string" }

func main() {
	var colorFlagVar colorFlag = "auto"
	var mtimeFlagVar mtimeFlag = "relative"

	rootCmd := &cobra.Command{
		Use:   "kubectl fields",
		Short: "Annotate Kubernetes YAML with field ownership information",
		Long: `kubectl fields reads Kubernetes resource YAML from stdin, annotates each
managed field with its owner (manager name and timestamp), and writes the
annotated YAML to stdout.

Usage:
  kubectl get deploy nginx -o yaml --show-managed-fields | kubectl fields
  kubectl get deploy nginx -o yaml --show-managed-fields | kubectl fields --color always
  kubectl get deploy nginx -o yaml --show-managed-fields | kubectl fields --mtime hide
  kubectl get deploy -o yaml --show-managed-fields | kubectl fields --above
  kubectl get deploy nginx -o yaml --show-managed-fields | kubectl fields --show-operation

The tool processes managedFields metadata to show who owns each field
and when it was last updated, making field ownership visible without
reading raw managedFields JSON.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			aboveMode, _ := cmd.Flags().GetBool("above")
			showOperation, _ := cmd.Flags().GetBool("show-operation")

			// Resolve color mode: auto detects TTY, always/never override.
			colorEnabled := output.ResolveColor(string(colorFlagVar), term.IsTerminal(int(os.Stdout.Fd())))
			colorMgr := output.NewColorManager()

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
						Above:         aboveMode,
						Now:           time.Now(),
						Mtime:         annotate.MtimeMode(mtimeFlagVar),
						ShowOperation: showOperation,
					})
				}

				// Strip managedFields from the YAML tree.
				managed.StripManagedFields(root)
			}

			if !foundManagedFields {
				msg := "Warning: no managedFields found. Did you use --show-managed-fields?"
				if term.IsTerminal(int(os.Stderr.Fd())) {
					msg = "\x1b[33m" + msg + "\x1b[0m" // orange/yellow
				}
				fmt.Fprintln(os.Stderr, msg)
			}

			// Encode YAML to buffer, then post-process (align + colorize).
			var buf bytes.Buffer
			if err := parser.EncodeDocuments(&buf, allDocs); err != nil {
				return err
			}

			result := output.FormatOutput(buf.String(), colorEnabled, colorMgr)
			_, err = fmt.Fprint(os.Stdout, result)
			return err
		},
	}

	rootCmd.Flags().Bool("above", false, "Place annotations on the line above each field instead of inline")
	rootCmd.Flags().Bool("show-operation", false, "Include operation type (apply, update) in annotations")
	rootCmd.Flags().Var(&colorFlagVar, "color", "Color output: auto, always, never")
	rootCmd.Flags().Var(&mtimeFlagVar, "mtime", "Timestamp display: relative, absolute, hide")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
