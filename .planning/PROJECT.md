# kubectl-fields

## What This Is

A kubectl plugin written in Go that annotates Kubernetes YAML output with field ownership information. Users pipe `kubectl get ... -o yaml` output through `kubectl fields`, and each field gets a YAML comment showing which manager owns it, its subresource (if any), and when it was last updated — using human-readable relative timestamps. The `managedFields` section is stripped from output since its information is now presented inline.

## Core Value

Instantly see who or what manages every field in a Kubernetes resource, and when it last changed — without leaving the terminal or reading raw managedFields JSON.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Parse Kubernetes YAML from stdin (single objects and List/multi-document YAML)
- [ ] Parse `managedFields` FieldsV1 format (`f:`, `k:`, `v:` prefix notation) and correlate to actual YAML paths
- [ ] Inline mode (default): append `# manager-name (age)` comment at end of each managed line
- [ ] Above mode (`--above`): place `# manager-name (age)` comment on the line above each managed field
- [ ] Show subresource in annotation when present (e.g., `(/status)`)
- [ ] Leave unmanaged fields bare (no annotation)
- [ ] Strip `managedFields` section from output
- [ ] Human-readable relative timestamps (e.g., "5m ago", "3h10m ago", "2d ago")
- [ ] `--absolute-time` flag for absolute timestamps instead of relative
- [ ] `--no-time` flag to hide timestamps and show only manager names
- [ ] Color output: each manager name gets a distinct color when stdout is a TTY
- [ ] `--no-color` flag to force disable color output
- [ ] Output must be valid YAML (comments are valid YAML, structure preserved)
- [ ] Comprehensive unit tests covering parsing, annotation, edge cases
- [ ] No live cluster access — tool only processes YAML from stdin

### Out of Scope

- File input (`-f` flag) — stdin-only keeps the interface clean
- Querying live clusters or calling the Kubernetes API
- Conflict detection between managers — each field has one owner
- GUI or web interface
- Filtering by manager name or age — can be added later if needed

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
- **Dependencies**: Keep minimal. Standard library + a YAML library (likely `gopkg.in/yaml.v3` for node-level access with comments)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Stdin-only input | Simplicity; fits kubectl pipe workflow naturally | — Pending |
| Go as language | Kubectl plugin convention, single binary distribution, ecosystem alignment | — Pending |
| Strip managedFields from output | Info is now in annotations; raw block is noise | — Pending |
| Comments for annotations | Preserves valid YAML; parsers ignore comments | — Pending |
| One manager per field assumption | Kubernetes managed fields guarantee single ownership per field | — Pending |

---
*Last updated: 2025-02-07 after initialization*
