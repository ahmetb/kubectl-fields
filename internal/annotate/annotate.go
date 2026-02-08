package annotate

import (
	"fmt"
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"github.com/rewanthtammana/kubectl-fields/internal/timeutil"
	"go.yaml.in/yaml/v3"
)

// Options configures annotation behaviour.
type Options struct {
	Above bool      // true = HeadComment above field key, false = LineComment inline
	Now   time.Time // current time for relative timestamps (enables deterministic tests)
}

// Annotate injects ownership comments into a YAML resource tree based on
// managedFields entries. The root should be the resource MappingNode (not a
// DocumentNode -- the caller must unwrap it first).
//
// The function operates in two passes:
//  1. Collect: walk each entry's FieldsV1 tree in parallel with the YAML tree,
//     building a map of annotation targets keyed by ValueNode pointer.
//  2. Inject: for each target, set LineComment (inline) or HeadComment (above)
//     on the appropriate node.
func Annotate(root *yaml.Node, entries []managed.ManagedFieldsEntry, opts Options) {
	targets := make(map[*yaml.Node]AnnotationTarget)

	// Pass 1 -- Collect targets from all managed fields entries.
	for _, entry := range entries {
		if entry.FieldsV1 == nil {
			continue
		}
		walkFieldsV1(root, nil, entry.FieldsV1, entry, targets)
	}

	// Pass 2 -- Inject comments.
	for _, target := range targets {
		comment := formatComment(target.Info, opts.Now)
		injectComment(target, comment, opts.Above)
	}
}

// injectComment places a comment on the appropriate node based on mode and
// node kind.
func injectComment(target AnnotationTarget, comment string, above bool) {
	if above {
		// Above mode: HeadComment on key node (or value if no key).
		if target.KeyNode != nil {
			target.KeyNode.HeadComment = comment
		} else if target.ValueNode != nil {
			target.ValueNode.HeadComment = comment
		}
		return
	}

	// Inline mode: placement depends on value node kind.
	if target.ValueNode == nil {
		return
	}

	switch target.ValueNode.Kind {
	case yaml.ScalarNode:
		// Scalar value: comment at end of value line.
		// Handles both f: field values and v: set values (KeyNode==nil).
		target.ValueNode.LineComment = comment
	case yaml.MappingNode:
		if target.KeyNode == nil {
			// k: list item with dot marker (no parent key).
			// Place HeadComment on the first key of the mapping.
			// This renders as "- # comment\n  firstKey: val".
			if len(target.ValueNode.Content) > 0 {
				target.ValueNode.Content[0].HeadComment = comment
			}
		} else if isFlowEmpty(target.ValueNode) {
			// Empty flow-style mapping (e.g., "data: {}").
			target.ValueNode.LineComment = comment
		} else {
			// Container field with parent key.
			target.KeyNode.LineComment = comment
		}
	case yaml.SequenceNode:
		if target.KeyNode == nil {
			// k: list item that is a sequence (unusual but possible).
			// HeadComment on the sequence itself.
			target.ValueNode.HeadComment = comment
		} else if isFlowEmpty(target.ValueNode) {
			// Empty flow-style sequence (e.g., "conditions: []").
			target.ValueNode.LineComment = comment
		} else {
			// Container field with parent key.
			target.KeyNode.LineComment = comment
		}
	default:
		// Fallback: if KeyNode == ValueNode (dot on root-level, rare) or
		// unexpected kind, put on key.
		if target.KeyNode != nil {
			target.KeyNode.LineComment = comment
		}
	}
}

// isFlowEmpty returns true when a mapping or sequence node is empty and will
// render in flow style (e.g., "[]" or "{}"). go-yaml silently drops
// LineComment on the key node in this case, so the comment must go on the
// value node instead.
func isFlowEmpty(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	return (node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode) && len(node.Content) == 0
}

// formatComment builds the annotation string for a field.
// Format: "manager (age)" or "manager (/subresource) (age)".
// The returned string does NOT include the "# " prefix -- go-yaml adds that
// automatically when encoding HeadComment or LineComment.
func formatComment(info AnnotationInfo, now time.Time) string {
	age := timeutil.FormatRelativeTime(now, info.Time)
	if info.Subresource != "" {
		return fmt.Sprintf("%s (/%s) (%s)", info.Manager, info.Subresource, age)
	}
	return fmt.Sprintf("%s (%s)", info.Manager, age)
}
