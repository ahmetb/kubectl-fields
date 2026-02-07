# Architecture Patterns

**Domain:** kubectl plugin / Kubernetes YAML annotation tool
**Researched:** 2026-02-07
**Overall confidence:** HIGH (verified against Kubernetes source, official docs, and test fixtures)

## FieldsV1 Format: Complete Reference

Before describing the architecture, this section documents the FieldsV1 format in detail, because correct handling of this format is the central technical challenge.

### Managed Fields Entry Structure

Each entry in `.metadata.managedFields[]` has:

```yaml
- manager: "kubectl-client-side-apply"   # Who owns these fields
  operation: Update                       # "Apply" or "Update"
  apiVersion: apps/v1                     # API version context
  time: "2024-04-10T00:44:50Z"           # When last applied
  fieldsType: FieldsV1                    # Always "FieldsV1"
  fieldsV1: { ... }                       # The ownership tree (see below)
  subresource: status                     # Optional: "status", etc.
```

### Prefix Semantics

FieldsV1 is a JSON tree where keys use prefixes to indicate their type:

| Prefix | Meaning | Example | Matches |
|--------|---------|---------|---------|
| `f:` | **Field name** in a map/struct | `f:metadata` | The `metadata` key |
| `k:` | **Associative list key** - identifies a list item by key field(s) | `k:{"name":"nginx"}` | List item where `name == "nginx"` |
| `v:` | **Set value** - identifies a list item by its primitive value | `v:"example.com/foo"` | List item equal to `"example.com/foo"` |
| `i:` | **Index** - identifies a list item by position (rare) | `i:0` | First list item |
| `.` | **Self** - indicates the current node itself is owned | `.: {}` | The containing map/list itself |

### Tree Structure Rules

1. **Leaf ownership**: `f:fieldName: {}` means "this manager owns `fieldName`" and the value is a leaf (empty object = no children).

2. **Nested ownership**: `f:fieldName: { f:child: {} }` means "this manager owns `child` inside `fieldName`" but does NOT necessarily own `fieldName` itself.

3. **Self-plus-children**: When a node has BOTH `.` and `f:` children, it means the manager owns the node itself AND specific children:
   ```json
   "f:labels": {
     ".": {},           // Manager owns the labels map itself
     "f:app": {}        // Manager also owns the "app" key within labels
   }
   ```

4. **Associative list items**: `k:{"name":"nginx"}` identifies a list item whose `name` field equals `"nginx"`. The JSON after `k:` is always a JSON object with the key field(s) sorted alphabetically.

5. **Set items**: `v:"example.com/foo"` identifies a list item whose value equals `"example.com/foo"`. The JSON after `v:` is the literal JSON value.

### Real Example Walkthrough

Given this FieldsV1 tree (from test data):

```json
{
  "f:spec": {
    "f:template": {
      "f:spec": {
        "f:containers": {
          "k:{\"name\":\"nginx\"}": {
            ".": {},
            "f:image": {},
            "f:name": {},
            "f:ports": {
              ".": {},
              "k:{\"containerPort\":80,\"protocol\":\"TCP\"}": {
                ".": {},
                "f:containerPort": {},
                "f:protocol": {}
              }
            },
            "f:resources": {}
          }
        }
      }
    }
  }
}
```

This maps to YAML paths:

| FieldsV1 Path | YAML Path | Note |
|----------------|-----------|------|
| `f:spec.f:template.f:spec.f:containers` | `spec.template.spec.containers` | Container list |
| `...f:containers.k:{"name":"nginx"}` | `spec.template.spec.containers[name=nginx]` | The nginx container item |
| `...k:{"name":"nginx"}.f:image` | `spec.template.spec.containers[name=nginx].image` | The image field |
| `...k:{"name":"nginx"}.f:ports.k:{"containerPort":80,"protocol":"TCP"}` | `...containers[name=nginx].ports[containerPort=80,protocol=TCP]` | Specific port entry |
| `...k:{"name":"nginx"}.f:resources` | `...containers[name=nginx].resources` | Resources (empty map) |

### The Empty Map Case

When a YAML field's value is an empty map `{}` (like `resources: {}` or `securityContext: {}`), the annotation is **suppressed** even if FieldsV1 claims ownership. Rationale: the field carries no useful content; annotating it would be noise.

### The Dot (`.`) Entry

The `.: {}` entry means "this node itself is a member of the set." It appears in two contexts:

1. **Map containers**: When a manager owns the map itself, not just its keys. For example, `f:labels: { ".": {}, "f:app": {} }` means the manager owns the `labels` map AND the `app` key. Without `.`, it would only own `app`.

2. **List item identity**: Inside `k:` or `v:` entries, `.` marks ownership of the list item as a whole. For example, `k:{"name":"nginx"}: { ".": {}, "f:image": {} }` means the manager owns the container item itself AND its `image` field.

For annotation purposes, `.` ownership means the parent container node gets annotated.

---

## Recommended Architecture

### Pipeline Overview

```
stdin (YAML bytes)
       |
       v
  [1. Input Reader] ---- reads all stdin bytes
       |
       v
  [2. YAML Parser] ---- yaml.v3 Node tree(s)
       |
       v
  [3. ManagedFields Extractor] ---- extracts managedFields entries, returns OwnershipMap
       |
       v
  [4. ManagedFields Stripper] ---- removes managedFields from YAML Node tree
       |
       v
  [5. Path Mapper / Tree Walker] ---- walks FieldsV1 trees, builds YAML-path -> FieldInfo map
       |
       v
  [6. Comment Injector] ---- walks YAML Node tree, attaches comments using OwnershipMap
       |
       v
  [7. Output Renderer] ---- encodes annotated Node tree to stdout, handles color
       |
       v
     stdout
```

### Component Boundaries

| Component | Responsibility | Input | Output | Communicates With |
|-----------|---------------|-------|--------|-------------------|
| **Input Reader** | Read all bytes from stdin; detect multi-doc YAML | `io.Reader` (stdin) | `[]byte` | YAML Parser |
| **YAML Parser** | Parse bytes into `yaml.v3` Node tree(s); handle multi-doc and List kinds | `[]byte` | `[]*yaml.Node` (one per document/resource) | ManagedFields Extractor, Stripper |
| **ManagedFields Extractor** | Walk each resource's Node tree to find `managedFields`, deserialize entries | `*yaml.Node` (resource root) | `[]ManagedFieldsEntry` per resource | Path Mapper |
| **ManagedFields Stripper** | Remove `managedFields` key-value pair from each resource's metadata node | `*yaml.Node` (resource root) | mutated `*yaml.Node` (in-place) | (same node passed to Comment Injector) |
| **Path Mapper** | Walk FieldsV1 trees from all entries, resolve `f:`, `k:`, `v:` paths to YAML node paths, build ownership map | `[]ManagedFieldsEntry` | `OwnershipMap` (YAML path -> FieldInfo) | Comment Injector |
| **Comment Injector** | Walk YAML Node tree, look up each node's path in OwnershipMap, set HeadComment or LineComment | `*yaml.Node`, `OwnershipMap`, mode (inline/above) | mutated `*yaml.Node` (in-place) | Output Renderer |
| **Output Renderer** | Encode annotated Node tree(s) to writer; handle color codes for TTY | `[]*yaml.Node`, color config | YAML text to `io.Writer` | (terminal) |
| **Time Formatter** | Format timestamps as relative ("5m ago") or absolute | `time.Time`, reference `time.Time` | `string` | Comment Injector |
| **Color Manager** | Assign consistent colors to manager names; detect TTY | manager name | ANSI color code | Output Renderer |

### Data Flow Details

#### Step 1-2: Input Reading and Parsing

```go
// Read all stdin
data, err := io.ReadAll(os.Stdin)

// Parse into Node trees (handles multi-doc YAML via yaml.Decoder)
var docs []*yaml.Node
decoder := yaml.NewDecoder(bytes.NewReader(data))
for {
    var doc yaml.Node
    err := decoder.Decode(&doc)
    if err == io.EOF { break }
    docs = append(docs, &doc)
}
```

Multi-document YAML (`---` separated) produces multiple documents. Each document's root is a `DocumentNode` with `Content[0]` being the actual resource (a `MappingNode`).

For `kind: List` resources, the `items` sequence contains the individual resources, which must each be processed independently.

#### Step 3: ManagedFields Extraction

Walk the MappingNode to find `metadata` -> `managedFields`. The `managedFields` value is a SequenceNode where each item is a MappingNode with keys `manager`, `operation`, `time`, `subresource`, `fieldsV1`, etc.

The `fieldsV1` value is itself a MappingNode whose structure IS the ownership tree. This is critical: **in the YAML representation from `kubectl get -o yaml`, the FieldsV1 tree is already a YAML mapping, not a JSON string**. The keys are literally `f:metadata`, `k:{"name":"nginx"}`, etc.

This means extraction can walk the YAML Node tree directly rather than JSON-parsing a string.

```go
type ManagedFieldsEntry struct {
    Manager     string
    Operation   string
    Subresource string
    Time        time.Time
    FieldsV1    *yaml.Node  // The raw YAML mapping node of the ownership tree
}
```

#### Step 4: ManagedFields Stripping

After extraction, remove the `managedFields` key-value pair from the `metadata` MappingNode. Since MappingNode stores alternating key/value in `Content`, find the key node where `Value == "managedFields"` and splice out both the key and its following value.

#### Step 5: Path Mapping (Core Algorithm)

This is the hardest component. It must:

1. **Walk each FieldsV1 tree recursively**, building a path as it descends.
2. **Translate prefixed keys** into YAML path segments:
   - `f:fieldName` -> descend into field named `fieldName`
   - `k:{"name":"nginx"}` -> find list item where field `name` equals `"nginx"`
   - `v:"example.com/foo"` -> find list item whose scalar value equals `"example.com/foo"`
   - `.` -> mark current path as owned (the container itself)
3. **Record ownership**: For each leaf path (including `.`), store:
   ```go
   type FieldInfo struct {
       Manager     string
       Subresource string
       Time        time.Time
   }
   ```

The OwnershipMap is keyed by a normalized YAML path representation:

```go
// OwnershipMap maps YAML node paths to their field ownership info.
// Key is a structured path like ["spec", "template", "spec", "containers", "[name=nginx]", "image"]
type OwnershipMap map[string]FieldInfo
```

**Path representation**: Use a string-serialized path where each segment is separated by a delimiter. For `k:` list items, encode as `[key=value]` or use a structural representation. Since paths must be matchable during the Comment Injector walk, the path format must be deterministic and constructible from both the FieldsV1 walk and the YAML Node walk.

Recommended approach: Use a `[]PathSegment` type internally:

```go
type PathSegment struct {
    Field string                    // For f: prefix
    Key   map[string]interface{}    // For k: prefix (e.g., {"name": "nginx"})
    Value interface{}               // For v: prefix (e.g., "example.com/foo")
    Index int                       // For i: prefix (rare)
}
```

#### Step 6: Comment Injection

Walk the YAML Node tree in parallel with path construction. At each node, look up the current path in the OwnershipMap. If found, format the annotation and attach it:

- **Inline mode** (default): Set `node.LineComment` on the YAML value node
- **Above mode**: Set `node.HeadComment` on the YAML value node

Annotation format: `# manager-name (age)` or `# manager-name (/subresource) (age)`.

**List item handling**: For list items matched by `k:` or `v:`, the annotation goes on:
- **Inline mode**: The first key node or scalar node of the list item (the `-` line)
- **Above mode**: The first content node of the list item (using HeadComment)

**Empty map suppression**: If a node is a MappingNode with FlowStyle and zero Content (i.e., `{}`), skip annotation.

#### Step 7: Output Rendering

```go
encoder := yaml.NewEncoder(os.Stdout)
encoder.SetIndent(2)
for _, doc := range docs {
    encoder.Encode(doc)
}
encoder.Close()
```

For color output: post-process the encoded YAML string, replacing `# manager-name` patterns with ANSI-colored versions when stdout is a TTY. This is simpler than trying to inject ANSI codes into yaml.v3 comments because the encoder would not handle them correctly.

Alternative approach: write a custom encoder wrapper that intercepts comment output and wraps them in color codes. However, post-processing is more straightforward and less coupled to yaml.v3 internals.

---

## Key Design Decisions

### Use yaml.v3 Node API, Not Marshal/Unmarshal

**Why**: Marshal/Unmarshal loses comments, ordering, and formatting. The Node API preserves the AST structure and allows direct comment manipulation via `HeadComment`, `LineComment`, `FootComment` fields.

**Consequence**: All YAML manipulation happens at the `*yaml.Node` level. We never unmarshal into Go structs for the resource itself (only for managedFields entries if convenient).

### Walk FieldsV1 as YAML Nodes, Not JSON

**Why**: When `kubectl get -o yaml` outputs managedFields, the `fieldsV1` value is represented as YAML mappings, not a JSON string. The `f:`, `k:`, `v:` prefixes appear as literal YAML mapping keys. Walking the YAML Node tree directly avoids a marshal-to-JSON-then-reparse step.

**How**: The FieldsV1 node is a MappingNode. Iterate its Content in key/value pairs. Each key's `Value` string starts with `f:`, `k:`, `v:`, `i:`, or is `.`. Parse the prefix, recurse into the value (another MappingNode or empty `{}`).

### Two-Pass Walk for Comment Injection

**Why**: Separating path mapping from comment injection keeps concerns clean. The Path Mapper builds a complete OwnershipMap from ALL managedFields entries, resolving any overlapping ownership (last-writer or most-recent timestamp wins). The Comment Injector then does a single walk of the YAML tree, looking up each path.

**Alternative considered**: Single-pass walk that interleaves FieldsV1 and YAML traversal. Rejected because multiple managedFields entries can own different fields at the same YAML path level, requiring all entries to be processed before annotation.

### Path Matching Strategy

Two options for correlating FieldsV1 paths to YAML nodes:

**Option A (Recommended): Concurrent tree walk**. Walk the FieldsV1 tree and the YAML tree simultaneously. For each `f:fieldName` in the FieldsV1 tree, find the corresponding key in the YAML MappingNode's Content. For `k:{...}`, scan the YAML SequenceNode's items to find the matching list item. This avoids building an intermediate path representation entirely.

**Option B: Build path map, then walk YAML**. First pass: walk all FieldsV1 trees, producing string paths. Second pass: walk YAML tree, building the current path, look up in the map. Simpler but requires a canonical path string format.

Option A is recommended because it directly correlates nodes without serialization overhead and handles the `k:` matching naturally (scan list items for key match). The FieldsV1 tree structure mirrors the YAML structure, making parallel descent straightforward.

---

## Patterns to Follow

### Pattern 1: Recursive Parallel Descent

**What**: Walk the FieldsV1 ownership tree and the YAML Node tree simultaneously, descending into matching children.

**When**: During path mapping and comment injection (combined into one walk when using Option A).

**Pseudocode**:
```go
func annotateNode(yamlNode *yaml.Node, fieldsV1Node *yaml.Node, info FieldInfo, mode CommentMode) {
    // fieldsV1Node is a MappingNode with f:, k:, v:, . keys
    for each (key, value) in fieldsV1Node.Content pairs {
        prefix, name := parsePrefix(key.Value)
        switch prefix {
        case "f":
            // Find "name" key in yamlNode (must be MappingNode)
            childKey, childValue := findMappingKey(yamlNode, name)
            if value is empty {} {
                // Leaf: annotate childValue
                attachComment(childKey, childValue, info, mode)
            } else {
                // Recurse into child
                annotateNode(childValue, value, info, mode)
            }
        case "k":
            // Parse JSON key object, find matching list item in yamlNode (must be SequenceNode)
            keyFields := parseJSON(name)
            listItem := findListItemByKeys(yamlNode, keyFields)
            if "." in value's children {
                // Annotate the list item itself
                attachListItemComment(listItem, info, mode)
            }
            // Recurse into list item's fields
            annotateNode(listItem, value, info, mode)
        case "v":
            // Parse JSON value, find matching list item by scalar value
            targetValue := parseJSON(name)
            listItem := findListItemByValue(yamlNode, targetValue)
            attachComment(nil, listItem, info, mode)
        case ".":
            // Mark current node as owned (handled by parent)
            // Already processed by parent's f: handler
        }
    }
}
```

### Pattern 2: Comment Format Abstraction

**What**: A formatter function that takes FieldInfo and returns the comment string.

**When**: Any time a comment needs to be generated.

```go
func formatAnnotation(info FieldInfo, now time.Time, opts FormatOptions) string {
    var parts []string
    parts = append(parts, info.Manager)
    if info.Subresource != "" {
        parts[0] += " (/" + info.Subresource + ")"
    }
    if !opts.NoTime {
        if opts.AbsoluteTime {
            parts = append(parts, info.Time.Format(time.RFC3339))
        } else {
            parts = append(parts, formatRelativeTime(now, info.Time)+" ago")
        }
    }
    return strings.Join(parts, " ")
}
```

### Pattern 3: Ownership Conflict Resolution

**What**: When multiple managers claim the same field (from different `fieldsV1` entries), resolve by picking the most recent entry (by timestamp).

**When**: Building the OwnershipMap. In practice, Kubernetes guarantees single ownership per field, but the code should handle edge cases gracefully.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Marshaling to Go Structs

**What**: Using `json.Unmarshal` or `yaml.Unmarshal` into typed Go structs for the Kubernetes resource.

**Why bad**: Loses YAML formatting, comments, ordering, and makes it impossible to inject comments. Also requires importing Kubernetes API types, bloating the binary.

**Instead**: Work exclusively with `yaml.v3` Node trees.

### Anti-Pattern 2: String-Based YAML Manipulation

**What**: Using regex or string replacement to inject comments into YAML text.

**Why bad**: Fragile. YAML has complex quoting, multi-line strings, flow vs block style, and indentation rules. String manipulation will break on edge cases.

**Instead**: Use the yaml.v3 Node API to set `HeadComment` and `LineComment` fields, then let the encoder handle rendering.

### Anti-Pattern 3: Importing k8s.io/apimachinery

**What**: Importing the full Kubernetes apimachinery library to parse managedFields.

**Why bad**: Massive dependency tree. The managedFields format is simple enough to parse directly from the YAML Node tree. The `f:`, `k:`, `v:` prefix parsing is ~20 lines of code.

**Instead**: Parse FieldsV1 prefixes directly from YAML node key strings. Define a minimal `ManagedFieldsEntry` struct locally.

### Anti-Pattern 4: Building Complete Path Strings

**What**: Constructing full path strings like `"spec.template.spec.containers[name=nginx].image"` and using them as map keys.

**Why bad**: Requires canonical serialization, escaping, and matching. Error-prone for complex `k:` keys with multiple fields.

**Instead**: Use the parallel descent pattern (Pattern 1) where the FieldsV1 tree directly navigates the YAML tree without path string intermediaries.

---

## Component Details and Edge Cases

### Multi-Document YAML

`kubectl get` can return `---`-separated documents. Use `yaml.NewDecoder` in a loop to parse each document separately. Process each independently.

### List Kind Wrapping

`kubectl get` with multiple resources returns:
```yaml
apiVersion: v1
kind: List
items:
  - apiVersion: apps/v1
    kind: Deployment
    ...
```

Detect `kind: List` and iterate over the `items` sequence, processing each item as a separate resource.

### Comment Placement on List Items

For **inline mode** with list items matched by `k:` or `v:`:
- The `.` annotation (item ownership) goes as a LineComment on the `-` token, which in yaml.v3 is represented by the first content node of the sequence item.
- If the list item is a mapping, the `-` shares a line with the first key-value pair. The comment goes on that first key node's LineComment.

For **above mode** with list items:
- The `.` annotation goes as a HeadComment on the list item's first content node.

### The `k:` Key Matching Algorithm

To find a list item matching `k:{"name":"nginx"}`:
1. The parent node must be a SequenceNode.
2. Each item in the sequence is a MappingNode.
3. For each item, scan its key-value pairs for `name: nginx`.
4. For multi-key matches like `k:{"containerPort":80,"protocol":"TCP"}`, ALL key fields must match.

The JSON inside `k:` has keys sorted alphabetically. Values can be strings, numbers, or booleans.

### The `v:` Value Matching Algorithm

To find a list item matching `v:"example.com/foo"`:
1. The parent node must be a SequenceNode.
2. Each item is a ScalarNode.
3. Find the item whose `Value` equals the JSON-decoded string.

### Empty Map Suppression Rule

When the YAML value node is:
- Kind == MappingNode
- Style == FlowStyle (rendered as `{}`)
- len(Content) == 0

Then do NOT attach an annotation. The field has no meaningful content to annotate.

This rule applies regardless of FieldsV1 ownership claims.

---

## Suggested Build Order

Dependencies between components dictate this build order:

### Phase 1: Foundation (no dependencies)
1. **Time Formatter** - Pure function, easily testable. Format `time.Time` to relative strings like "5m ago", "2d ago", "1y3mo ago".
2. **FieldsV1 Prefix Parser** - Parse `f:`, `k:`, `v:`, `.` prefixes from YAML Node key strings. Return structured PathSegment.

### Phase 2: Core Pipeline (depends on Phase 1)
3. **YAML Parser** - Read stdin, produce `[]*yaml.Node` documents. Handle multi-doc and List kind.
4. **ManagedFields Extractor** - Walk YAML Node tree to extract managed fields entries with their FieldsV1 sub-trees.
5. **ManagedFields Stripper** - Remove managedFields from metadata MappingNode.

### Phase 3: Annotation Engine (depends on Phases 1-2)
6. **Parallel Descent Walker + Comment Injector** - The core algorithm. Walk FieldsV1 tree and YAML tree together, resolve `k:`/`v:` list matches, attach comments. Handles both inline and above modes.

### Phase 4: Output (depends on Phase 3)
7. **Output Renderer** - Encode annotated Node trees with yaml.v3 Encoder.
8. **Color Manager** - Assign colors to managers, detect TTY, post-process output for ANSI codes.

### Phase 5: CLI Integration (depends on Phase 4)
9. **Main / CLI** - Parse flags (`--above`, `--no-color`, `--absolute-time`, `--no-time`), wire pipeline, handle errors.

### Dependency Graph

```
Time Formatter ─────────────────┐
                                v
FieldsV1 Prefix Parser ──> Parallel Descent Walker + Comment Injector
                                ^                          |
YAML Parser ──────────────────>─┤                          v
                                |                   Output Renderer
ManagedFields Extractor ──────>─┤                          |
                                |                          v
ManagedFields Stripper ────────>┘                   Color Manager
                                                          |
                                                          v
                                                    Main / CLI
```

### Build Order Rationale

- **Phase 1 first** because Time Formatter and Prefix Parser are pure functions with no dependencies, easily testable in isolation, and needed by everything downstream.
- **Phase 2 before 3** because the annotation engine needs parsed YAML nodes and extracted managedFields to work with.
- **Phase 3 is the core risk** -- the parallel descent algorithm with `k:` and `v:` matching is the hardest part. Build it after the surrounding infrastructure is solid.
- **Phase 4 after 3** because output rendering can only be tested with annotated nodes.
- **Phase 5 last** because CLI is just wiring.

---

## Scalability Considerations

| Concern | Small resource (50 fields) | Large CRD (500+ fields) | Multi-resource List (100 items) |
|---------|---------------------------|--------------------------|--------------------------------|
| Memory | Negligible | ~10MB for Node trees | ~100MB, process items sequentially |
| Parse time | <1ms | ~10ms | ~100ms |
| Walk time | <1ms | ~5ms | ~50ms per item |
| Output time | <1ms | ~5ms | ~500ms total |

The tool processes stdin to stdout with no persistent state. Memory usage is bounded by the size of the input YAML. No optimization needed for typical Kubernetes resources.

---

## Sources

- Kubernetes Server-Side Apply documentation: https://kubernetes.io/docs/reference/using-api/server-side-apply/ (HIGH confidence)
- `kubernetes-sigs/structured-merge-diff` source code for PathElement serialization: `fieldpath/serialize-pe.go`, `fieldpath/serialize.go`, `fieldpath/element.go` (HIGH confidence)
- `kubernetes/apimachinery` ManagedFieldsEntry type definition: `pkg/apis/meta/v1/types.go` (HIGH confidence)
- `gopkg.in/yaml.v3` Node API documentation: https://pkg.go.dev/gopkg.in/yaml.v3 (HIGH confidence)
- Test fixture analysis from `testdata/1_deployment.yaml` and expected outputs (HIGH confidence -- first-party project data)
