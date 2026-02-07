# Domain Pitfalls

**Domain:** kubectl plugin for YAML annotation with managed field ownership
**Researched:** 2026-02-07
**Overall confidence:** HIGH (verified against go-yaml v3 docs, Kubernetes API docs, kustomize source, kubectl-neat issues)

---

## Critical Pitfalls

Mistakes that cause rewrites or fundamentally broken output.

### Pitfall 1: go-yaml v3 Re-encoding Destroys Original YAML Formatting

**What goes wrong:** Decoding YAML into `yaml.Node` and re-encoding it produces output that differs from the original input -- changed indentation, reordered keys, altered quoting styles, modified multiline string formatting. For a tool whose job is to *annotate* existing YAML, any formatting change is a bug visible to every user.

**Why it happens:** go-yaml v3 explicitly documents this: "The content when re-encoded will not have its original textual representation preserved. An effort is made to render the data pleasantly." The encoder does not respect the original `Line`/`Column` positions. It applies its own indentation and style preferences. Known open issues include: literal style (`|`) being ignored during encoding (#1041), indentation indicators breaking with certain content (#643), and leading spaces in multiline strings failing round-trip (#1071).

**Consequences:** Users pipe `kubectl get -o yaml` through the tool and get back YAML that *looks different* even without considering the added comments. This destroys trust immediately. Users doing `diff` against original output see noise everywhere.

**Warning signs:**
- Round-trip tests (decode then encode without changes) produce different output
- Multiline strings (like `description` fields, ConfigMap data) lose their block style
- Quoted values lose or gain quotes unexpectedly

**Prevention:**
- Build round-trip fidelity tests from day one: decode, encode, compare byte-for-byte (ignoring only the comments you added)
- Set `Node.Style` preservation as a hard requirement -- verify the encoder respects existing styles
- Test with real-world `kubectl get -o yaml --show-managed-fields` output from Deployments, ConfigMaps, CRDs, and Services
- Consider operating on raw text lines with YAML node positions as a guide rather than pure decode/encode if round-trip fidelity is insufficient

**Phase:** Must be validated in Phase 1 (core pipeline). If go-yaml v3 cannot round-trip adequately, the entire architecture must change.

**Confidence:** HIGH -- verified from go-yaml v3 official docs and open issues.

---

### Pitfall 2: FieldsV1 Key Format Parsing Is More Complex Than It Appears

**What goes wrong:** The `f:`, `k:`, `v:` prefix format in FieldsV1 looks simple but contains edge cases that break naive string parsing:
1. **`k:{}` keys with multiple fields:** `k:{"containerPort":8080,"protocol":"TCP"}` -- the JSON inside can contain commas, making naive splitting fail
2. **`k:{}` keys with string values containing special chars:** `k:{"key":"value with : colons"}` -- JSON strings can contain the `:` character
3. **Nested empty containers:** `f:metadata: {}` vs `f:metadata:` followed by children -- both are valid and mean different things (leaf ownership vs partial ownership)
4. **The `.` (dot) marker:** `.: {}` appears inside `k:{}` entries to indicate the value itself is managed, separate from its subfields
5. **`v:` prefix with literal values:** `v:"kubernetes.io/pvc-protection"` for set-type lists where items are identified by their actual value, not a key

**Why it happens:** FieldsV1 is essentially a trie encoded as nested JSON objects. The keys are serialized PathElements from the `structured-merge-diff` library. Developers typically parse the outer `f:fieldName` pattern and miss the `k:` and `v:` variants entirely, or parse `k:` keys with string splitting instead of proper JSON parsing.

**Consequences:** Fields in associative lists (containers, ports, volumeMounts, env vars) are not matched to their YAML counterparts. These are the most interesting fields for ownership annotation -- the exact ones users want to see.

**Warning signs:**
- Containers, ports, or env vars show no ownership annotations
- Annotations appear on the list node but not individual items
- Crash or panic when encountering `k:{}` keys with multiple fields

**Prevention:**
- Use proper JSON parsing for `k:{}` key content (the part between `k:` and the trailing `:` is valid JSON)
- Use the `sigs.k8s.io/structured-merge-diff/v4/fieldpath` package to parse FieldsV1 into `Set` objects with proper `PathElement` types, rather than writing custom parsing
- Build test cases for every PathElement variant: `f:simple`, `k:{"name":"nginx"}`, `k:{"containerPort":8080,"protocol":"TCP"}`, `v:"literal-value"`, and `.`
- Include a test with a real Deployment's managedFields that exercises containers, ports, env, volumeMounts simultaneously

**Phase:** Core of Phase 1. The FieldsV1 parser is the foundation; if it is wrong, all annotations are wrong.

**Confidence:** HIGH -- verified against Kubernetes server-side apply documentation and structured-merge-diff API.

---

### Pitfall 3: Matching FieldsV1 Paths to YAML Node Positions Is Non-Trivial

**What goes wrong:** Even after correctly parsing FieldsV1 into field paths, mapping those paths to specific `yaml.Node` positions in the decoded YAML tree is error-prone. The YAML tree has `MappingNode` (key-value pairs in `Content[0], Content[1], Content[2], Content[3]...`) and `SequenceNode` (items in `Content[0], Content[1]...`), while FieldsV1 uses `f:name` for map keys and `k:{"name":"nginx"}` for list item identification. Bridging these two representations requires:
1. Walking mapping nodes in pairs (key at index `i`, value at index `i+1`)
2. For sequence nodes, scanning all items to find the one matching `k:{}` criteria
3. Handling the case where a `k:{}` key references multiple identifying fields

**Why it happens:** go-yaml v3 `MappingNode.Content` is a flat slice alternating keys and values, not a map. Developers often forget to step by 2 or confuse key nodes with value nodes. For `k:{}` matching, you must decode each list item's relevant fields and compare them against the key spec -- this requires partial node decoding.

**Consequences:** Comments are placed on the wrong YAML line, or worse, on the key node instead of the value node (or vice versa). For list items, the wrong container gets annotated.

**Warning signs:**
- Comments appear on `name:` instead of `nginx` (the value)
- In a list of containers, annotations from container A appear on container B
- Off-by-one errors where every annotation is shifted

**Prevention:**
- Write a dedicated YAML tree walker that explicitly handles MappingNode pairs and SequenceNode items
- For `k:{}` matching, build a function that takes a SequenceNode and a key spec, decodes relevant fields from each item, and returns the matching item's index
- Test with mappings that have many keys (10+) to catch off-by-one errors
- Test with lists containing items with similar but not identical keys

**Phase:** Phase 1 core. This is the bridge between FieldsV1 parsing and comment injection.

**Confidence:** HIGH -- verified from go-yaml v3 Node type documentation.

---

### Pitfall 4: go-yaml v3 Is Archived and Unmaintained

**What goes wrong:** The canonical `gopkg.in/yaml.v3` (go-yaml/yaml) repository was archived on April 1, 2025. Open bugs including comment handling issues, encoding edge cases, and multiline string problems will never be fixed. Building a tool on top of known bugs with no upstream fix path creates long-term maintenance burden.

**Why it happens:** The original maintainer ran out of time and was unable to transfer the project. The library has 7000+ stars and is deeply embedded in the Go ecosystem, so it is not disappearing, but it is frozen.

**Consequences:** Any bug you hit in go-yaml v3 becomes your bug to work around. Known unfixed issues include: line comments on tagged nodes being parsed to wrong nodes (#1047), literal style being ignored (#1041), and indentation indicator breakage (#643).

**Warning signs:**
- You encounter a go-yaml bug and find the issue already filed but unresolved
- Workaround code starts accumulating in your codebase
- Alternative libraries (goccy/go-yaml) surface but have different APIs

**Prevention:**
- Accept go-yaml v3 as a known-frozen dependency and plan for workarounds from the start
- Document every go-yaml v3 workaround with a link to the upstream issue
- Build an abstraction layer around the yaml.Node manipulation so you could theoretically swap the YAML library later
- Consider goccy/go-yaml as an alternative -- it is actively maintained, has higher YAML test suite coverage, and supports AST-level access and comment preservation. However, it has a completely different API and less ecosystem validation. Evaluate during Phase 1.
- Monitor for community forks of go-yaml/yaml that may emerge

**Phase:** Architecture decision in Phase 0/1. Evaluate both libraries before committing.

**Confidence:** HIGH -- go-yaml archive status verified directly on GitHub.

---

## Moderate Pitfalls

Mistakes that cause delays, incorrect output in edge cases, or technical debt.

### Pitfall 5: Comment Placement on Wrong Node Part (HeadComment vs LineComment vs FootComment)

**What goes wrong:** go-yaml v3 has three comment slots per node: `HeadComment` (line before), `LineComment` (end of same line), `FootComment` (line after, before blank line). Setting the wrong one produces comments in unexpected positions. Worse, for mapping nodes, the comment must be set on the correct sub-node -- the key node vs the value node behave differently.

**Why it happens:** The distinction between "inline" and "above" comment modes maps to `LineComment` and `HeadComment` respectively, but only if set on the right node. For a mapping pair like `image: nginx`, the key node is `image` and the value node is `nginx`. Setting `LineComment` on the value node places the comment after `nginx`. Setting it on the key node places it after `image:` but before the value. Neither may be what you want.

**Consequences:** Comments appear in wrong positions, overlap with each other, or disappear entirely when the encoder decides they conflict with existing comments.

**Warning signs:**
- Inline comments appear on the line above instead of at the end
- Comments on mapping values appear between the key and value
- Comments on sequence items appear on the wrong line

**Prevention:**
- Build a small test matrix: scalar value, mapping key, mapping value, sequence item, nested mapping. For each, test HeadComment and LineComment placement.
- For inline mode: set `LineComment` on the **value** node of a mapping pair
- For above mode: set `HeadComment` on the **key** node of a mapping pair
- For sequence items: set on the item node itself
- If the node already has a comment in that slot, decide on a merge strategy (append, skip, or replace) -- do not silently overwrite

**Phase:** Phase 1, during comment injection implementation.

**Confidence:** HIGH -- verified from go-yaml v3 Node type documentation.

---

### Pitfall 6: Multi-Document YAML Handling

**What goes wrong:** kubectl output can contain multiple YAML documents separated by `---`. When getting multiple resources (`kubectl get pods -o yaml --show-managed-fields`), Kubernetes wraps them in a `List` kind. But users might also pipe arbitrary multi-document YAML from files. The tool must handle:
1. Single documents (common case)
2. Kubernetes `List` kind (items array)
3. Multi-document streams separated by `---`
4. Empty documents between separators

go-yaml v3 `Decoder.Decode()` reads one document at a time and returns `io.EOF` when done, so multi-document reading requires a loop. But the `---` separators themselves are not preserved as nodes; they are implicit document boundaries.

**Why it happens:** Developers test with single-resource output and never test with `kubectl get pods -o yaml`. The `List` kind is a meta-wrapper that does not appear in individual resource YAML. Multi-document streams require calling `Decode` in a loop, but the initial implementation often calls it once.

**Consequences:** Only the first document gets annotated. Or the `List` kind wrapper gets annotated but not its items. Or `---` separators are lost in output.

**Warning signs:**
- Tool outputs only the first resource when given multiple
- `---` separators disappear from output
- `List` kind items have no annotations

**Prevention:**
- Use `yaml.NewDecoder(reader)` and loop calling `Decode` until `io.EOF`
- Detect `List` kind by checking `apiVersion` and `kind` fields, then recurse into `items`
- Re-emit `---` separators between documents in output
- Test with: single doc, `List` kind, multi-doc stream, empty docs, doc with only `---`

**Phase:** Phase 2 or 3 (after core single-doc pipeline works). But design the architecture to accommodate it from Phase 1.

**Confidence:** HIGH -- kubectl-neat issue #109 confirms this is a real gap. go-yaml v3 Decoder API verified.

---

### Pitfall 7: Relative Timestamp Calculation Edge Cases

**What goes wrong:** The `time` field in `ManagedFieldsEntry` is a Kubernetes `metav1.Time` (wrapper around `time.Time`). Converting to relative timestamps ("2h ago", "3d ago") has edge cases:
1. **Nil time:** Some managed field entries have no timestamp (especially older entries or certain controllers)
2. **Clock skew:** The local machine's clock may differ from the API server's clock
3. **Future timestamps:** Clock skew can produce negative durations
4. **Timezone handling:** Kubernetes stores UTC; local display may confuse users
5. **Precision:** "2d ago" vs "2d3h ago" vs "51h ago" -- what granularity is useful?

**Why it happens:** The `Time` field is optional (`*Time`), so nil checks are required. Clock skew between developer laptops and clusters is common, especially with managed Kubernetes services.

**Consequences:** Panics on nil time pointers. Confusing output like "-2h ago" or "in 2h" for entries that should show past timestamps. Overly precise timestamps that clutter the output.

**Warning signs:**
- Panic when processing resources from certain controllers (nil Time)
- Negative durations in output
- Timestamps that seem wrong compared to `kubectl describe` output

**Prevention:**
- Always nil-check the Time pointer; use "unknown" or omit when nil
- Clamp negative durations to "just now" or "0s ago"
- Use human-friendly bucketing: "5s", "2m", "3h", "5d", "2mo", "1y" -- skip sub-units
- Display UTC indicator in comment if desired, but keep the relative format primary
- Test with: nil time, time exactly now, time 0.5s ago, time 1 year ago, time in the future

**Phase:** Phase 1, as part of comment content formatting.

**Confidence:** MEDIUM -- based on Kubernetes API documentation for ManagedFieldsEntry.Time being optional. Clock skew is common knowledge but specific behavior depends on cluster setup.

---

### Pitfall 8: Color Output Breaks Piping and Non-Terminal Contexts

**What goes wrong:** Adding ANSI color codes to comment text (e.g., colored manager names) breaks when output is piped to a file, another command, or a pager that does not support ANSI. The YAML itself becomes invalid because ANSI escape sequences appear inside comments.

**Why it happens:** Developers test in interactive terminals where colors look great. They forget that kubectl plugins are frequently used in pipelines: `kubectl fields < input.yaml | kubectl apply -f -` or `kubectl fields < input.yaml > annotated.yaml`.

**Consequences:** Piped output contains literal `\033[32m` escape sequences. Files contain garbage characters. YAML parsers that read the output may choke on non-ASCII content in unexpected places. Users on Windows terminals may see garbled output.

**Warning signs:**
- Output looks garbled when redirected to a file
- `| head` or `| less` shows escape codes
- Windows users report broken output

**Prevention:**
- Default to no color. Only enable color when ALL conditions are met: stdout is a TTY (`go-isatty` package), `NO_COLOR` env var is not set, and `--color` flag is not `never`
- Support `--color=auto|always|never` flag (matching `ls`, `grep`, `git` conventions)
- Respect `NO_COLOR` environment variable (https://no-color.org/)
- Keep ANSI codes out of YAML comment content entirely -- use them only for the surrounding output formatting if needed
- Test with: `| cat`, `> file`, `| less`, `| head`, direct terminal, `NO_COLOR=1`

**Phase:** Phase 2 (output formatting), but design the comment generation to be color-agnostic from Phase 1.

**Confidence:** HIGH -- NO_COLOR standard and go-isatty package verified from official sources.

---

### Pitfall 9: YAML 1.1 vs 1.2 Boolean and Null Value Corruption

**What goes wrong:** go-yaml v3 uses mostly YAML 1.2 but with YAML 1.1 backward compatibility for values like `yes`, `no`, `on`, `off`, `y`, `n`. When the tool decodes and re-encodes YAML, values that were quoted strings in the original (e.g., `"yes"`, `"on"`) may lose their quotes and be reinterpreted as booleans. Similarly, `null` vs `~` vs empty string handling can change.

**Why it happens:** Kustomize's source code explicitly documents this: "If an input is read with `field: "on"`, and the style is changed from DoubleQuote to 0, it will change the type of the field from a string to a bool." go-yaml v3 preserves `Node.Style` during decode, but if any code path resets the style or creates new nodes, the quoting is lost.

**Consequences:** The annotated YAML output changes the *meaning* of values. A ConfigMap with `data: { enabled: "yes" }` becomes `data: { enabled: yes }` which Kubernetes interprets as boolean `true`, potentially breaking applications.

**Warning signs:**
- String values like `"yes"`, `"no"`, `"true"`, `"false"` lose their quotes in output
- `null` values appear where empty strings were expected
- Kubernetes rejects the tool's output due to type mismatches

**Prevention:**
- Never modify `Node.Style` during annotation -- only add comments, never touch Value or Style fields
- When creating synthetic nodes (if needed), explicitly set `Style: yaml.DoubleQuotedStyle` for string values that would be ambiguous unquoted
- Test with a YAML file containing every YAML 1.1 ambiguous value: `yes`, `no`, `on`, `off`, `true`, `false`, `null`, `~`, `0o777`, `0x1F`
- Verify round-trip: decode, add comments (only), encode, decode again, compare values

**Phase:** Phase 1 validation. Must be caught in round-trip testing.

**Confidence:** HIGH -- verified from kustomize/kyaml compatibility.go source code.

---

### Pitfall 10: kubectl Plugin Binary Name Conflicts and Discovery Issues

**What goes wrong:** The binary must be named `kubectl-fields` to be invoked as `kubectl fields`. Common mistakes:
1. **Name conflicts with builtins:** If `fields` ever becomes a kubectl builtin, the plugin stops working (builtins take precedence)
2. **Dash/underscore confusion:** `kubectl-fields` works as `kubectl fields`. But `kubectl-field_owner` would create `kubectl field-owner` OR `kubectl field_owner` -- confusing
3. **PATH issues:** The binary must be in `$PATH`. Users installing via `go install` get it in `$GOPATH/bin` which may not be in PATH
4. **Krew naming restrictions:** If distributing via Krew, the name must be lowercase, hyphen-separated, and not conflict with existing plugins

**Why it happens:** Developers test by running the binary directly (`./kubectl-fields`) and never test actual `kubectl fields` invocation. The naming rules are not well-documented outside the kubectl plugin page.

**Consequences:** Users install the tool but `kubectl fields` says "unknown command." Or the binary name conflicts with another plugin. Or Krew submission is rejected due to naming issues.

**Warning signs:**
- `kubectl plugin list` does not show the plugin
- `kubectl fields` says "unknown command" while `kubectl-fields` works directly
- Users report the plugin works on Linux but not macOS (PATH differences)

**Prevention:**
- Verify the name `kubectl-fields` is not a builtin or commonly used plugin name (it is not, as of this research)
- Test actual `kubectl fields` invocation in CI, not just direct binary execution
- Document PATH requirements in installation instructions
- For Krew distribution: follow naming guidelines (no `kube-` prefix, descriptive, lowercase with hyphens)
- Add a `kubectl plugin list` check to CI or release testing

**Phase:** Phase 3 (distribution/packaging), but name must be chosen in Phase 0.

**Confidence:** HIGH -- verified from Kubernetes kubectl plugin documentation and Krew naming guidelines.

---

## Minor Pitfalls

Mistakes that cause annoyance but are fixable without architectural changes.

### Pitfall 11: Long Comment Lines Wrapping Poorly

**What goes wrong:** Inline comments like `# manager: kube-controller-manager, op: Update, subresource: status, 3d ago` can be very long. When combined with already-long YAML lines, the total line length exceeds typical terminal widths (80-120 chars), causing ugly wrapping.

**Prevention:**
- Abbreviate manager names if they exceed a threshold (e.g., `kube-controller-manager` -> `kcm` with a `--short` flag)
- For inline mode, consider truncation with an option for full output
- For above mode, line length is less of an issue since the comment is on its own line
- Test with real-world manager names: `kubectl-client-side-apply`, `kube-controller-manager`, `clusterrole-aggregation-controller`

**Phase:** Phase 2 (output formatting polish).

---

### Pitfall 12: managedFields Stripped by Default in kubectl Output

**What goes wrong:** Since Kubernetes 1.21+, `kubectl get -o yaml` strips `managedFields` from output by default. Users must pass `--show-managed-fields` to include them. The tool receives YAML with no managedFields and produces output with no annotations, confusing users who think the tool is broken.

**Prevention:**
- Detect when managedFields is absent and print a helpful error message: "No managedFields found. Did you use --show-managed-fields?"
- Document the `--show-managed-fields` requirement prominently
- Consider accepting a `--from-cluster` mode that fetches the resource with managedFields included (stretch goal)

**Phase:** Phase 1 (error handling and UX).

---

### Pitfall 13: Testing YAML Output Is Fragile

**What goes wrong:** Tests that compare YAML strings byte-for-byte break with any formatting change -- different indentation, key ordering, trailing whitespace, trailing newlines. go-yaml v3 may produce slightly different output across versions or platforms.

**Prevention:**
- Use semantic YAML comparison in tests (decode both expected and actual, compare as Go structures) for correctness tests
- Use golden file tests with exact byte comparison for formatting/comment placement tests, but be prepared to update golden files
- Separate tests into: (1) correctness tests (right fields annotated) using semantic comparison, and (2) formatting tests (comments in right position) using golden files
- In golden file tests, normalize trailing whitespace and newlines
- Use `testdata/` directory with real kubectl output as fixtures

**Phase:** Phase 1 (testing infrastructure).

---

### Pitfall 14: Handling of Already-Existing Comments in Input YAML

**What goes wrong:** Users may pipe YAML that already contains comments. The tool must not destroy existing comments when adding ownership annotations. Worse, if the tool is run twice on the same input, it should not duplicate annotations.

**Prevention:**
- Preserve all existing `HeadComment`, `LineComment`, and `FootComment` content on nodes
- When adding a comment to a node that already has one, append (with separator) rather than replace
- Consider a comment prefix like `# [fields]` to identify tool-generated comments, enabling idempotent re-runs by detecting and replacing existing annotations
- Test with: YAML with existing comments, YAML with existing tool-generated comments (idempotency), YAML with comments on every line

**Phase:** Phase 2 (robustness).

---

### Pitfall 15: Anchor and Alias Nodes in YAML

**What goes wrong:** YAML supports anchors (`&anchor`) and aliases (`*anchor`) for reusing content. go-yaml v3 represents aliases as `AliasNode` with a pointer to the anchored node. Annotating an aliased node may unintentionally affect all references to that anchor, or the alias may not be resolvable during the annotation pass.

**Why it happens:** Kubernetes YAML rarely uses anchors/aliases, but users may have Helm-generated or hand-crafted YAML that does. Kustomize's source code explicitly handles "de-anchoring" as a complex operation.

**Prevention:**
- Skip annotation of `AliasNode` types -- they should inherit from their anchor
- Or de-anchor before processing (expand all aliases inline) as kustomize does
- Test with a YAML file containing anchors and aliases

**Phase:** Phase 3 (edge case hardening). Low priority since Kubernetes API objects do not use anchors.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|---|---|---|
| Phase 1: FieldsV1 parsing | `k:{}` keys with multiple fields, `v:` literal values, `.` dot marker (#2) | Use structured-merge-diff library or comprehensive JSON parsing for key content |
| Phase 1: YAML round-trip | Formatting corruption, boolean/null value changes (#1, #9) | Round-trip fidelity test suite from day one; never modify Style fields |
| Phase 1: Node matching | Off-by-one in MappingNode pairs, wrong list item matched (#3) | Dedicated tree walker with pair-aware iteration |
| Phase 1: Comment placement | HeadComment vs LineComment on key vs value node (#5) | Test matrix for every node type in both inline and above modes |
| Phase 1: Missing managedFields | Users forget `--show-managed-fields` (#12) | Detect and print helpful error message |
| Phase 2: Multi-document | Only first doc processed, `---` lost, `List` kind ignored (#6) | Decoder loop with separator preservation |
| Phase 2: Color output | ANSI codes in piped output (#8) | TTY detection, NO_COLOR support, `--color` flag |
| Phase 2: Existing comments | Overwritten user comments, non-idempotent reruns (#14) | Append strategy with tool-generated comment prefix |
| Phase 3: Distribution | PATH issues, naming conflicts (#10) | Test actual `kubectl fields` invocation; document PATH setup |
| Phase 3: Edge cases | Anchors/aliases, YAML 1.1 booleans in exotic values (#15, #9) | Skip alias nodes; comprehensive round-trip test with ambiguous values |

---

## Sources

- go-yaml v3 official documentation: https://pkg.go.dev/gopkg.in/yaml.v3 (HIGH confidence)
- go-yaml/yaml GitHub repository and issues: https://github.com/go-yaml/yaml (HIGH confidence -- archived April 2025)
- Kubernetes Server-Side Apply documentation: https://kubernetes.io/docs/reference/using-api/server-side-apply/ (HIGH confidence)
- structured-merge-diff fieldpath package: https://pkg.go.dev/sigs.k8s.io/structured-merge-diff/v4/fieldpath (HIGH confidence)
- kustomize/kyaml source code (rnode.go, compatibility.go): https://github.com/kubernetes-sigs/kustomize (HIGH confidence)
- kubectl plugin documentation: https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/ (HIGH confidence)
- Krew naming guidelines: https://krew.sigs.k8s.io/docs/developer-guide/develop/naming-guide/ (HIGH confidence)
- kubectl-neat issues (multi-document gap): https://github.com/itaysk/kubectl-neat/issues (MEDIUM confidence)
- NO_COLOR standard: https://no-color.org/ (HIGH confidence)
- go-isatty package: https://pkg.go.dev/github.com/mattn/go-isatty (HIGH confidence)
- goccy/go-yaml alternative library: https://github.com/goccy/go-yaml (MEDIUM confidence -- evaluated as alternative)
- Kubernetes ManagedFieldsEntry type: https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ManagedFieldsEntry (HIGH confidence)
