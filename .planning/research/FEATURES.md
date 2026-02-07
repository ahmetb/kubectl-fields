# Feature Landscape

**Domain:** kubectl plugin for managed fields visualization / YAML annotation
**Researched:** 2026-02-07
**Overall confidence:** HIGH (verified against ahmetb/kubectl-fields source code, kubectl-mutated source, kubecolor, stern, yq, kubectl-neat, and Kubernetes official docs)

## Existing Tools

Before categorizing features, here is the landscape of tools this project either competes with or draws patterns from:

| Tool | Purpose | Relevance |
|------|---------|-----------|
| **ahmetb/kubectl-fields** (5 stars) | Inline managed-field annotation on YAML | Direct predecessor / near-identical scope. Written by same author. |
| **kubectl-mutated** | Detect manually-mutated fields for GitOps drift | Adjacent scope: highlights *which* fields are manual, not *who* owns each field. |
| **kubectl-neat** (2k stars, unmaintained) | Strip cluster-generated noise from YAML | Complementary: strips managedFields entirely rather than visualizing them. |
| **kubecolor** | Colorize all kubectl output | Pattern reference for color/TTY handling. |
| **stern** | Multi-pod log tailing with per-pod colors | Pattern reference for per-entity color assignment and `--color auto/always/never`. |
| **yq** | General YAML processor | Pattern reference for stdin handling, color flags (`-C`/`-M`), multi-doc support. |
| **kubectl-tree** | Owner-reference tree visualization | Pattern reference for relationship visualization UX. |

**Key finding:** There is essentially one direct competitor (ahmetb/kubectl-fields), which already implements the core concept with inline and above comment placement, relative/absolute timestamps, color output, and comment alignment. The project described in PROJECT.md is a rewrite/v2 of this same tool. The feature landscape below is therefore informed by what the existing tool does, what it lacks, and patterns from adjacent tools.

---

## Table Stakes

Features users expect. Missing any of these and the tool feels incomplete or broken compared to the existing implementation.

| # | Feature | Why Expected | Complexity | Dependencies | Notes |
|---|---------|--------------|------------|--------------|-------|
| T1 | **Stdin YAML parsing** | Core input mechanism. `kubectl get -o yaml \| kubectl fields` is the canonical usage. | Low | None | Must handle both piped and redirected input. Detect missing stdin gracefully. |
| T2 | **FieldsV1 parsing** (`f:`, `k:`, `v:` prefixes) | The entire value proposition requires parsing Kubernetes' opaque managed fields format. | High | None | Use `sigs.k8s.io/structured-merge-diff` for robust parsing. The trie structure of `f:` (struct fields), `k:` (list keys), and `v:` (list values) is non-trivial. |
| T3 | **Inline comment placement** (default) | The default and most common mode. Appends `# manager (age)` at end of line. | Med | T1, T2 | Must handle YAML values that already contain `#` characters in strings. |
| T4 | **Above comment placement** (`--above`) | Alternative for long lines or readability preference. Places comment on the line before. | Med | T1, T2 | Must preserve correct indentation. Existing tool supports this via `-p above`. |
| T5 | **Manager name display** | The primary information users want: "who manages this field?" | Low | T2 | Display the `manager` string from each ManagedFieldsEntry. |
| T6 | **Subresource display** | Fields managed via `/status` subresource need to be distinguishable from main resource fields. | Low | T2 | Format: `manager (/status)`. Critical for understanding controller behavior. |
| T7 | **Relative timestamps** (default) | Human-readable "5m ago", "2d ago" conveys recency at a glance. | Low | T2 | Use a duration formatting library. Existing tool uses `github.com/hako/durafmt`. |
| T8 | **Absolute timestamps** (`--absolute-time`) | Some users need exact dates for auditing or correlation with logs. | Low | T7 | Flag: `--absolute-time` or `-t absolute`. |
| T9 | **Strip managedFields from output** | The raw managedFields block is replaced by inline annotations -- keeping it is pure noise. | Low | T2 | Remove the entire `.metadata.managedFields` array from output. |
| T10 | **Valid YAML output** | Output must be parseable by downstream YAML tools. Comments are valid YAML. | Low | T3, T4, T9 | Verify with `yq` or a YAML parser that output round-trips. |
| T11 | **Color output on TTY** | Visual distinction between managers. Expected in modern CLI tools. | Med | T5 | Auto-detect TTY. Use ANSI colors. Each unique manager name gets a consistent color. |
| T12 | **`--no-color` flag** | Disable color for piping to files, CI, or accessibility. | Low | T11 | Standard kubectl ecosystem pattern. |
| T13 | **Unmanaged fields left bare** | Fields not tracked in managedFields should have no annotation -- avoids noise. | Low | T2, T3 | Already the natural behavior if you only annotate matched paths. |
| T14 | **`--no-time` flag** | Show only manager names without timestamps for cleaner output. | Low | T7 | Reduces visual clutter when age is not relevant. |

---

## Differentiators

Features that set this tool apart from the existing ahmetb/kubectl-fields implementation and adjacent tools. Not necessarily expected, but valued.

| # | Feature | Value Proposition | Complexity | Dependencies | Notes |
|---|---------|-------------------|------------|--------------|-------|
| D1 | **Multi-document YAML support** (`---` separated) | `kubectl get` with multiple resources returns multi-doc YAML. Existing tool rejects this with "error validating object." | Med | T1, T2 | Must iterate over YAML documents. PROJECT.md explicitly requires this. |
| D2 | **List kind support** | `kubectl get deploy -o yaml` wraps results in a `kind: List`. Existing tool does not handle this. | Med | D1 | Unwrap `.items[]`, process each, re-emit. PROJECT.md explicitly requires this. |
| D3 | **Comment alignment** | Align inline comments across adjacent lines so they form a readable column rather than ragged edges. | Med | T3 | Existing tool has this via `aligningPrinter` with a 60-char tolerance. Match or improve. |
| D4 | **Per-manager color assignment** (deterministic) | Same manager always gets the same color, even across different resources or invocations. Hash-based color assignment. | Low | T11 | Stern uses configurable `--pod-colors`. A simpler hash-to-palette approach works here. |
| D5 | **`--color auto/always/never`** (tri-state) | More flexible than just `--no-color`. Allows forcing color in pipes (for `less -R`). Industry standard (stern, git, ls). | Low | T11 | Existing tool only has implicit TTY detection + red hardcoded. No force-on. |
| D6 | **Graceful handling of missing managedFields** | If input has no managedFields (e.g., `--show-managed-fields` was forgotten), output the YAML unchanged with a stderr warning rather than erroring. | Low | T2 | Existing tool handles this case but worth being explicit. |
| D7 | **Manager name shortening** | Long manager names like `kubectl-client-side-apply` could optionally be shortened to `kubectl-csa` or a configurable alias map. | Med | T5 | Not in existing tool. Reduces horizontal noise. Could be a flag or config. |
| D8 | **Color palette variety** | More than one color. Existing tool hardcodes red for all annotations. Each manager should get a distinct color from a palette of 8-16 ANSI colors. | Low | T11 | Major visual improvement over existing tool's single-color approach. |
| D9 | **`--managers` filter** | Show annotations only for specific managers (e.g., `--managers=kubectl,helm`). Useful for focused investigation. | Low | T5 | Not in existing tool. Post-MVP candidate. |
| D10 | **Operation type display** | Optionally show `Apply` vs `Update` operation type alongside manager name. Helps understand SSA vs CSA workflows. | Low | T2, T5 | Format: `manager [Apply] (age)`. Useful for SSA migration debugging. |

---

## Anti-Features

Features to explicitly NOT build. Common mistakes in this domain or scope creep that would hurt the tool.

| # | Anti-Feature | Why Avoid | What to Do Instead |
|---|--------------|-----------|-------------------|
| A1 | **Live cluster querying** | The tool should be a pure stdin filter. Adding `--namespace`, `--context`, or resource fetch logic turns it into a kubectl wrapper with auth, kubeconfig, and version compatibility issues. | Stay stdin-only. Let users compose: `kubectl get ... \| kubectl fields`. Unix philosophy. |
| A2 | **Conflict detection / resolution** | Kubernetes SSA conflict detection is a server-side feature with complex semantics. Reimplementing it client-side is wrong and misleading. | Show ownership facts only. Let users draw their own conclusions. |
| A3 | **YAML rewriting / mutation** | The tool should not modify YAML values, only add comments. Adding "fix" or "apply" capabilities crosses into dangerous territory. | Read-only annotation only. Output is informational. |
| A4 | **JSON output format** | JSON does not support comments. Trying to embed ownership info in JSON requires inventing a schema, breaking the "annotated original" mental model. | YAML-only output. Users who need JSON can process the original. |
| A5 | **Interactive / TUI mode** | A terminal UI for browsing fields is a different tool entirely. It adds complexity (cursor movement, state, screen size) without clear benefit over scrollable annotated YAML. | Pipe-friendly CLI output. Users can use `less -R` for scrolling. |
| A6 | **Configuration file** | A config file (`.kubectl-fields.yaml`) adds discovery/precedence complexity for a tool with 4-5 flags. Not worth it. | Flags only. Simple, predictable, composable. |
| A7 | **Manager name color customization** | Per-manager color configuration is too niche for the complexity it adds (config file needed, validation, error handling). | Use a good default palette. Accept `--no-color` for users who dislike the defaults. |
| A8 | **Diff mode between two resources** | Comparing managed fields between two versions of a resource is useful but is a separate tool (`kubectl diff` + managed fields). | Single-resource annotation only. Diff is a different workflow. |
| A9 | **File input (`-f` flag)** | Adds path handling, glob support, error messages for missing files. Stdin covers all cases via shell redirection (`< file.yaml`). | Stdin-only. Document `< file.yaml` as the file-reading pattern. |
| A10 | **Plugin auto-update or version checking** | Network calls from a CLI filter tool are unexpected and slow. | Rely on krew for updates. Print version with `--version`. |

---

## Feature Dependencies

```
T1 (stdin parsing) ──> T2 (FieldsV1 parsing) ──> T5 (manager name)
                                                  T6 (subresource)
                                                  T7 (relative time) ──> T8 (absolute time)
                                                                         T14 (--no-time)

T2 ──> T3 (inline comments) ──> D3 (comment alignment)
       T4 (above comments)
       T9 (strip managedFields)
       T13 (unmanaged fields bare)

T5 ──> T11 (color on TTY) ──> T12 (--no-color)
                               D5 (--color tri-state)
                               D8 (color palette)
                               D4 (deterministic per-manager color)

T1 ──> D1 (multi-doc YAML) ──> D2 (List kind support)

T5 ──> D7 (manager name shortening)
       D9 (--managers filter)
       D10 (operation type display)

T10 (valid YAML output) depends on T3, T4, T9
```

**Critical path:** T1 -> T2 -> T3/T4 -> T5/T6/T7 -> T9 -> T10 (this is the minimum viable pipeline)

**Color is parallel:** T11 can be developed independently once T5 exists.

**Multi-doc is additive:** D1/D2 extend the input handling without changing the annotation pipeline.

---

## MVP Recommendation

For MVP, prioritize all table stakes features (T1-T14). They represent the baseline that makes the tool usable and competitive with the existing ahmetb/kubectl-fields.

**MVP (Phase 1):**
1. T1-T10: Core parsing, annotation, and output pipeline
2. T11-T12: Color output (users will expect this from a modern tool)
3. T13-T14: Polish (bare unmanaged fields, no-time flag)

**Phase 2 (Differentiators):**
1. D1, D2: Multi-document and List kind support (explicitly required in PROJECT.md)
2. D3: Comment alignment (significant readability improvement)
3. D4, D5, D8: Color improvements (palette variety, tri-state flag, deterministic assignment)

**Defer to post-Phase 2:**
- D7 (manager shortening): Nice-to-have, low priority
- D9 (manager filter): Can add when users request it
- D10 (operation type): Niche use case for SSA debugging

---

## Sources

- **ahmetb/kubectl-fields source code** (HIGH confidence): Verified by reading main.go, annotate.go, managedfields.go, printer.go, aligningprinter.go, and testdata output files directly from GitHub via `gh api`. This is the primary reference implementation.
- **xdavidwu/kubectl-mutated source code** (HIGH confidence): Verified by reading highlighted_yaml.go, coloring.go, and README. Uses `goccy/go-yaml` AST-based approach with ANSI bold+italic for highlighting.
- **Kubernetes official docs** (HIGH confidence): Server-Side Apply field management docs at kubernetes.io. ManagedFieldsEntry structure from ObjectMeta API reference. Fields: manager, operation, apiVersion, time, fieldsType, fieldsV1, subresource.
- **structured-merge-diff/v4/fieldpath** (HIGH confidence): Go package docs at pkg.go.dev. Key types: PathElement (FieldName, Key, Value, Index), Path, Set. Conversion via Set.FromJSON().
- **kubecolor** (HIGH confidence): README on GitHub. TTY auto-detection, `--force-colors`, `--plain` flags, `--light-background` preset.
- **stern** (HIGH confidence): README on GitHub. `--color auto/always/never` tri-state, `--pod-colors` SGR sequences, configurable per-entity color assignment.
- **yq** (HIGH confidence): README on GitHub. `-C`/`-M` color flags, stdin auto-detection, multi-document support, comment manipulation.
- **kubectl-neat** (MEDIUM confidence): README on GitHub. Unmaintained. Strips managedFields among other noise. `-f`, `-o` flags, `get` subcommand wrapper.
- **krew developer guide** (MEDIUM confidence): Official Krew docs. Plugin naming, manifest format, submission process.
