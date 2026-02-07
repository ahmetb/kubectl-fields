# Technology Stack

**Project:** kubectl-fields
**Researched:** 2026-02-07
**Overall Confidence:** HIGH

## Recommended Stack

### Language & Runtime

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.23+ (target 1.23 minimum, develop on 1.25) | Language | The Kubernetes ecosystem is Go. kubectl plugins are overwhelmingly written in Go. Using Go aligns with user expectations, Krew distribution, and access to the Kubernetes client libraries if ever needed. Target 1.23 as the minimum to match the broader kubectl plugin ecosystem's compatibility floor while developing on the latest stable (1.25.7). | HIGH |

### YAML Processing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `go.yaml.in/yaml/v3` | v3.0.4 | YAML parsing, comment injection, output | **Primary recommendation.** This is the community-maintained fork of the original `go-yaml/yaml`, now maintained by the official YAML organization after the original author declared `gopkg.in/yaml.v3` unmaintained in April 2025. The `yaml.Node` type provides `HeadComment`, `LineComment`, and `FootComment` fields -- exactly what this project needs to inject manager annotations as YAML comments. Cobra itself uses this fork. API-compatible with the original `gopkg.in/yaml.v3` so all existing docs, tutorials, and StackOverflow answers apply. | HIGH |

**Critical Feature: `yaml.Node` comment fields**

```go
type Node struct {
    Kind        Kind
    Content     []*Node
    HeadComment string   // Comments in lines preceding the node
    LineComment string   // Comments at the end of the node's line
    FootComment string   // Comments following the node before empty lines
    Line        int
    Column      int
    // ... other fields
}
```

This maps directly to the project's two comment modes:
- `--above` mode: Set `HeadComment` on the field's key node
- Default (inline) mode: Set `LineComment` on the field's value node

### CLI Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `github.com/spf13/cobra` | v1.10.2 | Command structure, flag parsing, help generation | De facto standard for Go CLI tools, used by kubectl itself, GitHub CLI, Hugo, and ~200K dependent projects. Provides POSIX-compliant flags via pflag, automatic help/usage generation, shell completion generation, and man page generation. For a simple stdin-processing tool, cobra may feel like overkill -- but it costs nothing in complexity and gives shell completions and `--help` for free. | HIGH |
| `github.com/spf13/pflag` | v1.0.9+ | Flag parsing (transitive via cobra) | Drop-in replacement for Go's `flag` package with GNU-style `--long-flag` support. Pulled in automatically by cobra. Provides `BoolVar`, `StringVar`, etc. for clean flag definitions. | HIGH |

**Alternative considered: bare `pflag` without cobra**

For a single-command tool reading from stdin, using `pflag` directly (without cobra) would be sufficient. However:
- Cobra adds negligible overhead (~1 extra file of setup)
- Cobra gives you `--help` formatting, shell completions, and version subcommand for free
- If the tool ever grows subcommands (e.g., `kubectl fields diff`), cobra is already there
- **Recommendation: Use cobra.** The cost-benefit ratio strongly favors it.

### Terminal Color & TTY Detection

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `github.com/fatih/color` | v1.18.0 | Colorized output per manager name | Most widely used Go color library (7.9k stars, 638K dependents). Simple API: `color.New(color.FgCyan).SprintFunc()` returns a function you can wrap strings with. Automatically disables color when stdout is not a TTY (via `go-isatty`). Respects `NO_COLOR` environment variable. Supports RGB colors as of v1.18.0. Perfect for assigning distinct colors to different field managers. | HIGH |
| `github.com/mattn/go-isatty` | v0.0.20 | TTY detection (transitive via fatih/color) | Pulled in by `fatih/color` automatically. Cross-platform terminal detection (macOS, Linux, Windows, BSD, Solaris). Can also be used directly for custom TTY-aware logic if needed. | HIGH |

**Alternative considered: `muesli/termenv`**

`termenv` (v0.16.0) is more powerful -- it detects color profile (ANSI, 256, TrueColor), supports theme detection, and has advanced terminal features. However:
- It is heavier than needed for this project's requirements
- It still only has a `v0.x` version (pre-1.0 API stability)
- `fatih/color` is simpler, stable (v1.18), and does exactly what we need
- **Do NOT use termenv.** Overkill for colored YAML comments.

### Testing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go standard `testing` | (stdlib) | Test framework | Go's built-in testing is the standard. No framework needed for assertions when combined with testify. | HIGH |
| `github.com/stretchr/testify` | v1.11.1 | Assertions (`assert`, `require`) | Most popular Go test assertion library (638K dependents). `assert.Equal(t, expected, actual)` and `require.NoError(t, err)` reduce test boilerplate dramatically. Use `require` for fatal preconditions, `assert` for non-fatal checks. Do NOT use testify `suite` or `mock` packages -- they are unnecessary for a CLI tool. | HIGH |
| `gotest.tools/v3/golden` | v3.5.2 | Golden file testing | YAML-in/YAML-out tools are ideal for golden file testing. Store input YAML in `testdata/`, expected output in `testdata/*.golden`, compare byte-for-byte. Run `go test -update ./...` to regenerate golden files when behavior changes intentionally. Handles CRLF normalization cross-platform. This is the same library used by Docker's CLI testing. | HIGH |

**Testing Strategy:**

1. **Unit tests** with testify assertions for individual functions (managedFields parsing, comment generation, timestamp formatting)
2. **Golden file tests** for end-to-end YAML transformation (input YAML -> annotated output YAML)
3. **Table-driven tests** (Go idiom) for flag combinations and edge cases
4. **No mocks needed.** The tool processes YAML strings -- pure data transformation. Use real input/output, not mocks.

### Build & Distribution

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| GoReleaser | v2.13.3 | Cross-compilation, release artifacts, Homebrew/Krew manifests | Automates building for all platforms (linux/darwin/windows x amd64/arm64), generates checksums, creates GitHub releases, and produces Krew plugin manifests. Used by the majority of kubectl plugins in the Krew index. One `.goreleaser.yaml` config file replaces hundreds of lines of Makefile and CI scripts. | HIGH |
| Krew | - | kubectl plugin distribution | Krew is THE package manager for kubectl plugins (330+ plugins in the index). Users install with `kubectl krew install fields`. GoReleaser generates the Krew manifest automatically. | HIGH |
| Homebrew (via GoReleaser) | - | macOS/Linux distribution | Many users prefer `brew install` over Krew. GoReleaser can generate Homebrew tap formulas automatically. Optional but recommended for broader reach. | MEDIUM |

### Project Structure

```
kubectl-fields/
  cmd/
    kubectl-fields/
      main.go              # Entrypoint, cobra root command setup
  pkg/
    fields/
      parser.go            # managedFields FieldsV1 parsing (f:, k:, v: prefixes)
      parser_test.go
      annotator.go          # YAML node walking + comment injection
      annotator_test.go
      color.go              # Manager-to-color assignment, TTY detection
      color_test.go
      time.go               # Relative/absolute timestamp formatting
      time_test.go
  testdata/
    *.yaml                  # Input fixtures
    *.golden                # Expected output fixtures
  .goreleaser.yaml
  .krew.yaml
  go.mod
  go.sum
  LICENSE
  Makefile                  # Convenience targets: build, test, lint, install
```

## What NOT to Use

| Technology | Why Not |
|------------|---------|
| `gopkg.in/yaml.v3` | **Unmaintained since April 2025.** The original author explicitly labeled it unmaintained. Use `go.yaml.in/yaml/v3` (the official YAML org fork) instead. API-identical, just a different import path. |
| `goccy/go-yaml` (v1.19.2) | Powerful alternative with AST-level access and a `CommentMap` API. However: (1) the API is different from the standard go-yaml ecosystem, meaning less community documentation and examples; (2) the `yaml.Node` approach in `go.yaml.in/yaml/v3` is sufficient for our comment injection needs; (3) using the standard fork keeps alignment with cobra's own dependency. Only consider `goccy/go-yaml` if `yaml.Node` comment fields prove insufficient for complex nesting scenarios. |
| `muesli/termenv` | Pre-1.0 (v0.16.0), heavier than needed. `fatih/color` does everything this project requires with a stable API. |
| `encoding/json` for managedFields | FieldsV1 is JSON internally, but we need to correlate JSON paths to YAML node positions. Parse FieldsV1 as raw JSON with `encoding/json` from stdlib (no external library needed), then match paths to YAML nodes. |
| `k8s.io/apimachinery` | Heavyweight Kubernetes dependency just to parse managedFields structs. The FieldsV1 format is simple enough to parse directly: `f:fieldName`, `k:{"key":"value"}`, `v:value`. Avoid pulling in the entire Kubernetes API machinery. |
| `github.com/urfave/cli` | Older CLI framework, not used in the Kubernetes ecosystem. cobra/pflag is the standard. |
| `github.com/jessevdk/go-flags` | Not compatible with cobra. The Kubernetes ecosystem standardized on pflag. |
| testify `suite` / `mock` | Unnecessary complexity for a data-transformation CLI tool. Use `assert`/`require` only. |
| `github.com/sebdah/goldie/v2` | Another golden file library (v2.8.0). `gotest.tools/v3/golden` is preferred because it is simpler, maintained by the Docker team, and convention in the Kubernetes ecosystem. |

## `go.mod` Blueprint

```go
module github.com/rewanthtammana/kubectl-fields

go 1.23

require (
    github.com/spf13/cobra      v1.10.2
    go.yaml.in/yaml/v3           v3.0.4
    github.com/fatih/color       v1.18.0
    github.com/stretchr/testify  v1.11.1
    gotest.tools/v3              v3.5.2
)
```

Note: `github.com/spf13/pflag` and `github.com/mattn/go-isatty` will appear as indirect dependencies pulled in by cobra and fatih/color respectively.

## kubectl Plugin Conventions

### Naming
- Binary must be named `kubectl-fields`
- Invoked as `kubectl fields`
- Discovered via `$PATH` -- any directory in PATH works

### Distribution Channels (Priority Order)
1. **GitHub Releases** -- GoReleaser creates these automatically with cross-compiled binaries
2. **Krew** -- `kubectl krew install fields` (submit to `krew-index` after initial release)
3. **Homebrew** -- `brew install kubectl-fields` (via a tap, GoReleaser generates formula)
4. **Manual** -- `go install github.com/.../cmd/kubectl-fields@latest`

### stdin Convention
kubectl plugins that process YAML typically read from stdin via pipe:
```bash
kubectl get deployment nginx -o yaml | kubectl fields
```

This is the standard pattern. Do NOT add `--filename` flag -- let Unix pipes handle input.

## NO_COLOR Standard Compliance

The tool MUST respect these conventions (in priority order):
1. `--no-color` flag (explicit user flag, highest priority)
2. `NO_COLOR` environment variable (when set and non-empty, disable color)
3. TTY detection (when stdout is not a terminal, disable color automatically)

`fatih/color` handles items 2 and 3 automatically. Item 1 requires setting `color.NoColor = true` when the flag is present.

## Sources

- go.yaml.in/yaml/v3: https://pkg.go.dev/go.yaml.in/yaml/v3 (v3.0.4, published Jun 29, 2025) -- HIGH confidence
- yaml.Node type: https://pkg.go.dev/gopkg.in/yaml.v3#Node (identical API in go.yaml.in fork) -- HIGH confidence
- go-yaml unmaintained: https://github.com/go-yaml/yaml (author statement April 2025) -- HIGH confidence
- cobra: https://pkg.go.dev/github.com/spf13/cobra (v1.10.2, Dec 4, 2025) -- HIGH confidence
- pflag: https://pkg.go.dev/github.com/spf13/pflag (v1.0.10, Sep 2, 2025) -- HIGH confidence
- fatih/color: https://pkg.go.dev/github.com/fatih/color (v1.18.0, Oct 3, 2024) -- HIGH confidence
- go-isatty: https://pkg.go.dev/github.com/mattn/go-isatty (v0.0.20, Oct 17, 2023) -- HIGH confidence
- goccy/go-yaml: https://pkg.go.dev/github.com/goccy/go-yaml (v1.19.2, Jan 8, 2026) -- HIGH confidence (evaluated, not recommended)
- termenv: https://pkg.go.dev/github.com/muesli/termenv (v0.16.0, Feb 21, 2025) -- HIGH confidence (evaluated, not recommended)
- testify: https://pkg.go.dev/github.com/stretchr/testify (v1.11.1, Aug 27, 2025) -- HIGH confidence
- gotest.tools golden: https://pkg.go.dev/gotest.tools/v3/golden (v3.5.2, Sep 5, 2024) -- HIGH confidence
- goldie: https://pkg.go.dev/github.com/sebdah/goldie/v2 (v2.8.0, Oct 11, 2025) -- HIGH confidence (evaluated, not recommended)
- goreleaser: https://github.com/goreleaser/goreleaser (v2.13.3, Jan 10, 2026) -- HIGH confidence
- kubectl plugin conventions: https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/ -- HIGH confidence
- Krew developer guide: https://krew.sigs.k8s.io/docs/developer-guide/ -- HIGH confidence
- NO_COLOR standard: https://no-color.org/ -- HIGH confidence
- Go latest: https://go.dev/dl/ (1.25.7 latest stable, Feb 2026) -- HIGH confidence
