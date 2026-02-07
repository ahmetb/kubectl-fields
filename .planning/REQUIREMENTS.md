# Requirements -- kubectl-fields v1

## REQ-001: Stdin YAML Parsing
**Source:** T1 | **Priority:** P0
Parse Kubernetes YAML from stdin. Handle piped and redirected input. Detect missing stdin gracefully with a helpful error message.

## REQ-002: FieldsV1 Parsing
**Source:** T2 | **Priority:** P0
Parse Kubernetes `managedFields` FieldsV1 format -- `f:` (struct fields), `k:` (list item by key), `v:` (list item by value) prefix notation. Map the recursive ownership tree back to corresponding YAML paths in the resource.

## REQ-003: Inline Comment Placement (Default)
**Source:** T3 | **Priority:** P0
Append `# manager (age)` comment at end of each managed YAML line. Handle values containing `#` in strings correctly.

## REQ-004: Above Comment Placement
**Source:** T4 | **Priority:** P0
`--above` flag places `# manager (age)` comment on the line above each managed field with correct indentation.

## REQ-005: Manager Name Display
**Source:** T5 | **Priority:** P0
Display the `manager` string from each ManagedFieldsEntry in annotations.

## REQ-006: Subresource Display
**Source:** T6 | **Priority:** P0
Show subresource in annotation when present. Format: `manager (/status) (age)`.

## REQ-007: Relative Timestamps (Default)
**Source:** T7 | **Priority:** P0
Human-readable relative timestamps: "5m ago", "3h10m ago", "2d ago".

## REQ-008: Absolute Timestamps
**Source:** T8 | **Priority:** P1
`--absolute-time` flag for absolute timestamps instead of relative.

## REQ-009: Strip managedFields
**Source:** T9 | **Priority:** P0
Remove the `.metadata.managedFields` array from output since its information is presented inline.

## REQ-010: Valid YAML Output
**Source:** T10 | **Priority:** P0
Output must be valid YAML. Comments are valid YAML. Structure must be preserved.

## REQ-011: Color Output on TTY
**Source:** T11 | **Priority:** P1
Auto-detect TTY and apply ANSI colors. Each unique manager name gets a consistent color.

## REQ-012: --no-color Flag
**Source:** T12 | **Priority:** P1
`--no-color` flag to force disable color output.

## REQ-013: Unmanaged Fields Bare
**Source:** T13 | **Priority:** P0
Fields not tracked in managedFields get no annotation.

## REQ-014: --no-time Flag
**Source:** T14 | **Priority:** P1
`--no-time` flag to hide timestamps, showing only manager names.

## REQ-015: Multi-Document YAML
**Source:** D1 | **Priority:** P0
Handle `---` separated multi-document YAML input. Process each document independently.

## REQ-016: List Kind Support
**Source:** D2 | **Priority:** P0
Handle `kind: List` wrapping multiple resources. Unwrap `.items[]`, process each, re-emit.

## REQ-017: Comment Alignment
**Source:** D3 | **Priority:** P1
Align inline comments across adjacent lines to form a readable column.

## REQ-018: Per-Manager Deterministic Color
**Source:** D4 | **Priority:** P1
Same manager always gets the same color via hash-based palette assignment. Consistent across invocations.

## REQ-019: --color Tri-State Flag
**Source:** D5 | **Priority:** P1
`--color auto/always/never` for flexible color control. `auto` = TTY detection. `always` = force (for `less -R`). `never` = disable.

## REQ-020: Graceful Missing managedFields
**Source:** D6 | **Priority:** P1
If input has no managedFields, output YAML unchanged with a stderr warning.

## REQ-021: Manager Name Shortening
**Source:** D7 | **Priority:** P2
Optional `--short-names` flag to shorten common manager names (e.g., `kubectl-client-side-apply` -> `kubectl-csa`).

## REQ-022: Color Palette Variety
**Source:** D8 | **Priority:** P1
Palette of 8-16 distinct ANSI colors for manager names (not single-color like predecessor).

## REQ-023: --managers Filter
**Source:** D9 | **Priority:** P2
`--managers=name1,name2` flag to show annotations only for specific managers.

## REQ-024: Operation Type Display
**Source:** D10 | **Priority:** P2
`--show-operation` flag to display Apply/Update operation type. Format: `manager [Apply] (age)`.

## Out of Scope (Anti-Features)

- **A1:** Live cluster querying -- stdin-only, Unix pipe philosophy
- **A2:** Conflict detection/resolution -- show facts, not server-side logic
- **A3:** YAML mutation -- read-only annotation only
- **A4:** JSON output -- JSON has no comments
- **A5:** Interactive/TUI mode -- pipe-friendly CLI
- **A6:** Configuration file -- flags only
- **A7:** Per-manager color customization -- good defaults + --no-color
- **A8:** Diff mode -- separate tool/workflow
- **A9:** File input (`-f`) -- stdin covers all via `< file.yaml`
- **A10:** Auto-update/version checking -- rely on krew

## Traceability

| REQ | Feature | Priority | Phase | Status |
|-----|---------|----------|-------|--------|
| REQ-001 | Stdin YAML parsing | P0 | Phase 1 | Complete |
| REQ-002 | FieldsV1 parsing | P0 | Phase 1 | Complete |
| REQ-003 | Inline comment placement | P0 | Phase 2 | Pending |
| REQ-004 | Above comment placement | P0 | Phase 2 | Pending |
| REQ-005 | Manager name display | P0 | Phase 2 | Pending |
| REQ-006 | Subresource display | P0 | Phase 2 | Pending |
| REQ-007 | Relative timestamps | P0 | Phase 1 | Complete |
| REQ-008 | Absolute timestamps | P1 | Phase 3 | Pending |
| REQ-009 | Strip managedFields | P0 | Phase 1 | Complete |
| REQ-010 | Valid YAML output | P0 | Phase 1 | Complete |
| REQ-011 | Color output on TTY | P1 | Phase 3 | Pending |
| REQ-012 | --no-color flag | P1 | Phase 3 | Pending |
| REQ-013 | Unmanaged fields bare | P0 | Phase 2 | Pending |
| REQ-014 | --no-time flag | P1 | Phase 3 | Pending |
| REQ-015 | Multi-document YAML | P0 | Phase 1 | Complete |
| REQ-016 | List kind support | P0 | Phase 1 | Complete |
| REQ-017 | Comment alignment | P1 | Phase 3 | Pending |
| REQ-018 | Per-manager deterministic color | P1 | Phase 3 | Pending |
| REQ-019 | --color tri-state flag | P1 | Phase 3 | Pending |
| REQ-020 | Graceful missing managedFields | P1 | Phase 1 | Complete |
| REQ-021 | Manager name shortening | P2 | Phase 4 | Pending |
| REQ-022 | Color palette variety | P1 | Phase 3 | Pending |
| REQ-023 | --managers filter | P2 | Phase 4 | Pending |
| REQ-024 | Operation type display | P2 | Phase 4 | Pending |

---
*Generated: 2026-02-07 from FEATURES.md research*
*Updated: 2026-02-07 with phase mappings from ROADMAP.md*
