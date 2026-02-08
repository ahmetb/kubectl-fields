# Phase 2: Annotation Engine - Research

**Researched:** 2026-02-07
**Domain:** go.yaml.in/yaml/v3 comment injection, FieldsV1 parallel descent algorithm, Kubernetes managedFields semantics
**Confidence:** HIGH

## Summary

The annotation engine requires walking the FieldsV1 ownership trees (one per manager) in parallel with the YAML document tree, injecting comments on matching nodes. Research confirms that go.yaml.in/yaml/v3 provides `HeadComment`, `LineComment`, and `FootComment` fields on `yaml.Node` that survive encoding -- enabling both inline and above comment placement without post-processing.

The parallel descent algorithm is straightforward: for each managedFields entry, walk the FieldsV1 tree and match `f:` (field), `k:` (associative key for list items), and `v:` (set value for list items) prefixes against corresponding YAML nodes. The `.` key marks the node itself as managed. Leaf entries (`f:fieldName: {}` with empty mapping) annotate the field; non-leaf entries recurse into children.

**Primary recommendation:** Build an `internal/annotate` package with a `Walker` that accepts YAML root node + slice of `ManagedFieldsEntry`, walks all entries to build a node-to-annotation map, then injects comments in a second pass. Two-pass approach (collect then inject) avoids conflicts when multiple managers own different children of the same subtree.

## Standard Stack

The established libraries/tools for this phase:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go.yaml.in/yaml/v3 | v3.0.4 | YAML node manipulation with comments | Already in use; HeadComment/LineComment fields enable annotation |
| github.com/spf13/cobra | v1.10.2 | CLI flag handling (--above) | Already in use; add flag in existing command |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/stretchr/testify | v1.11.1 | Test assertions | Already in use for all tests |
| encoding/json | stdlib | Parse k: associative key JSON | Already used in ParseAssociativeKey |

No new dependencies needed for this phase.

## Architecture Patterns

### Recommended Project Structure
```
internal/
  annotate/
    annotate.go       # Core Annotate() function, comment injection
    annotate_test.go  # Unit tests with fixture-based golden file tests
    walker.go         # Parallel descent walker (FieldsV1 + YAML tree)
    walker_test.go    # Walker unit tests
  managed/            # (existing) ManagedFieldsEntry, extraction, stripping
  parser/             # (existing) YAML parsing, encoding
  timeutil/           # (existing) Relative time formatting
cmd/
  kubectl-fields/
    main.go           # Wire annotate into pipeline, add --above flag
```

### Pattern 1: Two-Pass Annotation (Collect, then Inject)

**What:** First pass walks all ManagedFieldsEntry FieldsV1 trees against the YAML tree, building a map of `*yaml.Node -> AnnotationInfo`. Second pass iterates the map and sets HeadComment or LineComment on each node.

**When to use:** Always. Multiple managers may own different fields in the same YAML subtree. A two-pass approach prevents a later manager's walk from overwriting an earlier one's comment. It also cleanly separates "what to annotate" from "how to annotate" (inline vs above).

**Why not single-pass:** A single-pass walker that immediately sets comments works BUT: if two managers own different children of the same parent, the `.` (dot) annotation on the parent could come from either. With collect-then-inject, conflicts are resolved before injection.

```go
// AnnotationInfo holds the ownership annotation for a single YAML node.
type AnnotationInfo struct {
    Manager     string
    Subresource string
    Time        time.Time
}

// Annotate walks the YAML tree, matches it against managedFields entries,
// and injects ownership comments on managed nodes.
func Annotate(root *yaml.Node, entries []managed.ManagedFieldsEntry, opts Options) {
    // Pass 1: Collect annotations
    annotations := make(map[*yaml.Node]AnnotationInfo)
    for _, entry := range entries {
        if entry.FieldsV1 == nil {
            continue
        }
        walkFieldsV1(root, entry.FieldsV1, entry, annotations)
    }

    // Pass 2: Inject comments
    for node, info := range annotations {
        comment := formatComment(info, opts)
        if opts.Above {
            node.HeadComment = comment
        } else {
            node.LineComment = comment
        }
    }
}
```

### Pattern 2: Parallel Descent Walker

**What:** Recursively walk the FieldsV1 tree. At each level, parse the FieldsV1 key prefix (`f:`, `k:`, `v:`, `.`), find the matching YAML node, and either annotate (leaf) or recurse (non-leaf).

**When to use:** This is the core algorithm.

```go
func walkFieldsV1(yamlNode *yaml.Node, fieldsNode *yaml.Node, entry managed.ManagedFieldsEntry, annotations map[*yaml.Node]AnnotationInfo) {
    if fieldsNode.Kind != yaml.MappingNode {
        return
    }

    for i := 0; i < len(fieldsNode.Content)-1; i += 2 {
        key := fieldsNode.Content[i].Value
        val := fieldsNode.Content[i+1]

        prefix, content := managed.ParseFieldsV1Key(key)

        switch prefix {
        case ".":
            // "." means this node itself is managed
            annotations[yamlNode] = annotationFrom(entry)

        case "f":
            // f:fieldName - find the field in the YAML mapping
            if yamlNode.Kind != yaml.MappingNode {
                continue
            }
            targetKey, targetVal := findMappingField(yamlNode, content)
            if targetKey == nil {
                continue
            }
            if isLeaf(val) {
                // Leaf: annotate the field
                annotations[targetVal] = annotationFrom(entry) // for inline
                // OR annotations[targetKey] for above mode -- decided at injection
            } else {
                // Non-leaf: recurse into children
                walkFieldsV1(targetVal, val, entry, annotations)
            }

        case "k":
            // k:{"name":"nginx"} - find list item by associative key
            if yamlNode.Kind != yaml.SequenceNode {
                continue
            }
            assocKey, err := managed.ParseAssociativeKey(content)
            if err != nil {
                continue
            }
            item := findSequenceItemByKey(yamlNode, assocKey)
            if item == nil {
                continue
            }
            walkFieldsV1(item, val, entry, annotations)

        case "v":
            // v:"value" - find list item by value (set semantics)
            if yamlNode.Kind != yaml.SequenceNode {
                continue
            }
            item := findSequenceItemByValue(yamlNode, content)
            if item == nil {
                continue
            }
            if isLeaf(val) {
                annotations[item] = annotationFrom(entry)
            }
        }
    }
}
```

### Pattern 3: Comment Placement Rules (verified empirically)

**What:** Rules for where to place comments on yaml.Node based on node type and mode.

**Inline mode (LineComment):**

| YAML Node Type | Where Comment Goes | Produces |
|----------------|-------------------|----------|
| Scalar value (in mapping) | `LineComment` on value node | `key: value # comment` |
| Mapping-valued key (annotations:) via `.` | `LineComment` on key node | `annotations: # comment` |
| Sequence-valued key (finalizers:) via `.` | `LineComment` on key node | `finalizers: # comment` |
| Scalar in sequence (finalizer value) via `v:` | `LineComment` on scalar node | `- value # comment` |
| List item via `k:` (`.` entry) | `HeadComment` on first key of item mapping | `- # comment\n  firstKey: val` |
| Block scalar (literal \|) | `LineComment` on value node | `key: \| # comment` |
| Flow-style empty mapping `{}` | `LineComment` on mapping value node | `resources: {} # comment` |
| Leaf `f:selector: {}` (opaque container) | `LineComment` on key node | `selector: # comment` |

**Above mode (HeadComment):**

| YAML Node Type | Where Comment Goes | Produces |
|----------------|-------------------|----------|
| Field in mapping | `HeadComment` on key node | `# comment\nkey: value` |
| Mapping-valued key via `.` | `HeadComment` on key node | `# comment\nannotations:` |
| Sequence-valued key via `.` | `HeadComment` on key node | `# comment\nfinalizers:` |
| Scalar in sequence via `v:` | `HeadComment` on scalar node | `# comment\n- value` |
| List item via `k:` (`.` entry) | `HeadComment` on mapping node | `# comment\n- firstKey: val` |

**Verified behavior (HIGH confidence):**
- `LineComment` on scalar value: `key: value # comment` -- WORKS
- `LineComment` on key node: `key: # comment` (when value is mapping/seq) -- WORKS
- `HeadComment` on key node: `# comment\nkey: value` -- WORKS, correctly indented
- `HeadComment` on first key of mapping-in-sequence: `- # comment\n  key: value` -- WORKS
- `HeadComment` on MappingNode in sequence: `# comment\n- key: value` -- WORKS but comment indentation may not match sequence indent with CompactSeqIndent
- `LineComment` on block scalar value: `key: | # comment` -- WORKS
- `LineComment` on flow-style empty mapping: `resources: {} # comment` -- WORKS

### Pattern 4: Annotation Target Node Selection

**What:** The key insight for determining WHICH yaml.Node to annotate in the two-pass system.

For inline mode, annotations need to target different nodes depending on context:
- `f:fieldName: {}` (leaf scalar) -> annotate the VALUE node of the key-value pair
- `f:fieldName: {children}` with `. {}` -> annotate the KEY node (produces `key: # comment`)
- `k:{...}` with `. {}` -> annotate the first key of the mapping item (produces `- # comment`)
- `v:"value": {}` -> annotate the scalar node in the sequence

For above mode, annotations always target the KEY node (or mapping node for k: items).

The clean approach: store both the key and value nodes in the annotation map, and let the injection pass decide which to use based on mode and node type.

```go
type AnnotationTarget struct {
    KeyNode   *yaml.Node  // The key in a mapping (for above mode, or inline on containers)
    ValueNode *yaml.Node  // The value in a mapping (for inline mode on scalars)
    Info      AnnotationInfo
}
```

### Anti-Patterns to Avoid

- **String-path intermediary:** Do NOT convert FieldsV1 keys to dot-separated path strings and then look up YAML nodes by path. This loses the tree structure, is error-prone with keys containing dots, and requires re-parsing. The parallel descent is simpler and more correct.
- **Modifying YAML tree during walk:** Do NOT add/remove nodes while walking. Only set comment fields on existing nodes.
- **Ignoring empty mapping values:** `f:selector: {}` means `selector` is a leaf field to annotate. Do NOT skip it because it has an empty mapping.
- **Confusing `.` with other entries:** The `.` key is special -- it marks the current node as managed. It is NOT a field name. Do not try to find a YAML key named `.`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| FieldsV1 key parsing | Custom string splitting | Existing `managed.ParseFieldsV1Key()` | Already built in Phase 1, handles edge cases |
| Associative key matching | Custom JSON comparison | Existing `managed.ParseAssociativeKey()` + reflect.DeepEqual or manual compare | Already built, handles multi-field keys |
| Relative time formatting | Manual duration formatting | Existing `timeutil.FormatRelativeTime()` | Already built and tested |
| YAML node lookup in mapping | Linear scan each time | `findMappingField()` helper (simple loop) | Simple enough, but centralize in one helper |
| Comment formatting | Ad-hoc string concatenation | Single `formatComment()` function | Centralizes `# manager (/subresource) (age)` format |

## Common Pitfalls

### Pitfall 1: Leaf vs Non-Leaf Confusion
**What goes wrong:** `f:selector: {}` is treated as a non-leaf (because `{}` is an empty mapping), so no annotation is applied. Or `f:annotations: {f:app: {}}` is treated as a leaf, annotating only the parent.
**Why it happens:** In FieldsV1, both leaves and non-leaves use mapping nodes as values. The difference is whether the mapping is empty.
**How to avoid:** `isLeaf(node) = node.Kind == yaml.MappingNode && len(node.Content) == 0`. An empty mapping means "this is a leaf field."
**Warning signs:** Fields like `selector`, `progressDeadlineSeconds`, `replicas` missing annotations.

### Pitfall 2: k: Match Requires Walking Into Sequence Items
**What goes wrong:** When FieldsV1 has `f:containers: {k:{"name":"nginx"}: {f:image: {}}}`, the walker must: find `containers` key in YAML mapping -> get its SequenceNode value -> iterate sequence items to find the MappingNode where `name: nginx` -> then walk into that mapping for `f:image`.
**Why it happens:** The FieldsV1 `f:containers` key maps to a YAML MappingNode key, but its VALUE is a SequenceNode. The `k:` prefix means "find an item in the sequence," not "find a key in a mapping."
**How to avoid:** When processing `f:` prefix and the FieldsV1 value contains `k:` entries, recognize that the YAML value will be a SequenceNode and pass it to the k: handler.
**Warning signs:** Container fields, port fields, condition fields missing annotations.

### Pitfall 3: Annotation on Correct Node for Inline vs Above
**What goes wrong:** For inline mode on `f:annotations: {.: {}}`, the comment goes on the wrong node (value mapping instead of key), producing broken YAML or invisible comments.
**Why it happens:** The `.` entry means "annotate this node," but the node to annotate differs between inline and above modes, and between scalar values and container values.
**How to avoid:** Use the AnnotationTarget pattern with both key and value nodes. For inline mode: scalars get LineComment on value, containers get LineComment on key. For above mode: always HeadComment on key.
**Warning signs:** Comments appearing in wrong positions, missing comments on mapping/sequence-valued fields.

### Pitfall 4: v: Set Value Matching
**What goes wrong:** `v:"example.com/foo"` fails to match the YAML scalar `example.com/foo` because the JSON string includes quotes.
**Why it happens:** The `v:` content is JSON-encoded. For strings, it includes the surrounding quotes: `"example.com/foo"`. Must JSON-decode before comparing.
**How to avoid:** Use `json.Unmarshal` on the v: content, then compare the decoded value to the YAML scalar's Value field. For strings, the decoded value (without quotes) matches directly.
**Warning signs:** Finalizer values not getting annotations.

### Pitfall 5: Multiple Managers on Same Subtree
**What goes wrong:** Two managers own different fields in the same parent mapping. The second walker pass overwrites the first's annotation on the parent (if both have `.` entry).
**Why it happens:** In rare cases (like `deployment.kubernetes.io/revision` owned by kube-controller-manager while `kubectl.kubernetes.io/last-applied-configuration` is owned by kubectl), two managers own fields in `annotations:`. Both entries might have `. {}` in their `f:annotations` subtree.
**How to avoid:** The two-pass approach naturally handles this: the annotations map uses `*yaml.Node` as key, so the last writer wins for the parent's `.` annotation. This is acceptable since the parent annotation (`. {}`) is informational -- the individual field annotations are more important. Alternatively, concatenate multiple manager names.
**Warning signs:** Parent mapping annotations flickering between managers.

### Pitfall 6: HeadComment Indentation on Sequence Items (Above Mode)
**What goes wrong:** HeadComment on a MappingNode that is a child of a SequenceNode renders at incorrect indentation when CompactSeqIndent is enabled. The comment appears at the sequence's column, not at the item's content column.
**Why it happens:** go.yaml.in/yaml/v3's encoder does not adjust HeadComment indentation for CompactSeqIndent.
**How to avoid:** For k: match annotations in above mode, use HeadComment on the MappingNode (for the item-level annotation). Accept that the indentation may differ slightly from the ideal expected output. Alternatively, use HeadComment on the first key of the mapping (produces `- # comment` which is correctly indented but appears after the dash, not before it). The `- # comment` pattern is actually the more natural and readable output.
**Warning signs:** Comments at wrong indentation level in above mode output for list items.

## Code Examples

### Comment Format Function
```go
// formatComment creates the annotation comment string.
// Format: "manager (age)" or "manager (/subresource) (age)"
func formatComment(info AnnotationInfo, now time.Time) string {
    var parts []string
    parts = append(parts, info.Manager)
    if info.Subresource != "" {
        parts = append(parts, fmt.Sprintf("(/%s)", info.Subresource))
    }
    age := timeutil.FormatRelativeTime(now, info.Time)
    parts = append(parts, fmt.Sprintf("(%s)", age))
    return strings.Join(parts, " ")
}
```

### Finding a Mapping Field by Name
```go
// findMappingField locates a key-value pair in a MappingNode by key name.
// Returns the key node and value node, or nil, nil if not found.
func findMappingField(mapping *yaml.Node, fieldName string) (keyNode, valueNode *yaml.Node) {
    if mapping.Kind != yaml.MappingNode {
        return nil, nil
    }
    for i := 0; i < len(mapping.Content)-1; i += 2 {
        if mapping.Content[i].Value == fieldName {
            return mapping.Content[i], mapping.Content[i+1]
        }
    }
    return nil, nil
}
```

### Finding a Sequence Item by Associative Key
```go
// findSequenceItemByKey finds a MappingNode in a SequenceNode whose fields
// match the provided associative key values.
// For k:{"name":"nginx"}, finds the item where name == "nginx".
// For k:{"containerPort":80,"protocol":"TCP"}, finds the item matching both.
func findSequenceItemByKey(seq *yaml.Node, assocKey map[string]any) *yaml.Node {
    if seq.Kind != yaml.SequenceNode {
        return nil
    }
    for _, item := range seq.Content {
        if item.Kind != yaml.MappingNode {
            continue
        }
        if matchesAssociativeKey(item, assocKey) {
            return item
        }
    }
    return nil
}

func matchesAssociativeKey(mapping *yaml.Node, assocKey map[string]any) bool {
    for k, v := range assocKey {
        keyNode, valNode := findMappingField(mapping, k)
        if keyNode == nil {
            return false
        }
        // Compare YAML scalar value to JSON value
        if !matchValue(valNode.Value, v) {
            return false
        }
    }
    return true
}

func matchValue(yamlVal string, jsonVal any) bool {
    switch v := jsonVal.(type) {
    case string:
        return yamlVal == v
    case float64:
        // JSON numbers are float64; compare as string
        // Handle both "80" and "80.0"
        return yamlVal == fmt.Sprintf("%g", v)
    case bool:
        return yamlVal == fmt.Sprintf("%t", v)
    default:
        return false
    }
}
```

### Finding a Sequence Item by Value (v: prefix)
```go
// findSequenceItemByValue finds a scalar node in a SequenceNode whose value
// matches the JSON-decoded v: content.
func findSequenceItemByValue(seq *yaml.Node, jsonContent string) *yaml.Node {
    if seq.Kind != yaml.SequenceNode {
        return nil
    }
    var decoded any
    if err := json.Unmarshal([]byte(jsonContent), &decoded); err != nil {
        return nil
    }
    str, ok := decoded.(string)
    if !ok {
        return nil
    }
    for _, item := range seq.Content {
        if item.Kind == yaml.ScalarNode && item.Value == str {
            return item
        }
    }
    return nil
}
```

### Comment Injection (Inline Mode)
```go
func injectInlineComment(target AnnotationTarget, comment string) {
    info := target.Info
    valNode := target.ValueNode
    keyNode := target.KeyNode

    if valNode == nil {
        // Sequence scalar (v: match) - comment on the scalar itself
        keyNode.LineComment = comment
        return
    }

    switch valNode.Kind {
    case yaml.ScalarNode:
        // Simple scalar field: comment on value
        valNode.LineComment = comment
    case yaml.MappingNode, yaml.SequenceNode:
        // Container field (annotations:, finalizers:, etc.)
        // For the `. {}` entry: comment goes on the KEY node
        keyNode.LineComment = comment
    }
}
```

### Comment Injection for k: Match Items (Inline Mode)
```go
// For k: associative key matches with `. {}`, the item itself is managed.
// In inline mode, place HeadComment on the first key of the mapping item.
// This produces the `- # comment` pattern.
func injectItemComment(item *yaml.Node, comment string) {
    if item.Kind == yaml.MappingNode && len(item.Content) >= 2 {
        // HeadComment on first key produces: - # comment
        item.Content[0].HeadComment = comment
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| gopkg.in/yaml.v3 | go.yaml.in/yaml/v3 | 2024 (canonical URL) | Same codebase, new import path; already using v3.0.4 |
| FieldsV1 as JSON blob | FieldsV1 as *yaml.Node | Phase 1 decision | Enables direct parallel descent without re-parsing |
| Path-string intermediary | Parallel descent | Roadmap decision | Simpler, avoids path-encoding ambiguities |

**No deprecated APIs in use.** All go.yaml.in/yaml/v3 Node comment fields are current and stable.

## Open Questions

1. **Duplicate `.` annotations from multiple managers on same parent**
   - What we know: Rare but possible (e.g., two managers both have `. {}` in `f:annotations`)
   - What's unclear: Should we show both managers or last-writer-wins?
   - Recommendation: Last-writer-wins is fine. The individual field annotations (f:) matter more than the parent container annotation (`.`). In practice, the `.` entry is redundant with the individual fields.

2. **HeadComment indentation for k: items in above mode**
   - What we know: HeadComment on MappingNode inside SequenceNode with CompactSeqIndent does not properly indent the comment.
   - What's unclear: Whether this is a go-yaml bug or intended behavior.
   - Recommendation: For above mode k: match, put HeadComment on the MappingNode (the seq item). Accept minor indentation difference from ideal output. The expected output files may need adjustment to match go-yaml's actual behavior.

3. **Expected output files as golden tests**
   - What we know: `testdata/1_deployment_inline.out` and `1_deployment_above.out` exist as expected outputs.
   - What's unclear: Whether they were generated from actual tool output or hand-crafted as design specs.
   - Recommendation: Use them as reference but adjust timestamps and minor formatting differences as needed. Create a test helper that injects a fixed `now` time for deterministic timestamps.

## Sources

### Primary (HIGH confidence)
- go.yaml.in/yaml/v3 `go doc` output -- Node struct with HeadComment/LineComment/FootComment fields
- Empirical testing of go.yaml.in/yaml/v3 v3.0.4 comment encoding behavior (7 test programs run locally)
- Existing codebase: `internal/managed/extract.go`, `internal/managed/fieldsv1.go`, `internal/parser/parser.go`
- Kubernetes FieldsV1 format observed from `testdata/1_deployment.yaml` real-world data

### Secondary (MEDIUM confidence)
- Expected output files `testdata/1_deployment_inline.out` and `testdata/1_deployment_above.out` -- believed to be design specs

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all libraries already in use, no new dependencies
- Architecture: HIGH - parallel descent algorithm empirically verified with real FieldsV1 data, comment placement verified with go-yaml
- Comment placement: HIGH - 7 test programs run against actual go.yaml.in/yaml/v3 v3.0.4 encoding
- Pitfalls: HIGH - discovered through empirical testing (HeadComment indentation issue, v: JSON decoding, leaf detection)

**Research date:** 2026-02-07
**Valid until:** 2026-03-07 (stable domain, no expected changes)
