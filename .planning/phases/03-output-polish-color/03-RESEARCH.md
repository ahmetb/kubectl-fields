# Phase 3: Output Polish + Color - Research

**Researched:** 2026-02-07
**Domain:** Terminal output formatting (ANSI color, comment alignment, timestamp display)
**Confidence:** HIGH

## Summary

Phase 3 transforms the raw annotated YAML output from Phase 2 into polished, professionally formatted terminal output. Three primary subsystems are needed: (1) a color manager that assigns bright ANSI colors to managers and respects TTY/NO_COLOR/--color semantics, (2) a text post-processor that aligns inline comments into per-block columns, and (3) updates to the comment format and timestamp system to support `--mtime relative|absolute|hide` and the revised annotation format.

The critical architectural decision is that color and alignment must operate as a **text post-processing step** after go-yaml encoding, not within the YAML AST. go-yaml has no API for column-aligned comments or colored text -- it simply outputs `# comment` with a single space separator. The post-processor reads the encoded YAML line-by-line, identifies comment-bearing lines, groups them into alignment blocks, pads to a uniform column, and optionally wraps the comment portion in ANSI escape codes.

**Primary recommendation:** Add an `internal/output/` package containing the color manager, comment aligner, and formatter. Post-process the encoded YAML text as a string, not the AST. Use `golang.org/x/term` for TTY detection. Use raw ANSI escape sequences (no color library needed for 8 bright colors).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Bright/vivid palette with 8 distinct colors
- Colorize the full comment (manager name + subresource + timestamp) in the manager's assigned color
- YAML text stays plain (no syntax highlighting) -- only annotations get color
- Hash `#` is part of the colored comment
- Insertion-order color mapping: first manager encountered gets color 1, second gets color 2, etc.
- Consistent within a single invocation, may vary between runs
- `--color auto|always|never` flag (default: `auto`)
- `auto` mode: color when stdout is a TTY, no color when piped
- Respect `NO_COLOR` environment variable (no-color.org convention) -- disables color unless `--color always` explicitly overrides
- Per-block alignment: adjacent annotated lines form a group and align their comments to the same column
- A line without an annotation breaks the group
- Alignment column per block = longest YAML line in the block + minimum gap
- Minimum gap: 2 spaces between YAML content and comment
- Long lines: if a YAML line exceeds the block's alignment column, push the comment right with the 2-space minimum gap (other lines in the block stay aligned)
- Above mode (`--above`): comments left-align to the field's indentation level, no column alignment
- Alignment applies to both TTY and piped output (always on)
- Single flag: `--mtime relative|absolute|hide` (default: `relative`)
- Replaces previously planned `--absolute-time` and `--no-time` flags
- Relative format: two-unit granularity (e.g., `2h15m ago`, `3d12h ago`)
- Relative units: s, m, h, d, w, mo, y (full range)
- Absolute format: full ISO 8601 (`2026-02-07T12:00:00Z`)
- Hide: removes timestamp entirely from annotation, comment becomes `# manager-name /subresource`
- Slash-prefix notation: `/status`
- Shown after manager name: `# manager-name /status (2h15m ago)`

### Claude's Discretion
- Exact 8-color bright palette values
- Alignment algorithm implementation details
- Two-unit rollover thresholds (when does `59m` become `1h`? when does `23h59m` become `1d`?)
- Edge cases: what happens with 0-second-old timestamps

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `golang.org/x/term` | v0.39.0 | TTY detection | Official Go team package; `term.IsTerminal(int(fd))` is the canonical way to detect TTY in Go |
| Raw ANSI sequences | N/A | Color output | Only 8 bright colors needed; SGR 90-97 are trivial to implement directly without a color library |
| `github.com/spf13/cobra` | v1.10.2 (existing) | CLI flags | Already in project; handles `--color`, `--mtime` flag registration |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/spf13/pflag` | v1.0.9 (existing, indirect) | Custom flag Value types | For `--color` and `--mtime` enum validation via pflag.Value interface |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `golang.org/x/term` | `github.com/mattn/go-isatty` | go-isatty is more popular (5400+ importers) but adds another third-party dep; x/term is official Go team |
| Raw ANSI | `github.com/fatih/color` | fatih/color adds unnecessary dep for just 8 color codes; also bundles its own isatty |
| Raw ANSI | `github.com/muesli/termenv` | Overkill for our needs; brings in large dependency tree |

**Installation:**
```bash
go get golang.org/x/term@latest
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
  output/
    color.go          # ColorManager: palette, manager->color mapping, ANSI wrapping
    color_test.go
    align.go          # AlignComments: per-block inline comment alignment
    align_test.go
    formatter.go      # FormatOutput: orchestrates align + color pipeline
    formatter_test.go
  annotate/
    annotate.go       # Updated formatComment for --mtime modes
  timeutil/
    relative.go       # Enhanced two-unit granularity for all time ranges
```

### Pattern 1: Text Post-Processing Pipeline

**What:** After go-yaml encodes the YAML, the output text is processed line-by-line to align comments and inject ANSI colors. This is necessary because go-yaml provides no API for column-aligned comments or colored text.

**When to use:** Always -- this is the only viable approach given go-yaml's limitations.

**Pipeline order:**
```
YAML AST -> go-yaml Encode -> plain text
                                  |
                                  v
                          1. Split into lines
                          2. Identify comment-bearing lines (inline # comments)
                          3. Group adjacent annotated lines into blocks
                          4. For each block: compute alignment column, pad
                          5. If color enabled: wrap comment portion in ANSI codes
                          6. Join lines, write to stdout
```

**Why alignment BEFORE color:** ANSI escape codes add invisible characters that break column calculations. Align first on clean text, then inject color codes into the already-padded comments.

**Example:**
```go
// Step 1: Encode to plain text
var buf bytes.Buffer
parser.EncodeDocuments(&buf, docs)
plainText := buf.String()

// Step 2: Align comments (text-level)
aligned := output.AlignComments(plainText)

// Step 3: Colorize if enabled (text-level)
if colorEnabled {
    colorized := output.Colorize(aligned, colorMgr)
    io.WriteString(os.Stdout, colorized)
} else {
    io.WriteString(os.Stdout, aligned)
}
```

### Pattern 2: Insertion-Order Color Manager

**What:** A color manager that assigns colors to manager names in the order they are first encountered during annotation. Uses a slice of 8 bright ANSI colors and a map for lookup.

**When to use:** Always during the colorization step.

**Example:**
```go
type ColorManager struct {
    palette []string        // 8 ANSI escape sequences
    assign  map[string]int  // manager name -> palette index
    order   []string        // insertion order for determinism
}

func (cm *ColorManager) Color(managerName string) string {
    if idx, ok := cm.assign[managerName]; ok {
        return cm.palette[idx % len(cm.palette)]
    }
    idx := len(cm.order)
    cm.assign[managerName] = idx
    cm.order = append(cm.order, managerName)
    return cm.palette[idx % len(cm.palette)]
}

func (cm *ColorManager) Wrap(text, managerName string) string {
    return cm.Color(managerName) + text + "\x1b[0m"
}
```

### Pattern 3: Per-Block Comment Alignment

**What:** Groups consecutive lines that have inline comments (`# ...`) into blocks. Within each block, all comments are aligned to the same column. A line without a comment breaks the block.

**When to use:** For inline mode output (not `--above` mode, which has no column alignment per user decision).

**Algorithm:**
```
1. Parse each line into (yaml_content, comment) tuple
2. Group consecutive lines that have comments into blocks
3. For each block:
   a. alignment_col = max(len(yaml_content) for all lines in block) + MIN_GAP
   b. For each line in block:
      - If len(yaml_content) > alignment_col - MIN_GAP:
          padded = yaml_content + "  " + comment  (2-space minimum)
      - Else:
          padded = yaml_content.ljust(alignment_col) + comment
4. Lines without comments pass through unchanged
```

**Critical detail:** The "yaml_content" portion of the line is everything BEFORE the `# ` comment marker. go-yaml outputs exactly one space before `#`. The splitter should find the LAST ` # ` in the line (not the first) to avoid breaking on YAML values containing `#` in quoted strings. However, go-yaml only adds `# ` for LineComment -- it never appears in YAML value text without quoting. A safe approach: split on ` # ` where the `#` is followed by a manager name pattern.

**Simpler approach:** Since we control the comment format, we can use a unique marker. The comment always starts with a manager name (no `#` prefix -- go-yaml adds that). We can match ` # ` and check that what follows is NOT a YAML comment (i.e., not a standalone line starting with `#`). For inline comments, go-yaml places `# comment` after the YAML value on the same line, always preceded by a space. For HeadComment (above mode), the `#` starts at the line's indentation level.

**Even simpler:** For inline mode, detect lines where `# ` appears after YAML content (i.e., the line does not start with optional whitespace + `#`). If the line has content before the `#`, it's an inline comment. If the line is just whitespace + `# ...`, it's an above-mode comment (HeadComment).

### Pattern 4: Comment Format Update

**What:** The annotation format changes from Phase 2's format to Phase 3's format.

**Current format (Phase 2):**
```
manager (/subresource) (age)
```

**New format (Phase 3):**
```
manager /subresource (age)        # with --mtime relative (default)
manager /subresource (2026-02-07T12:00:00Z)  # with --mtime absolute
manager /subresource              # with --mtime hide
manager (age)                     # no subresource, --mtime relative
manager                           # no subresource, --mtime hide
```

Note: subresource drops the parentheses wrapper, uses space + slash-prefix notation directly.

### Pattern 5: TTY / NO_COLOR / --color Resolution

**What:** A function that resolves the final color-enabled state from the combination of `--color` flag, `NO_COLOR` env var, and TTY detection.

**Decision table:**
```
--color=always  -> color ON  (overrides everything, including NO_COLOR)
--color=never   -> color OFF
--color=auto    -> check NO_COLOR env: if set and non-empty -> color OFF
                   else -> check TTY: if stdout is TTY -> color ON, else color OFF
```

**Example:**
```go
func ResolveColor(flag string) bool {
    switch flag {
    case "always":
        return true
    case "never":
        return false
    default: // "auto"
        if os.Getenv("NO_COLOR") != "" {
            return false
        }
        return term.IsTerminal(int(os.Stdout.Fd()))
    }
}
```

### Anti-Patterns to Avoid
- **Modifying YAML AST for color:** Never put ANSI codes in yaml.Node comments. go-yaml would double-escape them or break encoding.
- **Color before alignment:** Computing column widths on text that contains invisible ANSI sequences will produce wrong alignment. Always align first, colorize second.
- **Global mutable color state:** The ColorManager should be passed explicitly, not stored in package-level vars. This enables testing.
- **Regex-heavy comment parsing:** The comment format is well-defined. Simple string splitting on ` # ` is sufficient and faster than regex.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TTY detection | `os.Stat()` mode check, `syscall.Isatty` | `golang.org/x/term.IsTerminal()` | Cross-platform, handles edge cases (Cygwin, etc.) |
| ANSI color library | Full-featured color package | Raw `\x1b[91m` .. `\x1b[97m` + `\x1b[0m` | Only 8 colors needed; a library is overkill |
| Custom flag enum type | Runtime string validation | pflag.Value interface | Gives usage errors automatically, shell completion support |
| Time formatting | Custom duration parser | Enhance existing `timeutil.FormatRelativeTime` | Foundation already exists; just needs two-unit extension |

**Key insight:** The scope is small enough that no additional libraries beyond `golang.org/x/term` are needed. The 8-color ANSI palette, text alignment, and flag validation are all simple enough to implement directly.

## Common Pitfalls

### Pitfall 1: ANSI Codes Breaking Alignment Calculations
**What goes wrong:** If you compute column widths on text that already contains ANSI escape sequences, the invisible bytes count toward the width, making columns misaligned.
**Why it happens:** ANSI codes like `\x1b[91m` are 4-5 bytes but render as zero width in terminals.
**How to avoid:** Always align on plain text first, then inject color codes into already-aligned output. The pipeline MUST be: encode -> align -> colorize.
**Warning signs:** Comments appear misaligned in colored output but correct in `--color never` output.

### Pitfall 2: Splitting Comments on `#` Inside YAML Values
**What goes wrong:** A YAML value like `image: nginx:1.14 # comment` looks like it has an inline comment, but `#` could also appear in unquoted YAML values or inside quoted strings.
**Why it happens:** `#` is valid in YAML values when preceded by a non-space character (unquoted) or inside quotes.
**How to avoid:** go-yaml ALWAYS outputs LineComment with a space before `#`. The split pattern should be ` # ` (space-hash-space). Additionally, go-yaml quotes values containing bare `#` characters, so they won't appear as ` # ` in the output. For extra safety, only treat ` # ` as a comment if it appears after YAML content on the same line (not as a standalone head comment line).
**Warning signs:** Lines with `#` in values get incorrectly split.

### Pitfall 3: Above-Mode Comments Being Colorized Incorrectly
**What goes wrong:** Above-mode (`--above`) comments are full lines starting with `# ` at some indentation. The post-processor must colorize these too, but the detection logic differs from inline comments.
**Why it happens:** Inline comments have YAML content before `# `, above comments have only whitespace before `# `.
**How to avoid:** Handle both cases: (1) inline comment = line has non-whitespace before ` # `, colorize the ` # ...` suffix; (2) above comment = line is `^\s*# ...`, colorize the `# ...` portion.
**Warning signs:** Above-mode comments are not colorized, or alignment is attempted on above-mode comments.

### Pitfall 4: Inconsistent Manager Name Extraction for Coloring
**What goes wrong:** The colorizer needs to extract the manager name from the comment text to look up its color. If the extraction regex/logic doesn't match the format produced by `formatComment`, colors break.
**Why it happens:** The format string is defined in `annotate.go` and parsed in `output/color.go`. If one changes without the other, extraction fails.
**How to avoid:** Define the manager name as everything from the start of the comment text (after `# `) up to the first ` /` (subresource) or ` (` (timestamp) or end of string (hide mode). A simple approach: split on space, take the first token -- but manager names can contain hyphens and dots (e.g., `kubectl-client-side-apply`, `kube-controller-manager`). Better: the manager name is everything before the first ` /` or ` (` whichever comes first, or the entire comment if neither is present.
**Warning signs:** Different managers get the same color, or colors change mid-output.

### Pitfall 5: Two-Unit Time Formatting Inconsistency
**What goes wrong:** The existing `FormatRelativeTime` only does two-unit granularity for minutes+seconds and hours+minutes. Days, months, and years are single-unit. The CONTEXT requires two-unit for all ranges (e.g., `3d12h ago`, `2w3d ago`, `3mo2w ago`, `1y2mo ago`).
**Why it happens:** The Phase 1 implementation was minimal; Phase 3 needs full coverage.
**How to avoid:** Systematically extend `FormatRelativeTime` to compute two-unit values for every range: `d+h`, `w+d`, `mo+w` (or `mo+d`), `y+mo`. Define rollover thresholds clearly.
**Warning signs:** Timestamps show `5d ago` instead of `5d12h ago`.

### Pitfall 6: go-yaml Comment Placement Quirks with Alignment
**What goes wrong:** go-yaml places the `# ` separator with exactly one space before `#` in inline mode. When a line has a very long YAML value (e.g., the long JSON annotation value), the comment appears on the same line far to the right.
**Why it happens:** go-yaml always puts LineComment on the same line as the value, regardless of length.
**How to avoid:** The alignment algorithm should handle these naturally -- if a line's YAML content exceeds the block's alignment column, it gets the 2-space minimum gap, and other lines in the block remain at the standard alignment column.
**Warning signs:** Very long lines push the alignment column impossibly far right for the whole block.

## Code Examples

### ANSI Bright Color Palette (8 colors)
```go
// Recommended bright palette for dark terminal backgrounds.
// Uses SGR codes 91-96 (bright red through bright cyan) + 93 (bright yellow)
// and 92 (bright green). Avoids bright black (90, hard to read) and
// bright white (97, too close to default text).
var BrightPalette = []string{
    "\x1b[96m", // Bright Cyan
    "\x1b[92m", // Bright Green
    "\x1b[93m", // Bright Yellow
    "\x1b[95m", // Bright Magenta
    "\x1b[91m", // Bright Red
    "\x1b[94m", // Bright Blue
    "\x1b[36m", // Cyan (standard, for contrast variety)
    "\x1b[33m", // Yellow (standard, for contrast variety)
}
const Reset = "\x1b[0m"
```

Rationale: Start with cyan/green (high readability on dark backgrounds), follow with yellow/magenta/red/blue. The first two colors are used most (the most common managers like `kubectl-client-side-apply` and `kube-controller-manager`). Avoids bright black (invisible on dark backgrounds) and bright white (blends with default text on light backgrounds).

### Comment Splitting for Inline Lines
```go
// splitInlineComment splits a YAML line into content and comment parts.
// Returns (content, comment, hasComment).
// content includes trailing whitespace up to the "# ".
// comment includes the "# " prefix.
func splitInlineComment(line string) (string, string, bool) {
    // Find last occurrence of " # " to avoid false matches
    idx := strings.LastIndex(line, " # ")
    if idx < 0 {
        return line, "", false
    }
    // Verify there's actual YAML content before the comment
    // (not just whitespace, which would be an above-mode comment on its own line)
    content := line[:idx]
    if strings.TrimSpace(content) == "" {
        return line, "", false // This is a head comment, not inline
    }
    comment := line[idx+1:] // includes "# ..."
    return content, comment, true
}
```

### Per-Block Alignment Algorithm
```go
const MinGap = 2

func AlignComments(text string) string {
    lines := strings.Split(text, "\n")
    result := make([]string, len(lines))

    i := 0
    for i < len(lines) {
        // Find a block of consecutive lines with inline comments
        blockStart := i
        for i < len(lines) {
            _, _, has := splitInlineComment(lines[i])
            if !has {
                break
            }
            i++
        }

        if blockStart == i {
            // No comment on this line, pass through
            result[i] = lines[i]
            i++
            continue
        }

        // Calculate alignment column for this block
        maxContent := 0
        for j := blockStart; j < i; j++ {
            content, _, _ := splitInlineComment(lines[j])
            if len(content) > maxContent {
                maxContent = len(content)
            }
        }
        alignCol := maxContent + MinGap

        // Apply alignment
        for j := blockStart; j < i; j++ {
            content, comment, _ := splitInlineComment(lines[j])
            if len(content) > alignCol-MinGap {
                // Long line: use minimum gap
                result[j] = content + "  " + comment
            } else {
                // Normal line: pad to alignment column
                padding := alignCol - len(content)
                result[j] = content + strings.Repeat(" ", padding) + comment
            }
        }
    }

    return strings.Join(result, "\n")
}
```

### pflag.Value for --color Flag
```go
type ColorFlag string

const (
    ColorAuto   ColorFlag = "auto"
    ColorAlways ColorFlag = "always"
    ColorNever  ColorFlag = "never"
)

func (f *ColorFlag) String() string { return string(*f) }

func (f *ColorFlag) Set(val string) error {
    switch ColorFlag(val) {
    case ColorAuto, ColorAlways, ColorNever:
        *f = ColorFlag(val)
        return nil
    default:
        return fmt.Errorf("must be one of: auto, always, never")
    }
}

func (f *ColorFlag) Type() string { return "string" }
```

### pflag.Value for --mtime Flag
```go
type MtimeFlag string

const (
    MtimeRelative MtimeFlag = "relative"
    MtimeAbsolute MtimeFlag = "absolute"
    MtimeHide     MtimeFlag = "hide"
)

func (f *MtimeFlag) String() string { return string(*f) }

func (f *MtimeFlag) Set(val string) error {
    switch MtimeFlag(val) {
    case MtimeRelative, MtimeAbsolute, MtimeHide:
        *f = MtimeFlag(val)
        return nil
    default:
        return fmt.Errorf("must be one of: relative, absolute, hide")
    }
}

func (f *MtimeFlag) Type() string { return "string" }
```

### Updated formatComment with --mtime Support
```go
func formatComment(info AnnotationInfo, now time.Time, mtime MtimeFlag) string {
    var parts []string
    parts = append(parts, info.Manager)

    if info.Subresource != "" {
        parts = append(parts, "/"+info.Subresource)
    }

    switch mtime {
    case MtimeRelative:
        age := timeutil.FormatRelativeTime(now, info.Time)
        parts = append(parts, "("+age+")")
    case MtimeAbsolute:
        parts = append(parts, "("+info.Time.UTC().Format(time.RFC3339)+")")
    case MtimeHide:
        // No timestamp
    }

    return strings.Join(parts, " ")
}
```

### Enhanced Two-Unit Relative Time
```go
func FormatRelativeTime(now, then time.Time) string {
    d := now.Sub(then)
    if d < 0 {
        return "just now"
    }

    totalSec := int(d.Seconds())
    if totalSec == 0 {
        return "0s ago"
    }

    // Extract each unit
    years   := totalSec / (365 * 24 * 3600)
    remain  := totalSec % (365 * 24 * 3600)
    months  := remain / (30 * 24 * 3600)
    remain   = remain % (30 * 24 * 3600)
    weeks   := remain / (7 * 24 * 3600)
    remain   = remain % (7 * 24 * 3600)
    days    := remain / (24 * 3600)
    remain   = remain % (24 * 3600)
    hours   := remain / 3600
    remain   = remain % 3600
    minutes := remain / 60
    seconds := remain % 60

    // Two-unit output: use the two largest non-zero units
    units := []struct{ val int; suffix string }{
        {years, "y"}, {months, "mo"}, {weeks, "w"},
        {days, "d"}, {hours, "h"}, {minutes, "m"}, {seconds, "s"},
    }

    var result string
    count := 0
    for _, u := range units {
        if u.val > 0 && count < 2 {
            result += fmt.Sprintf("%d%s", u.val, u.suffix)
            count++
        }
        if count == 2 {
            break
        }
    }

    if result == "" {
        return "0s ago"
    }
    return result + " ago"
}
```

**Recommended rollover thresholds (Claude's discretion):**
- Seconds only: 0-59s -> `Xs ago`
- Minutes+seconds: 1m-59m59s -> `XmYs ago`
- Hours+minutes: 1h-23h59m -> `XhYm ago`
- Days+hours: 1d-6d23h -> `XdYh ago`
- Weeks+days: 1w-4w6d -> `XwYd ago`
- Months+weeks: 1mo-11mo -> `XmoYw ago` (or `Xmo ago` if 0 weeks)
- Years+months: 1y+ -> `XyYmo ago`

**Edge case for 0-second timestamps:** Display as `0s ago` (already handled).

### Manager Name Extraction from Comment Text
```go
// extractManagerName gets the manager name from a comment string.
// Comment format: "manager /sub (age)" or "manager (age)" or "manager /sub" or "manager"
func extractManagerName(comment string) string {
    // Strip "# " prefix if present
    text := strings.TrimPrefix(comment, "# ")

    // Manager name is everything before first " /" (subresource) or " (" (timestamp)
    if idx := strings.Index(text, " /"); idx >= 0 {
        return text[:idx]
    }
    if idx := strings.Index(text, " ("); idx >= 0 {
        return text[:idx]
    }
    return text // hide mode, no subresource
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `--absolute-time` + `--no-time` (two flags) | `--mtime relative\|absolute\|hide` (one flag) | Phase 3 design | Simpler CLI, no conflicting flags |
| Hash-based color (REQ-018) | Insertion-order color (user decision) | Phase 3 design | Simpler implementation, same visual effect |
| Single-unit time (`5d ago`) | Two-unit time (`5d12h ago`) | Phase 3 | More precise timestamps |
| `manager (/sub) (age)` format | `manager /sub (age)` format | Phase 3 | Cleaner annotation style |

**Changes to existing code:**
- `internal/timeutil/relative.go`: Rewrite to support full two-unit granularity with weeks
- `internal/annotate/annotate.go`: Update `formatComment` to accept mtime mode, change subresource format
- `internal/annotate/annotate.go`: Update `Options` struct with `MtimeMode` field
- `cmd/kubectl-fields/main.go`: Add `--color`, `--mtime` flags; wire post-processing pipeline
- Golden test files: Must be regenerated due to format changes

## Open Questions

1. **Comment detection in edge cases**
   - What we know: go-yaml outputs inline comments as `value # comment` with a single space. For above mode, comments are standalone `# comment` lines.
   - What's unclear: Whether go-yaml ever outputs a `#` in a non-quoted value that could confuse the splitter. From testing, go-yaml quotes any value containing ` # `, so this is safe.
   - Recommendation: Implement the simple ` # ` split and add a test case with `#` in values to verify.

2. **Weeks unit in relative time**
   - What we know: CONTEXT lists units as `s, m, h, d, w, mo, y` -- weeks are included.
   - What's unclear: Current `FormatRelativeTime` does not use weeks at all. Adding weeks changes existing golden file timestamps.
   - Recommendation: Add weeks. Update golden files accordingly. The rollover from days to weeks is at 7d.

3. **Color palette ordering for maximum contrast**
   - What we know: Need 8 bright colors. The most common managers (kubectl-client-side-apply, kube-controller-manager) should get the most distinct, readable colors.
   - What's unclear: Optimal ordering depends on the user's terminal theme (dark vs light background).
   - Recommendation: Use bright cyan, green, yellow, magenta, red, blue + 2 standard colors as the palette. This gives good contrast on both dark and light backgrounds. The first two colors (cyan, green) are the most universally readable.

## Sources

### Primary (HIGH confidence)
- `golang.org/x/term` v0.39.0 -- `IsTerminal(int) bool` for TTY detection. Verified via `go list -m`.
- ANSI SGR codes 90-97 (bright foreground) -- standard terminal escape sequences, universally supported
- no-color.org -- `NO_COLOR` env var convention: "when present and not an empty string, prevents ANSI color"
- go-yaml encoding behavior -- verified by examining golden file output in project testdata

### Secondary (MEDIUM confidence)
- pflag.Value interface (`String() string`, `Set(string) error`, `Type() string`) -- from official pflag docs
- cobra `RegisterFlagCompletionFunc` for enum completion -- from official cobra docs

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- `golang.org/x/term` is the canonical Go TTY detection package; ANSI codes are a universal standard
- Architecture: HIGH -- text post-processing is the only viable approach; go-yaml has no alignment/color API. Confirmed by examining project's actual go-yaml output.
- Pitfalls: HIGH -- identified from actual codebase analysis and go-yaml behavior
- Time formatting: HIGH -- existing code examined; gaps in two-unit coverage confirmed by reading `timeutil/relative.go`
- Color palette values: MEDIUM -- palette ordering is aesthetic/subjective; recommended values are reasonable defaults

**Research date:** 2026-02-07
**Valid until:** 2026-03-07 (stable domain, 30-day validity)
