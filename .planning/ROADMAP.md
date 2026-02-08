# Roadmap: kubectl-fields

## Overview

Build a kubectl plugin that annotates Kubernetes YAML with field ownership comments, delivered in four phases: parsing infrastructure with round-trip fidelity validation, the core annotation engine (parallel descent algorithm matching FieldsV1 trees to YAML nodes), output polish with color and alignment, and extended filtering/display flags. Each phase produces testable, working functionality -- Phase 1 outputs clean YAML with managedFields stripped, Phase 2 adds ownership annotations, Phase 3 adds color and formatting, Phase 4 adds convenience flags.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3, 4): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation + Input Pipeline** - Parse stdin YAML, extract and strip managedFields, validate round-trip fidelity
- [x] **Phase 2: Annotation Engine** - Parallel descent algorithm matching FieldsV1 ownership to YAML nodes with comment injection
- [x] **Phase 3: Output Polish + Color** - Color system, comment alignment, timestamp/display flags
- [ ] **Phase 4: Extended Features** - Manager filtering, name shortening, operation type display

## Phase Details

### Phase 1: Foundation + Input Pipeline
**Goal**: Users can pipe kubectl YAML through the tool and get clean, valid YAML back with managedFields stripped -- proving the parsing foundation works before annotation logic is built
**Depends on**: Nothing (first phase)
**Requirements**: REQ-001, REQ-002, REQ-007, REQ-009, REQ-010, REQ-015, REQ-016, REQ-020
**Success Criteria** (what must be TRUE):
  1. User can pipe `kubectl get deploy -o yaml | kubectl-fields` and receive valid YAML output with the managedFields section removed
  2. Multi-document YAML input (--- separated) produces multi-document output with each document processed independently
  3. List kind input (kind: List with items array) processes each item's managedFields and strips them individually
  4. Input YAML without managedFields passes through unchanged with a warning on stderr
  5. Round-trip fidelity: output YAML preserves the original formatting (indentation, quoting, key ordering) with no changes other than managedFields removal and added comments
**Plans**: 2 plans

Plans:
- [x] 01-01-PLAN.md -- Go project scaffold, YAML parser with multi-doc/List kind, round-trip fidelity tests
- [x] 01-02-PLAN.md -- ManagedFields extraction, FieldsV1 prefix parsing, time formatter, stripping, CLI wiring

### Phase 2: Annotation Engine
**Goal**: Users see ownership annotations on every managed field -- the tool's core value proposition works end-to-end
**Depends on**: Phase 1
**Requirements**: REQ-003, REQ-004, REQ-005, REQ-006, REQ-013
**Success Criteria** (what must be TRUE):
  1. Each managed field in the output has an inline comment showing the manager name and relative timestamp (e.g., `image: nginx # kubectl-client-side-apply (5d ago)`)
  2. With `--above` flag, annotations appear as comments on the line above each managed field with correct indentation
  3. Fields with subresource ownership show the subresource in the annotation (e.g., `# kube-controller-manager (/status) (2h ago)`)
  4. Fields not tracked in any managedFields entry appear bare with no annotation
  5. List items matched by associative keys (k: prefix -- e.g., containers by name, ports by containerPort+protocol) are correctly annotated
**Plans**: 2 plans

Plans:
- [x] 02-01-PLAN.md -- Parallel descent walker with f: field matching and comment injection (inline + above modes)
- [x] 02-02-PLAN.md -- List item matching (k: associative keys, v: set values) and CLI wiring with cobra

### Phase 3: Output Polish + Color
**Goal**: The tool produces professionally formatted, colorized output that is pleasant to read in a terminal and correct when piped
**Depends on**: Phase 2
**Requirements**: REQ-008, REQ-011, REQ-012, REQ-014, REQ-017, REQ-018, REQ-019, REQ-022
**Success Criteria** (what must be TRUE):
  1. When stdout is a TTY, each manager name in annotations is rendered in a distinct, consistent color (same manager always gets same color across invocations)
  2. When output is piped (not a TTY), no ANSI color codes appear in the output
  3. `--color always` forces color even when piped, `--color never` disables color even on TTY, `--no-color` disables color
  4. Inline comments across adjacent lines are aligned into a readable column (not ragged)
  5. `--absolute-time` shows ISO timestamps instead of relative, `--no-time` hides timestamps entirely
**Plans**: 2 plans

Plans:
- [x] 03-01-PLAN.md -- Internal libraries: two-unit time formatting, updated comment format with --mtime modes, output package (color manager, comment alignment, formatter pipeline)
- [x] 03-02-PLAN.md -- CLI flag wiring (--color, --mtime), x/term TTY detection, post-processing pipeline integration, golden file regeneration

### Phase 4: Extended Features
**Goal**: Power users can filter and customize annotation display for specific debugging workflows
**Depends on**: Phase 3
**Requirements**: REQ-021, REQ-023, REQ-024
**Success Criteria** (what must be TRUE):
  1. `--managers=name1,name2` shows annotations only for the specified managers; other managed fields appear bare
  2. `--short-names` shortens well-known manager names (e.g., `kubectl-client-side-apply` becomes `kubectl-csa`)
  3. `--show-operation` includes the operation type in annotations (e.g., `# kubectl-csa [Apply] (5d ago)`)
**Plans**: TBD

Plans:
- [ ] 04-01: Manager filter, name shortening, and operation type display flags

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation + Input Pipeline | 2/2 | Complete | 2026-02-07 |
| 2. Annotation Engine | 2/2 | Complete | 2026-02-08 |
| 3. Output Polish + Color | 2/2 | Complete | 2026-02-08 |
| 4. Extended Features | 0/1 | Not started | - |
