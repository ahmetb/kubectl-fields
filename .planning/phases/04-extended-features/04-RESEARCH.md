# Phase 4: Extended Features - Research

**Researched:** 2026-02-07
**Domain:** CLI flag addition, annotation pipeline extension, Kubernetes managedFields operation types
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Scope reduction**: Only `--show-operation` is implemented in this phase. `--managers` filtering and `--short-names` shortening are dropped entirely.
- **Operation type format**: Operation type appears **after the timestamp** inside the same parentheses: `# manager (5d ago, apply)`. Lowercase as-is from Kubernetes: `apply`, `update`. Same color as the rest of the annotation (manager color). Consistent format in both inline and `--above` mode.
- **Flag design**: Flag name `--show-operation`. Opt-in (off by default). Boolean flag, no value argument.
- **Flag interactions**:
  - With `--mtime hide` (referred to as `--no-time` in CONTEXT): operation still shown in parens -- `# manager (apply)`
  - With `--mtime absolute`: timestamp then operation -- `# manager (2026-02-07T10:30:00Z, apply)`
  - With `--mtime` modes: operation always appends after whatever time format is active
- **Conflict handling**: Single manager per field assumed. If multiple managers own the same field, use the first one encountered (existing last-writer-wins behavior already handles this).

### Claude's Discretion
- Internal implementation details for passing operation type through the annotation pipeline
- Test fixture design for operation type combinations

### Deferred Ideas (OUT OF SCOPE)
- `--managers=name1,name2` manager filtering -- removed from this phase
- `--short-names` manager name shortening -- removed from this phase
</user_constraints>

## Summary

This phase adds a single `--show-operation` boolean flag that optionally includes the Kubernetes operation type (`apply` or `update`) in field ownership annotations. The scope is deliberately narrow: one new flag, one new field in the annotation pipeline, and formatting logic that composes with all existing `--mtime` modes.

The implementation requires changes in four locations: (1) `AnnotationInfo` struct gains an `Operation` field, (2) `formatComment` gains operation-aware formatting, (3) `annotate.Options` gains a `ShowOperation` bool, and (4) the CLI adds a `--show-operation` flag wired through to the options struct. The `extractManagerName` function in the output/color package already handles the new format correctly because it stops at the first ` (` delimiter, which remains unchanged.

**Primary recommendation:** Add `Operation string` to `AnnotationInfo`, add `ShowOperation bool` to `Options`, extend `formatComment` to append `, operation` inside parentheses when enabled, and wire the new flag through the CLI. This is a contained change with no architectural risk.

## Standard Stack

No new libraries needed. This phase uses the exact same stack as Phases 1-3.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go.yaml.in/yaml/v3 | v3.0.4 | YAML parsing, node manipulation, encoding | Official fork; already in use |
| github.com/spf13/cobra | v1.10.2 | CLI framework | Already in use for command/flag handling |
| github.com/spf13/pflag | v1.0.9 | Flag parsing (via cobra) | Already in use; provides Bool flag type |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/stretchr/testify | v1.11.1 | Test assertions | Already used in all test files |

### No New Dependencies
This phase requires zero new dependencies. All changes are within existing packages using existing libraries.

## Architecture Patterns

### Current Pipeline (unchanged)
```
stdin -> ParseDocuments -> UnwrapListKind -> ExtractManagedFields -> Annotate -> StripManagedFields -> EncodeDocuments -> buffer -> FormatOutput -> stdout
```

The pipeline is unchanged. The only additions flow through the existing `annotate.Options` and `AnnotationInfo` structs.

### Pattern 1: Extending AnnotationInfo with Operation
**What:** Add `Operation string` field to `AnnotationInfo` struct and populate it from `ManagedFieldsEntry.Operation`
**When to use:** Always -- the operation data is already extracted in Phase 1 (`managed.ManagedFieldsEntry` already has an `Operation string` field)
**Confidence:** HIGH -- verified by reading the source code

The data path is already in place:
1. `managed.parseManagedFieldEntry` (extract.go:69) already reads `operation` from YAML into `ManagedFieldsEntry.Operation`
2. `annotationFrom` (walker.go:222-228) creates `AnnotationInfo` from `ManagedFieldsEntry` -- currently copies Manager, Subresource, Time but NOT Operation
3. Adding `Operation` to `AnnotationInfo` and copying it in `annotationFrom` completes the data flow

```go
// Current AnnotationInfo (walker.go)
type AnnotationInfo struct {
    Manager     string
    Subresource string
    Time        time.Time
}

// Extended AnnotationInfo
type AnnotationInfo struct {
    Manager     string
    Operation   string
    Subresource string
    Time        time.Time
}

// Current annotationFrom
func annotationFrom(entry managed.ManagedFieldsEntry) AnnotationInfo {
    return AnnotationInfo{
        Manager:     entry.Manager,
        Subresource: entry.Subresource,
        Time:        entry.Time,
    }
}

// Extended annotationFrom
func annotationFrom(entry managed.ManagedFieldsEntry) AnnotationInfo {
    return AnnotationInfo{
        Manager:     entry.Manager,
        Operation:   entry.Operation,
        Subresource: entry.Subresource,
        Time:        entry.Time,
    }
}
```

### Pattern 2: Extending Options with ShowOperation
**What:** Add `ShowOperation bool` to `annotate.Options` and thread it to `formatComment`
**Confidence:** HIGH -- follows existing pattern for `Above` and `Mtime` in Options

```go
// Current Options (annotate.go)
type Options struct {
    Above bool
    Now   time.Time
    Mtime MtimeMode
}

// Extended Options
type Options struct {
    Above         bool
    Now           time.Time
    Mtime         MtimeMode
    ShowOperation bool
}
```

### Pattern 3: Extending formatComment
**What:** Modify `formatComment` to append `, operation` inside parentheses when `showOperation` is true
**Confidence:** HIGH -- the format rules are fully specified in CONTEXT.md

Current `formatComment` signature: `func formatComment(info AnnotationInfo, now time.Time, mtime MtimeMode) string`

The function needs a new parameter for the show-operation flag. Two options:

**Option A (recommended): Pass showOperation as parameter**
```go
func formatComment(info AnnotationInfo, now time.Time, mtime MtimeMode, showOperation bool) string
```

**Option B: Pass full Options struct**
```go
func formatComment(info AnnotationInfo, opts Options) string
```

Option A is recommended because it keeps the function's dependencies explicit and minimal, matching the existing style of passing `mtime MtimeMode` rather than the full Options struct.

Format logic with showOperation=true:

| MtimeMode | showOperation=false (current) | showOperation=true (new) |
|-----------|-------------------------------|--------------------------|
| relative | `manager (5d ago)` | `manager (5d ago, apply)` |
| absolute | `manager (2026-02-07T10:30:00Z)` | `manager (2026-02-07T10:30:00Z, apply)` |
| hide | `manager` | `manager (apply)` |

With subresource, the base is `manager /sub` in all cases.

Key implementation detail: The operation value from Kubernetes is capitalized (`Apply`, `Update`). CONTEXT.md specifies lowercase (`apply`, `update`). Use `strings.ToLower(info.Operation)` for the conversion.

### Pattern 4: CLI Flag Wiring
**What:** Add `--show-operation` boolean flag to cobra command and pass to Options
**Confidence:** HIGH -- follows exact same pattern as `--above`

```go
// In main.go, add flag:
rootCmd.Flags().Bool("show-operation", false, "Include operation type (apply, update) in annotations")

// In RunE, read it:
showOperation, _ := cmd.Flags().GetBool("show-operation")

// Pass to Annotate:
annotate.Annotate(root, entries, annotate.Options{
    Above:         aboveMode,
    Now:           time.Now(),
    Mtime:         annotate.MtimeMode(mtimeFlagVar),
    ShowOperation: showOperation,
})
```

### Anti-Patterns to Avoid
- **Adding a new flag type**: `--show-operation` is a simple boolean, not a tri-state. Use cobra's `Bool` directly, not a custom pflag.Value type.
- **Modifying ManagedFieldsEntry**: The `Operation` field is already there. Do not add it again or change its type.
- **Changing existing comment format when flag is off**: When `ShowOperation` is false (default), output must be byte-identical to current output. All existing tests must continue to pass without modification.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Case conversion | Manual ASCII lowering | `strings.ToLower()` | Handles edge cases; Kubernetes operation values are ASCII but use standard library anyway |
| Boolean CLI flag | Custom pflag.Value | `cobra.Command.Flags().Bool()` | Boolean flags are built-in, no need for custom type |

**Key insight:** This phase is intentionally small. The main risk is over-engineering. Do not introduce new packages, interfaces, or abstractions for a single boolean flag and a string append.

## Common Pitfalls

### Pitfall 1: Breaking Existing Output When Flag is Off
**What goes wrong:** Adding the Operation field to AnnotationInfo or changing formatComment signature breaks existing behavior when `--show-operation` is not passed.
**Why it happens:** Forgetting to guard the operation display behind the flag check, or changing function signatures in a way that affects callers.
**How to avoid:** All existing unit tests and golden file tests must pass unchanged. The `showOperation` parameter defaults to false, and when false, `formatComment` must produce byte-identical output to the current implementation.
**Warning signs:** Any existing test failure after the change.

### Pitfall 2: Incorrect Lowercase Conversion
**What goes wrong:** Displaying `Apply` or `Update` instead of `apply` or `update`.
**Why it happens:** The `ManagedFieldsEntry.Operation` field stores the value as-is from Kubernetes YAML, which uses PascalCase (`Apply`, `Update`).
**How to avoid:** Always `strings.ToLower()` the operation value before formatting. The test fixture at `testdata/1_deployment.yaml` has `operation: Update` for all entries.
**Warning signs:** Test assertions comparing against lowercase fail because the source data is uppercase.

### Pitfall 3: CONTEXT.md References `--no-time` but Implementation Uses `--mtime hide`
**What goes wrong:** Creating a `--no-time` flag that does not exist in the codebase.
**Why it happens:** CONTEXT.md uses the original requirements language (`--no-time`), but Phase 3 consolidated this into `--mtime hide`.
**How to avoid:** The interaction "with `--no-time`" means "with `--mtime hide`". Format: `# manager (apply)` (just operation in parens, no time).
**Warning signs:** Looking for a `--no-time` flag in the code and not finding it.

### Pitfall 4: extractManagerName Breakage with New Format
**What goes wrong:** The color system fails to extract the correct manager name from the new format.
**Why it happens:** If the operation type format somehow changes the position of the ` (` delimiter that `extractManagerName` relies on.
**How to avoid:** Verify that `extractManagerName` already handles the new format correctly. Current logic (color.go:58-73): strips `# ` prefix, then finds first ` /` (subresource) or ` (` (timestamp). With operation in the format, the comment becomes `# manager (5d ago, apply)` -- the ` (` delimiter is in the same position, so manager extraction is unaffected.
**Warning signs:** Color tests failing with the new format.

### Pitfall 5: Hide Mode with Operation -- Missing Parentheses
**What goes wrong:** When `--mtime hide --show-operation`, output is `# manager apply` instead of `# manager (apply)`.
**Why it happens:** The hide mode currently returns just `base` (no parentheses). With operation enabled, it needs to add `(operation)`.
**How to avoid:** The format rule is: when `--show-operation` is on and `--mtime hide`, wrap the operation in parentheses: `(apply)`. This means the hide branch needs a sub-check for showOperation.

## Code Examples

### formatComment with ShowOperation Support

```go
func formatComment(info AnnotationInfo, now time.Time, mtime MtimeMode, showOperation bool) string {
    var base string
    if info.Subresource != "" {
        base = fmt.Sprintf("%s /%s", info.Manager, info.Subresource)
    } else {
        base = info.Manager
    }

    op := ""
    if showOperation && info.Operation != "" {
        op = strings.ToLower(info.Operation)
    }

    switch mtime {
    case MtimeAbsolute:
        ts := info.Time.UTC().Format(time.RFC3339)
        if op != "" {
            return fmt.Sprintf("%s (%s, %s)", base, ts, op)
        }
        return fmt.Sprintf("%s (%s)", base, ts)
    case MtimeHide:
        if op != "" {
            return fmt.Sprintf("%s (%s)", base, op)
        }
        return base
    default: // MtimeRelative
        age := timeutil.FormatRelativeTime(now, info.Time)
        if op != "" {
            return fmt.Sprintf("%s (%s, %s)", base, age, op)
        }
        return fmt.Sprintf("%s (%s)", base, age)
    }
}
```

### Expected Output Examples

```yaml
# --show-operation with default --mtime relative:
replicas: 3 # kubectl-client-side-apply (50m ago, update)

# --show-operation with --mtime absolute:
replicas: 3 # kubectl-client-side-apply (2024-04-10T00:44:50Z, update)

# --show-operation with --mtime hide:
replicas: 3 # kubectl-client-side-apply (update)

# --show-operation with subresource:
availableReplicas: 3 # kube-controller-manager /status (1h ago, update)

# --show-operation in above mode:
# kubectl-client-side-apply (50m ago, update)
replicas: 3

# Without --show-operation (unchanged from current):
replicas: 3 # kubectl-client-side-apply (50m ago)
```

### extractManagerName Verification

The existing `extractManagerName` function handles the new format correctly without modification:

```go
// Input: "# kubectl-client-side-apply (50m ago, update)"
// After stripping "# ": "kubectl-client-side-apply (50m ago, update)"
// First " /": not found
// First " (": index 25
// Result: "kubectl-client-side-apply" -- CORRECT

// Input: "# kube-controller-manager /status (1h ago, update)"
// After stripping "# ": "kube-controller-manager /status (1h ago, update)"
// First " /": index 23
// Result: "kube-controller-manager" -- CORRECT
```

No changes needed to `extractManagerName` or any output package code.

## Kubernetes Operation Type Reference

**Source:** github.com/kubernetes/apimachinery `pkg/apis/meta/v1/types.go`
**Confidence:** HIGH (verified from Kubernetes source)

Kubernetes `ManagedFieldsEntry.operation` has exactly two valid values:
- `Apply` -- server-side apply operations
- `Update` -- standard update/patch operations (also used for create)

These appear in YAML as PascalCase strings. Per CONTEXT.md, display them lowercase: `apply`, `update`.

The existing test fixture (`testdata/1_deployment.yaml`) has `operation: Update` for all four entries. For comprehensive testing, create fixtures or unit tests that exercise `operation: Apply` as well.

## Test Strategy Recommendations (Claude's Discretion)

### Unit Tests for formatComment
Add test cases for all combinations of `showOperation` x `MtimeMode`:

| Test Case | showOperation | MtimeMode | Expected |
|-----------|---------------|-----------|----------|
| relative + operation | true | relative | `mgr (5m ago, update)` |
| absolute + operation | true | absolute | `mgr (2026-02-07T12:00:00Z, apply)` |
| hide + operation | true | hide | `mgr (update)` |
| relative + no operation | false | relative | `mgr (5m ago)` (unchanged) |
| hide + no operation | false | hide | `mgr` (unchanged) |
| with subresource | true | relative | `mgr /status (1h ago, update)` |
| empty operation string | true | relative | `mgr (5m ago)` (graceful fallback) |

### Golden File Tests
Create new golden files for the `--show-operation` case using the existing deployment fixture:
- `1_deployment_inline_operation.out` -- inline mode with `--show-operation`
- `1_deployment_above_operation.out` -- above mode with `--show-operation`

These can reuse the existing `processDeploymentFixture` helper with a new parameter for ShowOperation.

### extractManagerName Tests
Add test cases for the new format to `TestExtractManagerName` in `color_test.go`:
- `"# manager (5d ago, apply)"` -> `"manager"`
- `"# manager /status (1h ago, update)"` -> `"manager"`
- `"# manager (apply)"` -> `"manager"` (hide mode + operation)

### End-to-End Test
Extend the pipeline test in `formatter_test.go` with input containing the new format to verify alignment and colorization work correctly with operation annotations.

## Open Questions

1. **Operation value when empty**
   - What we know: The `Operation` field in `ManagedFieldsEntry` is always populated in practice (`Apply` or `Update`). The Kubernetes API schema marks it as required.
   - What's unclear: Whether there are edge cases where operation could be empty in malformed YAML.
   - Recommendation: Treat empty operation as "don't show operation even if flag is on" (the code example above handles this with `if showOperation && info.Operation != ""` guard). This is defensive and costs nothing.

## Sources

### Primary (HIGH confidence)
- **Codebase analysis**: All Go source files in `internal/annotate/`, `internal/managed/`, `internal/output/`, `cmd/kubectl-fields/` read and analyzed
- **Kubernetes apimachinery source** (github.com/kubernetes/apimachinery `pkg/apis/meta/v1/types.go`): Confirmed `ManagedFieldsOperationType` has exactly two values: `Apply` and `Update`
- **Test fixtures**: `testdata/1_deployment.yaml` verified to contain `operation: Update` for all 4 managed fields entries

### Secondary (MEDIUM confidence)
- **Kubernetes API docs** (kubernetes.io/docs/reference/generated/kubernetes-api/v1.31): Confirmed ManagedFieldsEntry structure but did not list operation values explicitly

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependencies, all patterns verified from existing code
- Architecture: HIGH - all touched files read, data flow traced end-to-end, function signatures verified
- Pitfalls: HIGH - each identified by code analysis, verified with existing tests
- Test strategy: HIGH - follows existing patterns, all test file structures analyzed

**Research date:** 2026-02-07
**Valid until:** 2026-03-07 (stable -- no external dependency changes expected)
