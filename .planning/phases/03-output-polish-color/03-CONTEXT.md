# Phase 3: Output Polish + Color - Context

**Gathered:** 2026-02-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Produce professionally formatted, colorized terminal output with aligned comments and configurable timestamp display. The annotation engine (Phase 2) already produces correct annotations — this phase makes them pleasant to read. Color system, comment alignment, and timestamp/display flags.

</domain>

<decisions>
## Implementation Decisions

### Color palette
- Bright/vivid palette with 8 distinct colors
- Colorize the full comment (manager name + subresource + timestamp) in the manager's assigned color
- YAML text stays plain (no syntax highlighting) — only annotations get color
- Hash `#` is part of the colored comment

### Color management
- Insertion-order color mapping: first manager encountered gets color 1, second gets color 2, etc.
- Consistent within a single invocation, may vary between runs
- `--color auto|always|never` flag (default: `auto`)
- `auto` mode: color when stdout is a TTY, no color when piped
- Respect `NO_COLOR` environment variable (no-color.org convention) — disables color unless `--color always` explicitly overrides

### Comment alignment
- Per-block alignment: adjacent annotated lines form a group and align their comments to the same column
- A line without an annotation breaks the group
- Alignment column per block = longest YAML line in the block + minimum gap
- Minimum gap: 2 spaces between YAML content and comment
- Long lines: if a YAML line exceeds the block's alignment column, push the comment right with the 2-space minimum gap (other lines in the block stay aligned)
- Above mode (`--above`): comments left-align to the field's indentation level, no column alignment
- Alignment applies to both TTY and piped output (always on)

### Timestamp display
- Single flag: `--mtime relative|absolute|hide` (default: `relative`)
- Replaces previously planned `--absolute-time` and `--no-time` flags
- Relative format: two-unit granularity (e.g., `2h15m ago`, `3d12h ago`)
- Relative units: s, m, h, d, w, mo, y (full range)
- Absolute format: full ISO 8601 (`2026-02-07T12:00:00Z`)
- Hide: removes timestamp entirely from annotation, comment becomes `# manager-name /subresource`

### Subresource display
- Slash-prefix notation: `/status`
- Shown after manager name: `# manager-name /status (2h15m ago)`

### Claude's Discretion
- Exact 8-color bright palette values
- Alignment algorithm implementation details
- Two-unit rollover thresholds (when does `59m` become `1h`? when does `23h59m` become `1d`?)
- Edge cases: what happens with 0-second-old timestamps

</decisions>

<specifics>
## Specific Ideas

- Color palette should be "bright" like git diff colors — high visibility against dark terminal backgrounds
- The `--mtime` flag consolidation was user-initiated — single flag with `relative|absolute|hide` is preferred over multiple boolean flags
- Alignment should feel natural: if three consecutive fields are annotated, their comments line up; a bare field breaks the group and the next annotated run starts fresh

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-output-polish-color*
*Context gathered: 2026-02-07*
