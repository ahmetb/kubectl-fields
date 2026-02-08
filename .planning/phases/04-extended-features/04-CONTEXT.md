# Phase 4: Extended Features - Context

**Gathered:** 2026-02-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Add a `--show-operation` flag that includes the Kubernetes operation type (apply, update) in field ownership annotations. Manager filtering (`--managers`) and name shortening (`--short-names`) from the original roadmap are **not being implemented** in this phase.

</domain>

<decisions>
## Implementation Decisions

### Scope reduction
- Only `--show-operation` is implemented in this phase
- `--managers` filtering and `--short-names` shortening are dropped from this phase entirely

### Operation type format
- Operation type appears **after the timestamp** inside the same parentheses: `# manager (5d ago, apply)`
- Lowercase as-is from Kubernetes: `apply`, `update`
- Same color as the rest of the annotation (manager color)
- Consistent format in both inline and `--above` mode

### Flag design
- Flag name: `--show-operation`
- Opt-in: off by default, user passes `--show-operation` to enable
- Boolean flag, no value argument

### Flag interactions
- With `--no-time`: operation still shown in parens — `# manager (apply)`
- With `--absolute-time`: timestamp then operation — `# manager (2026-02-07T10:30:00Z, apply)`
- With `--mtime` modes: operation always appends after whatever time format is active

### Conflict handling
- Single manager per field assumed (no multi-manager conflicts)
- If multiple managers somehow own the same field, use the first one encountered

### Claude's Discretion
- Internal implementation details for passing operation type through the annotation pipeline
- Test fixture design for operation type combinations

</decisions>

<specifics>
## Specific Ideas

- Format examples the user specified:
  - Default: `# kubectl-client-side-apply (5d ago, apply)`
  - No time: `# kubectl-client-side-apply (apply)`
  - Absolute time: `# kubectl-client-side-apply (2026-02-07T10:30:00Z, apply)`

</specifics>

<deferred>
## Deferred Ideas

- `--managers=name1,name2` manager filtering — removed from this phase, add to backlog
- `--short-names` manager name shortening — removed from this phase, add to backlog

</deferred>

---

*Phase: 04-extended-features*
*Context gathered: 2026-02-07*
