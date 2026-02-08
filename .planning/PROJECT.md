# kubectl-fields

## What This Is

A kubectl plugin written in Go that annotates Kubernetes YAML output with field ownership information. Users pipe `kubectl get ... -o yaml` output through `kubectl fields`, and each field gets a YAML comment showing which manager owns it, its subresource (if any), and when it was last updated — using human-readable relative timestamps. The `managedFields` section is stripped from output since its information is now presented inline.

## Core Value

Instantly see who or what manages every field in a Kubernetes resource, and when it last changed — without leaving the terminal or reading raw managedFields JSON.

## Requirements

### Validated (v1.0)

**Core (P0) -- All Complete:**
- [x] Parse Kubernetes YAML from stdin (single objects, List kind, multi-document)
- [x] Parse `managedFields` FieldsV1 format (`f:`, `k:`, `v:` prefix notation) and correlate to actual YAML paths
- [x] Inline mode (default): append `# manager-name (age)` comment at end of each managed line
- [x] Above mode (`--above`): place `# manager-name (age)` comment on the line above each managed field
- [x] Show subresource in annotation when present (e.g., `/status`)
- [x] Leave unmanaged fields bare (no annotation)
- [x] Strip `managedFields` section from output
- [x] Human-readable relative timestamps (e.g., "5m ago", "3h10m ago", "2d ago")
- [x] Output must be valid YAML (comments are valid YAML, structure preserved)

**Flags & Color (P1) -- All Complete:**
- [x] `--mtime absolute` flag for absolute timestamps instead of relative
- [x] `--mtime hide` flag to hide timestamps and show only manager names
- [x] Color output: each manager name gets a distinct color when stdout is a TTY (8-color palette)
- [x] `--color auto/always/never` tri-state flag
- [x] Per-manager color via round-robin palette assignment
- [x] Comment alignment across adjacent lines with outlier-aware block splitting
- [x] Graceful handling of missing managedFields (output unchanged + stderr warning with color)

**Extended (P2) -- Partial:**
- [x] `--show-operation` flag for Apply/Update display

**Quality:**
- [x] Comprehensive unit tests covering parsing, annotation, edge cases
- [x] No live cluster access — tool only processes YAML from stdin

### Backlog

- [ ] `--short-names` flag for common manager name shortening (REQ-021, P2)
- [ ] `--managers=name1,name2` filter flag (REQ-023, P2)

### Out of Scope

- File input (`-f` flag) — stdin-only keeps the interface clean
- Querying live clusters or calling the Kubernetes API
- Conflict detection between managers — each field has one owner
- GUI / web / TUI interface
- JSON output format (no comments in JSON)
- Configuration file — flags only
- Per-manager color customization — good defaults + --no-color
- Diff mode between resources
- Auto-update or version checking

## Context

- Kubernetes managed fields use `FieldsV1` format where `f:fieldName` denotes a field, `k:{json}` denotes a list item by key, and `v:value` denotes a list item by value
- Each managed field entry has a `manager` name, `operation` type (Apply/Update), optional `subresource`, `time` timestamp, and the `fieldsV1` ownership tree
- The tool must handle the recursive tree structure of `fieldsV1` and map it back to the corresponding YAML paths in the resource
- `kubectl get ... -o yaml` returns a `List` kind wrapping multiple resources — the tool must handle this
- Multi-document YAML (`---` separated) must also be supported
- Test data already exists in `testdata/` with input YAML and expected output for both inline and above modes

## Constraints

- **Language**: Go — standard for kubectl plugins and Kubernetes ecosystem tooling
- **Distribution**: Must work as a kubectl plugin (binary named `kubectl-fields` in PATH)
- **Testing**: No live cluster access in tests. Use fixture YAML files for all test cases.
- **Dependencies**: Keep minimal. Standard library + a YAML library (go.yaml.in/yaml/v3 for node-level access with comments)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Stdin-only input | Simplicity; fits kubectl pipe workflow naturally | Validated in v1.0 |
| Go as language | Kubectl plugin convention, single binary distribution, ecosystem alignment | Validated in v1.0 |
| Strip managedFields from output | Info is now in annotations; raw block is noise | Validated in v1.0 |
| Comments for annotations | Preserves valid YAML; parsers ignore comments | Validated in v1.0 |
| One manager per field assumption | Kubernetes managed fields guarantee single ownership per field | Validated in v1.0 |
| go.yaml.in/yaml/v3 (official fork) | Not archived gopkg.in/yaml.v3; SetIndent(2)+CompactSeqIndent() for round-trip fidelity | Validated in v1.0 |
| Parallel descent algorithm | Walk FieldsV1 + YAML trees simultaneously; avoids path-string intermediary | Validated in v1.0 |
| Two-pass collect-then-inject | Targets map keyed by ValueNode pointer; natural last-writer-wins | Validated in v1.0 |
| Round-robin color assignment | FNV-1a hash clustered common k8s manager names; round-robin gives distinct colors | Fixed in UAT, validated |
| Outlier-aware alignment | 40-char threshold prevents long lines from pushing all adjacent comments right | Fixed in UAT, validated |

---
*Last updated: 2026-02-08 after v1.0 milestone completion*
