# Project Research Summary

**Project:** kubectl-fields
**Domain:** kubectl plugin / Kubernetes YAML annotation tool
**Researched:** 2026-02-07
**Confidence:** HIGH

## Executive Summary

kubectl-fields is a kubectl plugin that annotates Kubernetes YAML output with field ownership information from managedFields. The tool processes YAML from stdin, parses the FieldsV1 format from managedFields entries, and injects comments showing which manager owns each field, when it was last applied, and which subresource (if any) the field belongs to. This is a rewrite/v2 of an existing tool (ahmetb/kubectl-fields) with critical improvements for multi-document YAML, List kind support, and robust comment handling.

The recommended approach is to build on Go 1.23+ using `go.yaml.in/yaml/v3` (the official YAML org fork, not the archived `gopkg.in/yaml.v3`), cobra for CLI structure, and fatih/color for per-manager colorization. The architecture centers on a parallel descent algorithm that walks both the FieldsV1 ownership tree and the YAML Node tree simultaneously, matching `f:` (field), `k:` (associative list key), and `v:` (set value) prefixes to YAML nodes, then injecting HeadComment or LineComment based on user preference (--above flag vs inline default).

The key technical risk is **YAML round-trip fidelity**: go-yaml v3 is known to alter formatting during decode/encode cycles, which would destroy trust in a tool whose purpose is to annotate (not transform) YAML. This must be validated with round-trip tests from day one. Secondary risks include correctly parsing the complex FieldsV1 format (especially `k:` keys with multiple fields and JSON-encoded content), and matching those paths to the correct YAML Node positions (MappingNode Content is a flat alternating key/value slice, not a map). Mitigation: use the parallel descent pattern from ARCHITECTURE.md, comprehensive test fixtures with real kubectl output, and never modify yaml.Node.Style fields.

## Key Findings

### Recommended Stack

The Kubernetes ecosystem is Go. kubectl plugins are overwhelmingly Go, users expect Go, and Krew distribution assumes cross-compiled Go binaries. Go 1.23+ is the target minimum (matching broader kubectl plugin ecosystem compatibility), develop on 1.25.

**Core technologies:**
- **Go 1.23+**: Language — ecosystem standard, access to YAML libraries, cross-compilation via GoReleaser
- **go.yaml.in/yaml/v3 (v3.0.4)**: YAML parsing and comment injection — critical feature: `yaml.Node` with HeadComment/LineComment/FootComment fields. This is the official YAML org fork maintained after gopkg.in/yaml.v3 was archived April 2025. API-identical to the original, so all existing documentation applies.
- **github.com/spf13/cobra (v1.10.2)**: CLI framework — de facto standard in kubectl ecosystem, provides flag parsing via pflag, help generation, shell completions for free
- **github.com/fatih/color (v1.18.0)**: Colorized output — most widely used Go color library, automatically disables on non-TTY, respects NO_COLOR env var, simple API for per-manager color assignment
- **github.com/stretchr/testify (v1.11.1)**: Test assertions — reduce boilerplate, use assert/require only (not suite/mock)
- **gotest.tools/v3/golden (v3.5.2)**: Golden file testing — ideal for YAML-in/YAML-out tools, same library Docker uses
- **GoReleaser (v2.13.3)**: Build and distribution — cross-compilation for all platforms, GitHub releases, Krew manifest generation, Homebrew tap formulas

**Critical version notes:**
- Do NOT use `gopkg.in/yaml.v3` — archived and unmaintained as of April 2025
- Do NOT use `goccy/go-yaml` — different API, unnecessary complexity for this use case
- Do NOT import `k8s.io/apimachinery` — heavyweight dependency for simple managedFields parsing

### Expected Features

Users expect the tool to process stdin YAML (`kubectl get -o yaml | kubectl fields`), parse the FieldsV1 format (f:/k:/v: prefixes), inject comments showing manager name, subresource, and timestamp (relative by default, absolute via --absolute-time flag), and output valid YAML with managedFields stripped and unmanaged fields left bare. Color output on TTY is expected (modern CLI standard), with --no-color flag for pipes.

**Must have (table stakes):**
- T1-T10: Core pipeline (stdin parse, FieldsV1 parse, inline/above comment modes, manager/subresource/timestamp display, strip managedFields, valid YAML output)
- T11-T12: Color output on TTY with --no-color flag
- T13-T14: Unmanaged fields bare, --no-time flag

**Should have (competitive):**
- D1-D2: Multi-document YAML support (---) and List kind unwrapping — explicitly required in PROJECT.md, existing tool fails this
- D3: Comment alignment — inline comments form readable columns, not ragged edges
- D4-D5-D8: Improved color handling (deterministic per-manager color via hash, --color auto/always/never tri-state, palette variety beyond single red)

**Defer (v2+):**
- D7: Manager name shortening/aliases (nice-to-have)
- D9: --managers filter (add when requested)
- D10: Operation type display (niche SSA debugging use case)

**Anti-features (never build):**
- A1: Live cluster querying — stay stdin-only, Unix philosophy
- A2: Conflict detection — that's server-side SSA, don't reimplement client-side
- A3: YAML rewriting/mutation — read-only annotation only
- A4: JSON output — JSON doesn't support comments, wrong mental model
- A5: Interactive TUI — different tool entirely
- A6-A7: Config file or per-manager color customization — too much complexity for 4-5 flags
- A9: File input (-f flag) — stdin covers all via shell redirection (`< file.yaml`)

### Architecture Approach

The tool is a stdin-to-stdout pipeline with 7 components: Input Reader (read all stdin bytes), YAML Parser (yaml.v3 Node trees, handle multi-doc and List kinds), ManagedFields Extractor (walk Node tree to find managedFields entries), ManagedFields Stripper (remove managedFields from metadata), Path Mapper / Tree Walker (walk FieldsV1 trees and YAML tree in parallel, build ownership map), Comment Injector (set HeadComment or LineComment on matched nodes), and Output Renderer (encode annotated nodes, handle color for TTY).

**Major components:**
1. **YAML Parser** — produces `[]*yaml.Node` documents from stdin, handles multi-doc (---) and List kind unwrapping
2. **ManagedFields Extractor + Stripper** — walks metadata.managedFields to extract entries (manager, operation, subresource, time, fieldsV1 tree), then removes managedFields from output tree
3. **Parallel Descent Walker** — walks FieldsV1 ownership tree and YAML Node tree simultaneously, resolving `f:fieldName` to mapping keys, `k:{"key":"value"}` to list items by key match, `v:"literal"` to list items by value match
4. **Comment Injector** — at each matched node, formats annotation (`# manager (/subresource) (age)`), sets LineComment (inline mode) or HeadComment (above mode)
5. **Output Renderer + Color Manager** — encodes annotated Node trees with yaml.v3 Encoder, post-processes for ANSI color codes on TTY

**Key patterns:**
- Use yaml.v3 Node API exclusively (never Marshal/Unmarshal to structs)
- Walk FieldsV1 as YAML Nodes (not JSON strings) — the fieldsV1 value in kubectl output is already a YAML mapping
- Two-pass separation: path mapping builds OwnershipMap, comment injection does single YAML walk with lookups
- Parallel descent (recommended): walk FieldsV1 and YAML trees together, matching nodes directly without path string intermediary

**Build order:**
1. Phase 1 Foundation: Time Formatter, FieldsV1 Prefix Parser (pure functions, easily testable)
2. Phase 2 Core Pipeline: YAML Parser, ManagedFields Extractor/Stripper
3. Phase 3 Annotation Engine: Parallel Descent Walker + Comment Injector (hardest component, build last)
4. Phase 4 Output: Output Renderer, Color Manager
5. Phase 5 CLI: Main + cobra setup, wire pipeline

### Critical Pitfalls

1. **YAML round-trip formatting corruption (Pitfall #1)** — go-yaml v3 docs: "The content when re-encoded will not have its original textual representation preserved." Decode then encode produces different output (indentation, quoting, multiline strings). For an annotation tool, any formatting change destroys trust. **Prevention:** Round-trip fidelity tests from day one, never modify Node.Style fields, test with real kubectl output (Deployments, ConfigMaps, CRDs). If go-yaml v3 cannot round-trip adequately, architecture must change (consider raw text manipulation with Node positions as guide, or switch to goccy/go-yaml).

2. **FieldsV1 parsing complexity (Pitfall #2)** — The `f:`, `k:`, `v:` prefix format has edge cases: `k:{"containerPort":8080,"protocol":"TCP"}` (multi-field JSON keys with commas), `k:{"key":"value with : colons"}` (JSON strings contain colons), `v:"literal"` for set-type lists, `.` (dot) marker for self-ownership. Naive string splitting breaks. **Prevention:** Use proper JSON parsing for `k:` key content, or use `sigs.k8s.io/structured-merge-diff/v4/fieldpath` library. Test every PathElement variant: f:, k: single-field, k: multi-field, v:, dot.

3. **Node position matching errors (Pitfall #3)** — MappingNode.Content is a flat slice alternating keys/values, not a map. Off-by-one errors annotate wrong lines. For k: list matching, must scan all items to find the one whose key fields match. **Prevention:** Dedicated tree walker with explicit pair iteration (step by 2), k:/v: matching functions that decode and compare list item fields, test with 10+ key mappings and similar-but-not-identical list items.

4. **go-yaml v3 archived and unmaintained (Pitfall #4)** — gopkg.in/yaml.v3 (go-yaml/yaml) was archived April 2025. Open bugs (comment handling, encoding edge cases, multiline strings) will never be fixed. **Prevention:** Use `go.yaml.in/yaml/v3` (official YAML org fork with identical API), document all workarounds with upstream issue links, consider goccy/go-yaml as alternative during Phase 1 evaluation, build abstraction layer around yaml.Node manipulation for potential library swap.

5. **Comment placement on wrong node part (Pitfall #5)** — go-yaml v3 has HeadComment (line before), LineComment (end of same line), FootComment (line after). For mapping pair `image: nginx`, key node is `image`, value node is `nginx`. Setting LineComment on value node places comment after `nginx`. Setting on key node places between key and value. **Prevention:** Test matrix for every node type (scalar, mapping key, mapping value, sequence item) in both inline and above modes. Inline mode: set LineComment on value node. Above mode: set HeadComment on key node.

## Implications for Roadmap

Based on research, suggested 4-phase structure:

### Phase 1: Foundation & Core Pipeline
**Rationale:** Establish round-trip fidelity and FieldsV1 parsing correctness before building annotation logic. These are the highest-risk components (Pitfalls #1, #2, #4). Test with real kubectl output from day one.

**Delivers:**
- YAML stdin parsing with multi-doc and List kind support
- ManagedFields extraction and stripping
- FieldsV1 prefix parser (f:, k:, v:, dot)
- Time formatter (relative/absolute)
- Round-trip fidelity validation (no formatting changes except added comments)

**Addresses:** T1, T2, T7, T8, T9, T10 from FEATURES.md

**Avoids:** Pitfall #1 (round-trip corruption), Pitfall #2 (FieldsV1 parsing), Pitfall #4 (library choice validation), Pitfall #9 (YAML 1.1 vs 1.2 boolean corruption)

**Research flag:** Standard patterns (stdin YAML processing is well-documented). Skip research-phase.

### Phase 2: Annotation Engine
**Rationale:** The hardest technical component. Build parallel descent algorithm after foundation is solid. k: and v: list matching is complex; test with real Deployment YAML containing containers, ports, env, volumeMounts.

**Delivers:**
- Parallel descent walker (FieldsV1 tree + YAML tree simultaneously)
- k:/v: list item matching (associative lists by key, sets by value)
- Comment injection (inline and above modes)
- Comment format with manager, subresource, timestamp

**Addresses:** T3, T4, T5, T6, T13 from FEATURES.md

**Implements:** Parallel Descent Walker + Comment Injector from ARCHITECTURE.md

**Avoids:** Pitfall #3 (node position matching), Pitfall #5 (comment placement on wrong node part), Pitfall #7 (timestamp edge cases)

**Research flag:** Needs deep dive during phase planning. Parallel descent with k:/v: matching is novel algorithm specific to this tool. Plan to review structured-merge-diff source code for PathElement matching logic.

### Phase 3: Output & Polish
**Rationale:** After annotation logic works, add color, alignment, and UX polish. Color is lower risk (well-documented libraries). Test TTY detection, NO_COLOR env var, --color flag.

**Delivers:**
- Output renderer with yaml.v3 Encoder
- Color Manager (per-manager hash-based color assignment)
- TTY detection, NO_COLOR support, --color auto/always/never flag
- Comment alignment (inline comments form columns)
- --no-time flag

**Addresses:** T11, T12, T14, D3, D4, D5, D8 from FEATURES.md

**Uses:** fatih/color (v1.18.0), go-isatty (transitive) from STACK.md

**Avoids:** Pitfall #8 (color breaks piping), Pitfall #11 (long comment lines wrapping)

**Research flag:** Standard patterns (TTY detection and ANSI color are well-documented). Skip research-phase.

### Phase 4: CLI Integration & Distribution
**Rationale:** Wire everything together with cobra, add flags, error handling, distribution setup. GoReleaser automates cross-compilation and Krew manifest generation.

**Delivers:**
- Main + cobra command setup
- Flags: --above, --absolute-time, --no-time, --no-color, --color auto/always/never
- Error handling (detect missing managedFields, print helpful message)
- GoReleaser config for GitHub releases and Krew
- Installation docs (PATH requirements, --show-managed-fields requirement)

**Addresses:** CLI wrapper around all table stakes features, distribution channels (GitHub Releases, Krew, Homebrew)

**Uses:** cobra (v1.10.2), pflag (v1.0.9+), GoReleaser (v2.13.3) from STACK.md

**Avoids:** Pitfall #10 (kubectl plugin naming and discovery), Pitfall #12 (missing managedFields user confusion)

**Research flag:** Standard patterns (cobra CLI and GoReleaser are well-documented). Skip research-phase.

### Phase Ordering Rationale

- **Phase 1 first** because round-trip fidelity (Pitfall #1) is an architecture risk. If go-yaml v3 cannot preserve formatting, the entire approach must change (raw text manipulation or library swap). Validate this before building annotation logic.
- **Phase 2 after 1** because parallel descent algorithm needs parsed YAML nodes and extracted managedFields. This is the hardest component algorithmically (k:/v: matching), so build it after surrounding infrastructure is solid and testable.
- **Phase 3 after 2** because color and alignment are polish on top of working annotations. Color handling is lower risk (well-documented libraries), so defer to reduce Phase 2 complexity.
- **Phase 4 last** because CLI and distribution are wiring. cobra and GoReleaser are well-documented; this phase is straightforward once core logic works.

**Dependency chain:** Phase 1 (foundation) → Phase 2 (annotation engine using foundation) → Phase 3 (output using annotated nodes) → Phase 4 (CLI using output pipeline)

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2:** Parallel descent algorithm with k:/v: list matching is novel to this tool. Needs careful study of structured-merge-diff PathElement matching logic and go-yaml v3 Node tree traversal. Plan for 1-2 day spike on list matching algorithm with test fixtures.

Phases with standard patterns (skip research-phase):
- **Phase 1:** stdin YAML processing, multi-doc handling, and go-yaml v3 Decoder usage are well-documented
- **Phase 3:** TTY detection, ANSI color libraries, NO_COLOR standard all have clear documentation
- **Phase 4:** cobra CLI setup and GoReleaser configuration are extensively documented with kubectl plugin examples

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All libraries verified from official docs and pkg.go.dev. Versions checked against latest stable releases. go.yaml.in/yaml/v3 fork confirmed as official YAML org continuation. |
| Features | HIGH | Feature landscape verified against ahmetb/kubectl-fields source code (direct predecessor tool), kubectl-mutated, yq, stern, kubecolor, and kubectl plugin documentation. Table stakes vs differentiators validated against existing tool capabilities. |
| Architecture | HIGH | Parallel descent pattern verified against Kubernetes server-side apply docs and structured-merge-diff source code. yaml.v3 Node API usage patterns confirmed from official pkg.go.dev docs and kustomize/kyaml source (which uses same approach). |
| Pitfalls | HIGH | All critical pitfalls verified against primary sources: go-yaml v3 docs (round-trip warning), GitHub archive status (April 2025), structured-merge-diff fieldpath API (PathElement serialization), kustomize source code (YAML 1.1 compatibility notes). |

**Overall confidence:** HIGH

### Gaps to Address

- **go-yaml v3 round-trip fidelity:** Research confirms this is a known issue ("effort is made to render data pleasantly" in official docs), but the severity for this specific tool is unknown until tested with real kubectl output. **Mitigation:** Build round-trip test suite in Phase 1 day 1. If fidelity is insufficient, evaluate goccy/go-yaml as alternative or raw text manipulation approach.

- **k: list matching performance:** For resources with large arrays (100+ containers in a Pod, though uncommon), scanning all items to find k: matches could be slow. Research did not find performance benchmarks. **Mitigation:** Assume typical resources have <10 list items per array. If performance issues surface, build index map during YAML parse. Not a Phase 1 concern.

- **Anchor/alias handling:** YAML anchors and aliases are edge cases (Kubernetes API objects don't use them), but user-crafted or Helm-generated YAML might. Research shows kustomize has complex "de-anchoring" logic. **Mitigation:** Defer to Phase 3+ edge case hardening. Skip annotation of AliasNode types initially, document limitation, add proper support if users report issues.

## Sources

### Primary (HIGH confidence)
- go.yaml.in/yaml/v3: https://pkg.go.dev/go.yaml.in/yaml/v3 (v3.0.4, Jun 29, 2025) — Node API, comment fields
- go-yaml/yaml GitHub: https://github.com/go-yaml/yaml — archive status verified April 2025
- Kubernetes Server-Side Apply: https://kubernetes.io/docs/reference/using-api/server-side-apply/ — ManagedFieldsEntry structure, FieldsV1 format
- structured-merge-diff: https://pkg.go.dev/sigs.k8s.io/structured-merge-diff/v4/fieldpath — PathElement types, serialization
- kubectl plugin docs: https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/ — naming conventions, discovery
- Krew developer guide: https://krew.sigs.k8s.io/docs/developer-guide/ — distribution, manifest format
- ahmetb/kubectl-fields source: GitHub API direct read — existing tool implementation (verified annotate.go, managedfields.go, printer.go, aligningprinter.go)
- cobra: https://pkg.go.dev/github.com/spf13/cobra (v1.10.2) — CLI framework API
- fatih/color: https://pkg.go.dev/github.com/fatih/color (v1.18.0) — color library API, NO_COLOR support
- testify: https://pkg.go.dev/github.com/stretchr/testify (v1.11.1) — assertion library
- gotest.tools golden: https://pkg.go.dev/gotest.tools/v3/golden (v3.5.2) — golden file testing
- goreleaser: https://github.com/goreleaser/goreleaser (v2.13.3) — release automation
- NO_COLOR standard: https://no-color.org/ — color disabling convention

### Secondary (MEDIUM confidence)
- kustomize/kyaml source (rnode.go, compatibility.go) — YAML manipulation patterns, YAML 1.1 compatibility notes
- kubectl-neat issues — multi-document handling gaps (issue #109)
- kubectl-mutated source — alternative approach using goccy/go-yaml with AST-level access
- kubecolor, stern, yq READMEs — color flag patterns, TTY handling conventions

### Tertiary (LOW confidence)
- None — all findings verified against primary sources or direct code inspection

---
*Research completed: 2026-02-07*
*Ready for roadmap: yes*
