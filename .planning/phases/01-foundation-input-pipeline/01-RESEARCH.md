# Phase 1: Foundation + Input Pipeline - Research

**Researched:** 2026-02-07
**Domain:** Go YAML processing, Kubernetes managedFields parsing, kubectl plugin I/O
**Confidence:** HIGH

## Summary

Phase 1 establishes the parsing foundation and stdin-to-stdout pipeline for kubectl-fields. Users pipe `kubectl get -o yaml --show-managed-fields` through the tool and get back clean YAML with `managedFields` stripped, proving the round-trip fidelity of the YAML processing before any annotation logic is added in Phase 2.

The primary technical risk is **YAML round-trip fidelity**: go-yaml v3 explicitly documents that "the content when re-encoded will not have its original textual representation preserved." However, a critical finding of this research is that `go.yaml.in/yaml/v3` v3.0.4 includes `CompactSeqIndent()` -- a method that matches kubectl's sequence indentation style. Combined with `SetIndent(2)`, this should produce output very close to kubectl's formatting. Round-trip testing with real kubectl output is mandatory day-one validation.

Secondary findings: (1) The predecessor tool (ahmetb/kubectl-fields) uses `sigs.k8s.io/structured-merge-diff/v4/fieldpath` for FieldsV1 parsing -- a lightweight library (~4 dependencies, no heavy k8s imports) that handles all edge cases (multi-field `k:` keys, `v:` literals, `.` markers). Rolling our own parser would risk missing edge cases. However, the project roadmap explicitly chose to parse FieldsV1 as YAML nodes directly rather than pulling in structured-merge-diff. (2) The predecessor tool does NOT handle multi-document or List kind input. (3) The predecessor tool does NOT use `CompactSeqIndent()`, resulting in output with different indentation from the input.

**Primary recommendation:** Use `go.yaml.in/yaml/v3` v3.0.4 with `SetIndent(2)` + `CompactSeqIndent()` for encoding. Parse FieldsV1 from YAML node keys directly (avoiding structured-merge-diff dependency per roadmap decision). Build round-trip fidelity tests on day one with real kubectl output.

## Standard Stack

The established libraries/tools for this phase:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go.yaml.in/yaml/v3` | v3.0.4 | YAML parsing, Node tree manipulation, encoding | Official YAML org fork. `yaml.Node` type provides `HeadComment`, `LineComment`, `FootComment` for comment injection. `CompactSeqIndent()` matches kubectl's sequence style. API-identical to archived `gopkg.in/yaml.v3` so all documentation applies. |
| `github.com/spf13/cobra` | v1.10.2 | CLI framework, flag parsing | De facto standard for kubectl plugins. Provides `--help`, shell completions, version subcommand for free. Even for a single-command tool, the cost is negligible. |
| Go standard `encoding/json` | stdlib | JSON parsing for `k:` key content | FieldsV1 `k:` prefix keys contain JSON objects (e.g., `k:{"name":"nginx"}`). Use `json.Unmarshal` from stdlib to parse them. No external library needed. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/stretchr/testify` | v1.11.1 | Test assertions (`assert`, `require`) | Every test file. Use `require` for fatal preconditions, `assert` for non-fatal checks. Do NOT use `suite` or `mock` packages. |
| `gotest.tools/v3/golden` | v3.5.2 | Golden file testing | YAML-in/YAML-out round-trip tests. Store input in `testdata/*.yaml`, expected output in `testdata/*.golden`. Use `-update` flag to regenerate. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `go.yaml.in/yaml/v3` | `goccy/go-yaml` v1.19.2 | Higher YAML test suite coverage, actively maintained, AST-level access with CommentMap. But: completely different API, less ecosystem documentation, unnecessary complexity for this use case. Only consider if go-yaml v3 round-trip fidelity proves insufficient. |
| `go.yaml.in/yaml/v3` | `go.yaml.in/yaml/v4` (v4.0.0-rc.4) | New active development branch of go-yaml. But: still Release Candidate (not stable), does NOT expose HeadComment/LineComment/FootComment in public API, breaks the core comment injection approach. Do NOT use v4. |
| Custom FieldsV1 parser | `sigs.k8s.io/structured-merge-diff/v4/fieldpath` | Production-proven parser used by Kubernetes itself and the predecessor tool. Handles all edge cases (multi-field `k:` keys, `v:` literals, `.` markers). Lightweight (~4 deps). But: roadmap explicitly chose to walk FieldsV1 as YAML nodes directly. Custom parsing is ~50 lines of code for `f:`, `k:`, `v:`, `.` prefix handling. |
| Custom FieldsV1 parser | `k8s.io/apimachinery` | Used by predecessor for ManagedFieldsEntry typed parsing. But: massive dependency tree pulling in the entire Kubernetes API machinery. Explicitly excluded in STACK.md and PROJECT.md. |
| Custom time formatting | `github.com/hako/durafmt` | Used by predecessor for relative timestamps. But: unmaintained since June 2021. Custom formatting is ~30 lines. |

**Installation:**
```bash
go mod init github.com/rewanthtammana/kubectl-fields
go get go.yaml.in/yaml/v3@v3.0.4
go get github.com/spf13/cobra@v1.10.2
go get github.com/stretchr/testify@v1.11.1
go get gotest.tools/v3@v3.5.2
```

## Architecture Patterns

### Recommended Project Structure
```
kubectl-fields/
  cmd/
    kubectl-fields/
      main.go              # Entrypoint, cobra root command setup
  internal/
    parser/
      parser.go            # YAML stdin parsing, multi-doc, List kind
      parser_test.go
    managed/
      extract.go           # ManagedFields extraction from YAML nodes
      extract_test.go
      strip.go             # ManagedFields stripping from YAML nodes
      strip_test.go
      fieldsv1.go          # FieldsV1 prefix parsing (f:, k:, v:, .)
      fieldsv1_test.go
    timeutil/
      relative.go          # Relative timestamp formatting
      relative_test.go
  testdata/
    0_no_managedFields.yaml
    1_deployment.yaml
    1_deployment_inline.out
    1_deployment_above.out
    roundtrip/             # Round-trip fidelity test fixtures
      deployment.yaml
      configmap.yaml
      service.yaml
  go.mod
  go.sum
  Makefile
```

**Why `internal/` instead of `pkg/`:** All packages are internal to the binary. No external consumers. `internal/` enforces this at the Go compiler level.

### Pattern 1: YAML Document Pipeline (Multi-doc + List Kind)
**What:** Read stdin, decode into `[]*yaml.Node` document trees, detect and unwrap List kind, process each resource independently.
**When to use:** Entry point for all YAML processing.

```go
// Source: go.yaml.in/yaml/v3 Decoder API
func ParseDocuments(r io.Reader) ([]*yaml.Node, error) {
    decoder := yaml.NewDecoder(r)
    var docs []*yaml.Node
    for {
        var doc yaml.Node
        err := decoder.Decode(&doc)
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("YAML parse error: %w", err)
        }
        docs = append(docs, &doc)
    }
    return docs, nil
}

// Unwrap List kind into individual resources
func UnwrapListKind(doc *yaml.Node) []*yaml.Node {
    root := doc.Content[0] // DocumentNode -> MappingNode
    kind, ok := getMapValue(root, "kind")
    if !ok || kind != "List" {
        return []*yaml.Node{doc} // not a List, return as-is
    }
    items, ok := getMapValueNode(root, "items")
    if !ok || items.Kind != yaml.SequenceNode {
        return []*yaml.Node{doc} // no items, return as-is
    }
    // Each item becomes its own document
    var resources []*yaml.Node
    for _, item := range items.Content {
        docNode := &yaml.Node{
            Kind:    yaml.DocumentNode,
            Content: []*yaml.Node{item},
        }
        resources = append(resources, docNode)
    }
    return resources
}
```

### Pattern 2: MappingNode Pair Iteration
**What:** Walk MappingNode.Content in key/value pairs (step by 2), because Content is a flat slice of alternating keys and values.
**When to use:** Every time you traverse a MappingNode -- finding fields, stripping managedFields, extracting entries.

```go
// Source: go.yaml.in/yaml/v3 Node type docs
// MappingNode.Content = [key0, val0, key1, val1, key2, val2, ...]
func getMapValueNode(mapping *yaml.Node, key string) (*yaml.Node, bool) {
    for i := 0; i < len(mapping.Content)-1; i += 2 {
        if mapping.Content[i].Value == key {
            return mapping.Content[i+1], true
        }
    }
    return nil, false
}

// Splice out a key-value pair from a MappingNode
func removeMapKey(mapping *yaml.Node, key string) bool {
    for i := 0; i < len(mapping.Content)-1; i += 2 {
        if mapping.Content[i].Value == key {
            mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
            return true
        }
    }
    return false
}
```

### Pattern 3: FieldsV1 Prefix Parsing
**What:** Parse `f:`, `k:`, `v:`, `.`, `i:` prefixes from FieldsV1 MappingNode key strings.
**When to use:** Walking FieldsV1 ownership trees to extract paths.

```go
// Parse a FieldsV1 key string into its prefix type and content
func parseFieldsV1Key(key string) (prefix string, content string) {
    if key == "." {
        return ".", ""
    }
    // Split at first ":"
    idx := strings.IndexByte(key, ':')
    if idx < 0 {
        return "", key // malformed, treat as literal
    }
    return key[:idx], key[idx+1:]
}

// For k: prefix, the content is a JSON object
// e.g., k:{"name":"nginx"} -> prefix="k", content=`{"name":"nginx"}`
// e.g., k:{"containerPort":80,"protocol":"TCP"} -> multi-field key
func parseAssociativeKey(jsonStr string) (map[string]interface{}, error) {
    var result map[string]interface{}
    err := json.Unmarshal([]byte(jsonStr), &result)
    return result, err
}
```

### Pattern 4: Encoder Configuration for kubectl Output Fidelity
**What:** Configure the yaml.Encoder to match kubectl's output formatting as closely as possible.
**When to use:** The output rendering step.

```go
// Source: go.yaml.in/yaml/v3 v3.0.4 Encoder API
func encodeDocuments(w io.Writer, docs []*yaml.Node) error {
    enc := yaml.NewEncoder(w)
    enc.SetIndent(2)          // kubectl uses 2-space indent
    enc.CompactSeqIndent()    // kubectl uses compact sequence indent (- at parent level)
    defer enc.Close()
    for _, doc := range docs {
        if err := enc.Encode(doc); err != nil {
            return err
        }
    }
    return nil
}
```

**CRITICAL:** `CompactSeqIndent()` was added in the go.yaml.in/yaml/v3 fork (synced from sigs.k8s.io/yaml patches). It is NOT available in the original archived `gopkg.in/yaml.v3` v3.0.1. This is one of the key reasons to use the fork.

### Pattern 5: ManagedFields Entry Extraction from YAML Nodes
**What:** Walk the YAML Node tree to extract managedFields entries without importing k8s.io/apimachinery.
**When to use:** Extracting ownership information before stripping managedFields.

```go
type ManagedFieldsEntry struct {
    Manager     string
    Operation   string
    Subresource string
    Time        time.Time
    APIVersion  string
    FieldsV1    *yaml.Node // The raw YAML MappingNode of the ownership tree
}

func extractManagedFields(root *yaml.Node) ([]ManagedFieldsEntry, error) {
    metadata, ok := getMapValueNode(root, "metadata")
    if !ok {
        return nil, nil // no metadata
    }
    mfNode, ok := getMapValueNode(metadata, "managedFields")
    if !ok {
        return nil, nil // no managedFields
    }
    if mfNode.Kind != yaml.SequenceNode {
        return nil, fmt.Errorf("managedFields is not a sequence")
    }
    var entries []ManagedFieldsEntry
    for _, item := range mfNode.Content {
        entry, err := parseManagedFieldEntry(item)
        if err != nil {
            return nil, err
        }
        entries = append(entries, entry)
    }
    return entries, nil
}

func parseManagedFieldEntry(node *yaml.Node) (ManagedFieldsEntry, error) {
    var entry ManagedFieldsEntry
    // node is a MappingNode with keys: manager, operation, time, fieldsV1, etc.
    if v, ok := getMapValue(node, "manager"); ok {
        entry.Manager = v
    }
    if v, ok := getMapValue(node, "operation"); ok {
        entry.Operation = v
    }
    if v, ok := getMapValue(node, "subresource"); ok {
        entry.Subresource = v
    }
    if v, ok := getMapValue(node, "time"); ok {
        t, err := time.Parse(time.RFC3339, v)
        if err == nil {
            entry.Time = t
        }
    }
    if v, ok := getMapValueNode(node, "fieldsV1"); ok {
        entry.FieldsV1 = v
    }
    return entry, nil
}
```

### Anti-Patterns to Avoid
- **Marshaling to Go structs:** Never `yaml.Unmarshal` the resource into typed Go structs. Loses formatting, comments, ordering. Work exclusively with `*yaml.Node`.
- **String-based YAML manipulation:** Never use regex or string replacement to modify YAML. Use the Node API.
- **Importing k8s.io/apimachinery:** Heavyweight dependency. Parse managedFields from YAML nodes directly (~40 lines of code).
- **Modifying Node.Style:** Never change a node's Style field. This causes quoting changes (e.g., `"yes"` becomes `yes`, changing semantics). Only add comments.
- **Using yaml.v4:** The v4 RC does not expose HeadComment/LineComment/FootComment in its public API. Breaks the entire comment injection approach.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CLI framework | Custom flag parsing | cobra + pflag | Free `--help`, shell completions, version subcommand. ~1 file of setup. |
| YAML parsing | Custom YAML tokenizer | `go.yaml.in/yaml/v3` Node API | YAML is complex (quoting, multiline, anchors, tags). Thousands of edge cases. |
| Test assertions | Manual `if got != want` | testify `assert`/`require` | Better error messages, cleaner test code. |
| Golden file comparison | Manual file read + compare | `gotest.tools/v3/golden` | Handles CRLF normalization, `-update` flag for regeneration. |

**Key insight:** The temptation to avoid dependencies is strong in Go, but YAML parsing and CLI frameworks are genuinely complex problems. The YAML spec alone is 200+ pages. Use established libraries for these.

The one area where hand-rolling IS appropriate: **FieldsV1 prefix parsing** and **relative time formatting**. These are simple, domain-specific, and the alternatives (structured-merge-diff, durafmt) either add unnecessary dependency weight or are unmaintained.

## Common Pitfalls

### Pitfall 1: Round-Trip Formatting Changes
**What goes wrong:** Decoding YAML into `yaml.Node` and re-encoding it produces output with different indentation, quoting, or multiline string formatting. go-yaml v3 docs explicitly state: "the content when re-encoded will not have its original textual representation preserved."
**Why it happens:** go-yaml v3's encoder applies its own formatting preferences. Without `CompactSeqIndent()`, sequences get extra indentation. Without `SetIndent(2)`, the default 4-space indent differs from kubectl's 2-space.
**How to avoid:**
  1. Always use `enc.SetIndent(2)` and `enc.CompactSeqIndent()` to match kubectl's format.
  2. Never modify `Node.Style` -- only add comments.
  3. Build round-trip fidelity tests on day one: decode, encode (without changes), compare to original.
  4. Test with real kubectl output from Deployments, ConfigMaps, Services, and CRDs.
  5. Accept that some formatting differences are unavoidable (go-yaml may normalize some whitespace). Document known differences.
**Warning signs:** Round-trip test produces diffs. Multiline strings (`|`) change to quoted strings. Quoted values (`"yes"`) lose quotes.

### Pitfall 2: Missing CompactSeqIndent
**What goes wrong:** The encoder outputs sequences with non-compact indentation, making the output look visually different from kubectl's output even though it's semantically identical.
**Why it happens:** The default `DefaultSeqIndent()` adds extra indentation for sequence items. kubectl uses compact style.
**How to avoid:** Always call `enc.CompactSeqIndent()` before encoding.
**Warning signs:** Input has `containers:\n- name: nginx` but output has `containers:\n  - name: nginx`.

**Detailed comparison:**

kubectl output (compact, our target):
```yaml
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
```

go-yaml default (non-compact, WRONG):
```yaml
spec:
  containers:
    - name: nginx
      image: nginx:1.14.2
```

go-yaml with CompactSeqIndent (matches kubectl):
```yaml
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
```

### Pitfall 3: YAML 1.1 Boolean/Null Value Corruption
**What goes wrong:** Values like `"yes"`, `"on"`, `"true"`, `"null"` lose their quotes during round-trip, changing from strings to booleans/nulls.
**Why it happens:** go-yaml v3 has YAML 1.1 compatibility. If Node.Style is modified or new nodes are created without explicit style, quoted values may become unquoted.
**How to avoid:** Never modify `Node.Style`. Only add `HeadComment` or `LineComment`. When the only change is comment addition, Style should be preserved through decode/encode.
**Warning signs:** ConfigMap data `enabled: "yes"` becomes `enabled: yes`. Status fields `status: "True"` become `status: true`.

### Pitfall 4: MappingNode Off-by-One Errors
**What goes wrong:** `MappingNode.Content` is a flat slice alternating keys and values: `[key0, val0, key1, val1, ...]`. Iterating with `i++` instead of `i+=2` reads values as keys or vice versa.
**Why it happens:** Go has no built-in map iteration for YAML nodes. The flat slice is non-obvious.
**How to avoid:** Always iterate with `i += 2`. Always access `Content[i]` (key) and `Content[i+1]` (value) as a pair. Build helper functions (`getMapValueNode`, `removeMapKey`) that enforce pair semantics.
**Warning signs:** Key "managedFields" not found (looking at value nodes). Annotations on wrong fields. Panic on odd-length Content slice.

### Pitfall 5: FieldsV1 `k:` Key JSON Parsing
**What goes wrong:** The `k:` prefix key content is a JSON object that can contain commas, colons, quotes, and multiple fields. Naive string splitting breaks.
**Why it happens:** `k:{"containerPort":80,"protocol":"TCP"}` has commas inside the JSON. Splitting on `,` would break the key spec.
**How to avoid:** Use `json.Unmarshal` to parse the content after the `k:` prefix. The content is always valid JSON.
**Warning signs:** Containers, ports, or env vars show incorrect matching. Panic when encountering multi-field `k:` keys.

### Pitfall 6: Multi-Document Separator Handling
**What goes wrong:** `---` separators between YAML documents are lost or duplicated in output.
**Why it happens:** go-yaml v3 Decoder consumes `---` implicitly. The Encoder adds `---` before the second+ document automatically. But the first document has no separator, and trailing `---` or `...` markers are handled differently.
**How to avoid:** Let the Encoder handle separators naturally. When encoding multiple documents via the same Encoder, it automatically adds `---` before the second+ document. Do NOT manually add separators.
**Warning signs:** Output has `---` before the first document (shouldn't). Or missing `---` between documents.

### Pitfall 7: Empty managedFields Detection
**What goes wrong:** Users forget `--show-managed-fields` flag. The tool receives YAML with no managedFields and silently outputs unchanged YAML without any useful feedback.
**Why it happens:** Since Kubernetes 1.21+, `kubectl get -o yaml` strips managedFields by default.
**How to avoid:** Detect when no managedFields are found across ALL documents. Print a helpful warning to stderr: `"Warning: no managedFields found. Did you use --show-managed-fields?"`. Output the YAML unchanged (don't error).
**Warning signs:** Users report "the tool does nothing."

## Code Examples

### Example 1: Complete Round-Trip Test Pattern
```go
// Source: gotest.tools/v3/golden pattern
func TestRoundTrip(t *testing.T) {
    input, err := os.ReadFile("testdata/roundtrip/deployment.yaml")
    require.NoError(t, err)

    // Parse
    docs, err := ParseDocuments(bytes.NewReader(input))
    require.NoError(t, err)

    // Encode without changes
    var buf bytes.Buffer
    err = encodeDocuments(&buf, docs)
    require.NoError(t, err)

    // Compare (byte-for-byte except trailing newline)
    expected := strings.TrimRight(string(input), "\n")
    actual := strings.TrimRight(buf.String(), "\n")
    assert.Equal(t, expected, actual, "round-trip should preserve formatting")
}
```

### Example 2: Relative Timestamp Formatting
```go
// Custom implementation (no external dependency)
func FormatRelativeTime(now, then time.Time) string {
    d := now.Sub(then)
    if d < 0 {
        return "just now"
    }
    switch {
    case d < time.Minute:
        return fmt.Sprintf("%ds ago", int(d.Seconds()))
    case d < time.Hour:
        m := int(d.Minutes())
        s := int(d.Seconds()) % 60
        if s > 0 {
            return fmt.Sprintf("%dm%ds ago", m, s)
        }
        return fmt.Sprintf("%dm ago", m)
    case d < 24*time.Hour:
        h := int(d.Hours())
        m := int(d.Minutes()) % 60
        if m > 0 {
            return fmt.Sprintf("%dh%dm ago", h, m)
        }
        return fmt.Sprintf("%dh ago", h)
    case d < 30*24*time.Hour:
        days := int(d.Hours()) / 24
        return fmt.Sprintf("%dd ago", days)
    case d < 365*24*time.Hour:
        months := int(d.Hours()) / (30 * 24)
        return fmt.Sprintf("%dmo ago", months)
    default:
        years := int(d.Hours()) / (365 * 24)
        return fmt.Sprintf("%dy ago", years)
    }
}
```

### Example 3: ManagedFields Stripping
```go
// Strip managedFields from a resource's metadata MappingNode
func StripManagedFields(root *yaml.Node) bool {
    if root.Kind != yaml.MappingNode {
        return false
    }
    metadata, ok := getMapValueNode(root, "metadata")
    if !ok || metadata.Kind != yaml.MappingNode {
        return false
    }
    return removeMapKey(metadata, "managedFields")
}
```

### Example 4: List Kind Detection and Processing
```go
func ProcessDocument(doc *yaml.Node) ([]*yaml.Node, error) {
    if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
        return nil, fmt.Errorf("invalid document node")
    }
    root := doc.Content[0]
    if root.Kind != yaml.MappingNode {
        return []*yaml.Node{doc}, nil // pass through non-mapping documents
    }

    kind, _ := getMapValue(root, "kind")
    if kind == "List" {
        return UnwrapListKind(doc), nil
    }
    return []*yaml.Node{doc}, nil
}
```

### Example 5: FieldsV1 Tree Walking
```go
type FieldOwnership struct {
    Manager     string
    Subresource string
    Time        time.Time
}

// Walk a FieldsV1 MappingNode and collect all leaf paths
func WalkFieldsV1(node *yaml.Node, prefix []string, info FieldOwnership, collect func(path []string, info FieldOwnership)) {
    if node.Kind != yaml.MappingNode {
        return
    }
    for i := 0; i < len(node.Content)-1; i += 2 {
        key := node.Content[i].Value
        val := node.Content[i+1]

        pfx, content := parseFieldsV1Key(key)
        switch pfx {
        case "f":
            path := append(append([]string{}, prefix...), content)
            if isEmptyMapping(val) {
                collect(path, info) // leaf: this field is owned
            } else {
                WalkFieldsV1(val, path, info, collect) // recurse
            }
        case "k":
            keySpec, _ := parseAssociativeKey(content)
            // Encode key spec as path segment for matching
            path := append(append([]string{}, prefix...), formatKeySegment(keySpec))
            if hasDotMember(val) {
                collect(path, info) // the list item itself is owned
            }
            WalkFieldsV1(val, path, info, collect) // recurse into item's fields
        case "v":
            path := append(append([]string{}, prefix...), "v:"+content)
            collect(path, info)
        case ".":
            // Self marker -- the current container is owned
            // Already handled by parent's k: processing
        }
    }
}

func isEmptyMapping(node *yaml.Node) bool {
    return node.Kind == yaml.MappingNode && len(node.Content) == 0
}

func hasDotMember(node *yaml.Node) bool {
    if node.Kind != yaml.MappingNode {
        return false
    }
    for i := 0; i < len(node.Content)-1; i += 2 {
        if node.Content[i].Value == "." {
            return true
        }
    }
    return false
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gopkg.in/yaml.v3` (archived) | `go.yaml.in/yaml/v3` (official fork) | April 2025 | Must use fork. Same API, different import path. Fork adds `CompactSeqIndent()`. |
| `k8s.io/apimachinery` for managedFields parsing | Direct YAML Node tree walking | Architecture decision | Avoids massive dependency. ~40 lines of custom code. |
| `sigs.k8s.io/structured-merge-diff` for FieldsV1 | Direct prefix parsing from YAML node keys | Architecture decision | Avoids dependency. ~50 lines of custom code. Risk: may miss edge cases. |
| `github.com/hako/durafmt` for time formatting | Custom implementation | durafmt unmaintained since 2021 | ~30 lines of custom code. Full control over format. |
| Default go-yaml encoder settings | `SetIndent(2)` + `CompactSeqIndent()` | go.yaml.in/yaml/v3 v3.0.2+ | Matches kubectl's output format. Critical for round-trip fidelity. |

**Deprecated/outdated:**
- `gopkg.in/yaml.v3`: Archived April 2025. Use `go.yaml.in/yaml/v3` instead. API-identical.
- `go.yaml.in/yaml/v4`: RC status, does not expose comment fields in public API. Do not use for this project.
- `github.com/hako/durafmt`: Unmaintained since 2021. Implement custom.

## Predecessor Tool Analysis

The predecessor tool (ahmetb/kubectl-fields) provides valuable reference patterns but has important limitations:

**What it does well:**
- Uses `yaml.Node` API for comment injection (LineComment/HeadComment)
- Uses `sigs.k8s.io/structured-merge-diff/v4/fieldpath` for robust FieldsV1 parsing
- Sentinel-based color injection (`<BEGIN>...<END>` markers in comments, post-processed)
- Comment alignment via `aligningPrinter` with 60-char tolerance

**What it does NOT handle (our improvements):**
- Multi-document YAML (`---` separated) -- explicitly rejects with "error validating object"
- List kind (`kind: List`) -- explicitly rejects with "only single objects are supported"
- CompactSeqIndent -- does not use it, output indentation differs from input
- Custom color palette -- hardcodes single red color for all annotations
- `--color auto/always/never` -- only has implicit TTY detection

**Key architectural differences from our approach:**
1. Predecessor uses `k8s.io/apimachinery` (unstructured.Unstructured) to parse the resource and extract managedFields. We parse directly from YAML nodes.
2. Predecessor uses `fieldpath.Set.FromJSON()` to parse FieldsV1. We parse from YAML node keys directly (the `f:`, `k:`, `v:` prefixes appear as YAML mapping keys in kubectl output).
3. Predecessor flattens all managed fields into a flat list of paths, then annotates each path independently. We plan to use parallel descent (walking FieldsV1 tree and YAML tree simultaneously) in Phase 2.

## Critical Validation: Round-Trip Fidelity

This is the #1 risk for Phase 1. The following test matrix MUST pass:

| Input | Expected Behavior | Risk Level |
|-------|-------------------|------------|
| Deployment with managedFields | Strip managedFields, output identical otherwise | HIGH |
| ConfigMap with quoted values (`"yes"`, `"true"`) | Quotes preserved exactly | HIGH |
| Multi-line annotation (literal block `\|`) | Block style preserved | MEDIUM |
| Flow-style mapping (`{key: value}`) | Flow style preserved | MEDIUM |
| Multi-document YAML (`---` separated) | Each doc processed, separators preserved | LOW |
| List kind (`kind: List`) | Items unwrapped, processed, output as separate docs | LOW |
| YAML without managedFields | Pass through unchanged, stderr warning | LOW |

**Round-trip acceptance criteria:**
- Decode input -> Encode output (with NO modifications) should produce byte-identical output to input (modulo trailing newline).
- Decode input -> Strip managedFields -> Encode output should produce output identical to input EXCEPT the managedFields section is removed.

**Known go-yaml v3 limitations that may cause differences:**
1. Trailing whitespace on empty lines may be added/removed
2. Literal block scalars (`|`) may sometimes be re-encoded as quoted strings (go-yaml issue #1041)
3. Indentation indicators in block scalars may break (go-yaml issue #643)
4. Line comments on tagged nodes may shift position (go-yaml issue #1047)

**Mitigation strategy:**
If round-trip produces unacceptable differences, consider a hybrid approach: use go-yaml Node tree for understanding structure, but operate on the raw text lines using node Line/Column positions as a guide. This is more complex but gives perfect formatting preservation.

## Open Questions

1. **CompactSeqIndent exact behavior with nested sequences**
   - What we know: `CompactSeqIndent()` matches kubectl's top-level sequence indentation.
   - What's unclear: Does it handle deeply nested sequences (e.g., containers -> ports -> items) correctly at every level? The existing test data shows 3 levels of nesting.
   - Recommendation: Build round-trip tests with the actual test fixture `1_deployment.yaml` in the first task. If nesting behavior is wrong, investigate encoder internals.

2. **Literal block scalar round-trip**
   - What we know: go-yaml issue #1041 reports that literal style (`|`) may be ignored during encoding.
   - What's unclear: Does this affect kubectl output? The test data has `kubectl.kubernetes.io/last-applied-configuration: |` which is a literal block.
   - Recommendation: Include a literal block test in the round-trip test suite. If it fails, consider workaround (explicitly set Style on affected nodes, or use the hybrid text approach).

3. **Empty map suppression in annotations**
   - What we know: `resources: {}` and `securityContext: {}` in the test data should NOT be annotated per the expected output (line 46: `resources: {}` has no comment, line 87: `securityContext: {}` has no comment in the above-mode output).
   - What's unclear: The inline output (line 46) also shows no annotation on `resources: {}`. Is this because the predecessor tool suppresses annotations on empty flow-style maps? Or because there's no ownership?
   - Recommendation: Check the FieldsV1 tree in the test data. If `f:resources: {}` is present (leaf ownership), then the expected behavior IS to suppress annotation on empty flow maps. Build this rule into Phase 2 annotation logic, but document it now.

4. **Parallel descent vs path string intermediary**
   - What we know: Roadmap chose parallel descent. Predecessor tool uses path string intermediary (flatten to paths, then walk YAML independently).
   - What's unclear: The parallel descent approach has not been validated with real data. It requires the FieldsV1 tree structure to mirror the YAML tree structure exactly.
   - Recommendation: Phase 1 builds the extraction and stripping components. Phase 2 builds the parallel descent walker. Phase 1 should validate that FieldsV1 trees from the test data DO mirror the YAML structure by logging/inspecting both trees.

## Sources

### Primary (HIGH confidence)
- `go.yaml.in/yaml/v3` v3.0.4 documentation: https://pkg.go.dev/go.yaml.in/yaml/v3 -- Node API, Encoder API, CompactSeqIndent, round-trip warning
- `go.yaml.in/yaml/v3` source code (yaml/go-yaml GitHub): CompactSeqIndent at line 278-286 of yaml.go -- verified exists in v3.0.4
- go-yaml issue #1041: Encode does not respect LiteralStyle -- confirmed open, affects literal blocks
- go-yaml issue #643: Indentation indicator bug -- confirmed open, affects block scalars
- go-yaml issue #1047: Line comment with tag unmarshalled to wrong node -- confirmed open
- ahmetb/kubectl-fields source code (yaml.go, annotate.go, managedfields.go, printer.go, aligningprinter.go, main.go) -- read via `gh api`, verified architecture patterns
- ahmetb/kubectl-fields go.mod -- confirmed uses `gopkg.in/yaml.v3` + `k8s.io/apimachinery` + `structured-merge-diff/v4`
- `sigs.k8s.io/structured-merge-diff/v4/fieldpath` v4.7.0 documentation: https://pkg.go.dev/sigs.k8s.io/structured-merge-diff/v4/fieldpath -- PathElement, Set, FromJSON API
- `structured-merge-diff/v4` go.mod: lightweight deps (go-cmp, json-iterator, randfill, sigs.k8s.io/yaml)
- Project test data: `testdata/1_deployment.yaml`, `testdata/1_deployment_inline.out`, `testdata/1_deployment_above.out`, `testdata/0_no_managedFields.yaml`
- Project planning docs: ROADMAP.md, PROJECT.md, REQUIREMENTS.md, STATE.md, STACK.md, ARCHITECTURE.md, PITFALLS.md

### Secondary (MEDIUM confidence)
- `goccy/go-yaml` GitHub: higher YAML test suite coverage, CommentMap, AST-level access -- evaluated as fallback alternative
- go-yaml v4 (v4.0.0-rc.4): RC status, does not expose comment fields in public API -- confirmed unsuitable
- kustomize/kyaml source: YAML 1.1 boolean compatibility notes -- relevant for value corruption pitfall

### Tertiary (LOW confidence)
- None -- all critical findings verified against primary sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified from official pkg.go.dev docs, versions confirmed
- Architecture: HIGH -- patterns verified against predecessor tool source code and go-yaml API docs
- Pitfalls: HIGH -- all critical pitfalls verified against go-yaml issue tracker and predecessor limitations
- Round-trip fidelity: MEDIUM -- CompactSeqIndent discovery is promising but must be validated with actual round-trip tests in implementation

**Research date:** 2026-02-07
**Valid until:** 2026-03-07 (30 days -- stack is stable, go-yaml v3 is frozen/security-fixes-only)
