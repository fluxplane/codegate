# codegate

![codegate Go quality](https://img.shields.io/badge/codegate%2Fgo-95%20A%2B-00ADD8)

`codegate` gives agents and automation a structured way to understand and improve a codebase. It can look up code facts, assess quality, suggest improvements, apply explicit operations in memory, validate the result, and show a diff before anything is committed.

It is built for bots first, but the API is plain Go:

```text
github.com/fluxplane/codegate
```

## Why Use It

- Agents get typed facts instead of scraping text.
- Review bots can produce repeatable quality reports with scores and evidence.
- Refactoring flows use explicit operations, not opaque rewrites.
- Changes stay in memory until callers inspect, validate, and commit them.
- Language support is explicit through registered backends.
- Core analysis does not run git, shells, tests, builds, or hidden disk writes. Go typecheck validation for disk-backed workspaces uses the Go package loader so dependency export data and generated-file context are resolved by the Go toolchain.

## What Bots Can Do

- `Lookup`: resolve symbols, source positions, Markdown headings, references, callers, and callees.
- `Assess`: produce JSON-friendly quality, architecture, safety, coverage, and maintainability reports.
- `Suggest`: list executable or advisory improvement work items.
- `Apply`: turn safe suggestions into pending in-memory edits.
- `Validate`: check pending changes before commit.
- `Diff`: show exactly what would change.

The intended loop is:

```text
lookup -> assess -> suggest -> apply -> validate -> reassess
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/fluxplane/codegate"
	"github.com/fluxplane/codegate/language/golang"
)

func main() {
	ctx := context.Background()

	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc Target() {}\n")},
		}).
		WithLanguage(golang.New(golang.Config{})).
		Build(ctx)
	if err != nil {
		panic(err)
	}

	result, err := engine.Lookup(ctx, codegate.LookupQuery{
		Name: "Target",
		Kind: codegate.SymbolFunction,
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Symbols[0].QualifiedName)
}
```

## Assess a Workspace

```go
report, err := engine.Assess(ctx, codegate.AssessmentOptions{
	Scope: codegate.Scope{Language: codegate.Go},
	Gates: []codegate.AssessmentGate{codegate.AssessmentGateAll},
})
if err != nil {
	panic(err)
}

fmt.Println(report.Summary.Score)
```

Reports include scores, findings, violations, diagnostics, top units, and suggestions. They are designed to be consumed by LLMs, CI bots, and review automation.

All component scores except `pressure` are capped at `100`. A score of `100` means codegate found no penalty for that gate in the selected scope; lower scores mean findings, violations, diagnostics, or policy failures applied deterministic penalties. `overall` is intentionally conservative: it follows the weakest active gate and is further reduced by hard violations. `pressure` is different: it is an unbounded prioritization signal based on dependency, call, public API, file, and implementation pressure, not a grade.

CLI reports also derive a human-readable rating from the numeric overall score: `A++` starts at `98`, `A` starts at `90`, `B` starts at `70`, `C` starts at `50`, `D` starts at `30`, and `F--` covers `0-4`. Use the numeric score for automation and the rating for quick review summaries.

For Go, maintainability and safety assessment includes deterministic AST-only quality signals such as cyclomatic complexity, nesting depth, function and file size, parameter and return counts, package/API shape, doc coverage, weak names, testability ratios, generated-code ratio, large structs, broad interfaces, ignored call results, unchecked type assertions, defer-in-loop, process exits, string concatenation in loops, unsafe/weak-crypto usage, dynamic process execution, composed SQL queries, dynamic file paths, reflection, obvious slice preallocation opportunities, and large range copies. These stay backend-local and are exposed through generic findings and aggregate metrics such as `max_cyclomatic_complexity`, `doc_coverage_percent`, `test_to_code_ratio`, `ignored_error_count`, `dynamic_exec_count`, and `missing_capacity_count`.

Go doc coverage metrics target exported symbols in public API packages outside implementation and command package trees. Implementation packages still contribute structural quality, safety, performance, and pressure signals, but their exported helper names are not treated as public documentation debt.

Maintainability assessment also counts source debt markers in comments and prose. `TODO`, `FIXME`, `HACK`, `XXX`, and `DEPRECATED` markers appear as `maintainability_debt_marker` findings, with aggregate `debt_marker_count` and `debt_marker_counts` metrics. These produce advisory review suggestions only; codegate does not remove or rewrite intent-bearing notes automatically.

Generated Go files are excluded by default. Pass `Scope.IncludeGenerated: true` in Go or `--generated` in the CLI when generated sources are owned by the repository and should contribute to the assessment.

## Architecture Rules

Go assessment can take explicit architecture rules. At the simplest level, rules are prefix matched against the importing unit, package directory, or source path and the imported path. More specific rules win, so a narrow `allow` can override a broader `deny`.

Without explicit architecture rules, architecture findings are advisory pressure signals: they can affect coupling but do not reduce the hard boundary score. Boundary, test-boundary, side-effect, and unknown-package failures become hard signals when callers provide architecture rules.

For larger projects, consumers define their own layers, allowed dependency directions, side-effect policies, coupling thresholds, and reasoned exceptions. `codegate` does not bake in concepts such as domain, adapter, runtime, or plugin; those names belong to the consuming project.

```go
report, err := engine.Assess(ctx, codegate.AssessmentOptions{
	Scope: codegate.Scope{Language: codegate.Go},
	Gates: []codegate.AssessmentGate{codegate.AssessmentGateArchitecture},
	Architecture: &codegate.ArchitectureRules{
		Layers: []codegate.ArchitectureLayer{
			{Name: "domain", Prefixes: []string{"domain"}},
			{Name: "infra", Prefixes: []string{"infra"}},
		},
		Dependencies: []codegate.ArchitectureDependencyRule{
			{FromLayer: "infra", ToLayer: "domain"},
			{FromLayer: "domain", ToLayer: "domain"},
		},
		Effects: []codegate.ArchitectureEffectRule{{
			Name:    "host_io",
			Scope:   codegate.ArchitectureScope{Layers: []string{"domain"}},
			Imports: []string{"os", "net/http"},
			Reason:  "domain code should not access host IO directly",
		}},
	},
})
```

The same policy can be passed to the CLI as JSON:

```json
{
  "layers": [
    {"name": "domain", "prefixes": ["domain"]},
    {"name": "infra", "prefixes": ["infra"]},
    {"name": "app", "prefixes": ["app"]}
  ],
  "dependencies": [
    {"from_layer": "domain", "to_layer": "domain"},
    {"from_layer": "app", "to_layer": "domain"},
    {"from_layer": "app", "to_layer": "infra"},
    {"from_layer": "infra", "to_layer": "domain"}
  ],
  "effects": [
    {
      "name": "host_io",
      "scope": {"layers": ["domain"]},
      "imports": ["os", "os/exec", "net/http", "database/sql"],
      "calls": [{"import": "os", "symbol": "Getenv"}],
      "reason": "domain code should use project-defined ports instead of host IO"
    }
  ],
  "coupling": {
    "fan_out_threshold": 12,
    "layers": ["domain"],
    "reviewed_fan_out": [
      {"package": "domain/catalog", "reason": "catalog intentionally aggregates domain registrations"}
    ]
  }
}
```

See [`examples/README.md`](examples/README.md) and [`examples/agentruntime-architecture.rules.json`](examples/agentruntime-architecture.rules.json) for a larger policy modeled after a layered agent runtime. It is an example of consumer-defined layer names and rules, not a built-in codegate architecture.

## Markdown Support

Markdown is supported through a structural backend:

```go
engine, err := codegate.New().
	WithFS(fsys).
	WithLanguage(markdown.New(markdown.Config{})).
	Build(ctx)
```

It indexes documents, headings, anchors, and sections, then reports quality issues such as missing H1 titles, heading-level jumps, duplicate anchors, oversized sections, empty sections, and broken local heading links.

Markdown also has a conservative fix loop. Missing H1 titles, heading-level jumps, empty sections, and safe duplicate-heading cases can produce executable operations. Broken local links stay advisory unless a future backend can infer the intended target unambiguously.

## CLI as an Agent Skill

The `cmd/codegate` CLI exposes the same engine loop as JSON-first commands:

```sh
go run ./cmd/codegate --root . --language go capabilities
go run ./cmd/codegate --root . --language go lookup --name Target --kind function
go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3
go run ./cmd/codegate --root . --language go assess --gate all --view full --suggestions 3
go run ./cmd/codegate --root . --language go assess --gate all --summary-only
go run ./cmd/codegate --root . --language go --format html assess --gate all > codegate-report.html
go run ./cmd/codegate --root . --language go --generated assess --gate all
go run ./cmd/codegate --root . --language go assess --gate architecture --rules codegate.rules.json
go run ./cmd/codegate --root . --language go assess --gate architecture --rules codegate.rules.json --fail-on boundary,effects,unknown
go run ./cmd/codegate --root . --language go suggest --executable
go run ./cmd/codegate --root . --language go op run --kind go_module_path_rename --from github.com/old/module --to github.com/new/module
go run ./cmd/codegate --root . --language go op run --operation-file operation.json --patch operation.patch
go run ./cmd/codegate --root . --language go cycle
go run ./cmd/codegate --root . --language markdown assess --gate maintainability
go run ./cmd/codegate --root . --language markdown cycle --apply-first
```

`assess --fail-on` prints the normal JSON report first, then exits non-zero if a selected category has an unallowed violation. Supported categories are `boundary`, `test-boundary`, `effects`, `unknown`, and `all`.

`assess` defaults to the compact agent view: scores, rating, compact metrics, counts, and small top lists without full evidence payloads. Use `--view full` for complete JSON reports and `--view summary` or `--summary-only` for the smallest score/count payload. Use `--format html` for a standalone browser report with expandable evidence blocks, CDN-backed syntax highlighting, and the full JSON report embedded behind a `Download JSON` link.

## Quality Badges

CI systems can turn the JSON report into a README badge. Badges should name the assessed language, because each language backend declares its own available metrics and findings. A simple static Go badge convention is:

```md
![codegate Go quality](https://img.shields.io/badge/codegate%2Fgo-82%20B%2B%2B-00ADD8)
```

Generate the values from `summary.score` and the CLI `rating` field in `codegate assess --format json`. The current README badge for this repository was generated from a Go assessment: `95 A+`.

For Go-specific badges, use the Go brand color `00ADD8`. If you prefer score-severity colors for dashboards, use:

| Score | Color |
| --- | --- |
| `90-100` | `brightgreen` |
| `80-89` | `green` |
| `70-79` | `yellowgreen` |
| `50-69` | `yellow` |
| `30-49` | `orange` |
| `0-29` | `red` |

Bot update recipe:

```sh
go run ./cmd/codegate --root . --language go --format json assess --gate all > /tmp/codegate-assessment.json
score="$(jq -r '.summary.score' /tmp/codegate-assessment.json)"
rating="$(jq -r '.rating' /tmp/codegate-assessment.json)"
label="$(printf '%s %s' "$score" "$rating" | jq -sRr @uri)"
printf '![codegate Go quality](https://img.shields.io/badge/codegate%%2Fgo-%s-00ADD8)\n' "$label"
```

Keep the HTML report as the human artifact and the JSON report as the bot artifact.

Use `cycle --apply-first` only when you want the first executable suggestion applied to an in-memory change set and returned as a diff.

Use `op run` when an agent already knows the exact structured edit operation to execute. It defaults to dry-run JSON with validation and a unified diff. Add `--patch file.patch` to persist the diff, or `--write` to write validated changes back to the workspace.

## Agent Loop Examples

Compile-tested examples in [`example_test.go`](example_test.go) cover the public engine builder, a Go lookup-assess-suggest-apply-validate-diff loop, Markdown structural cleanup, the direct `Editor` compatibility API, and the `adapter/agentruntime` source bridge.

## Direct Edit API

Most bot workflows should use `codegate.New()`. The lower-level `codegate.NewEditor()` API remains available when you need direct control over symbol reads and change sets:

```go
editor, err := codegate.NewEditor(".", codegate.WithFS(fsys), codegate.WithLanguage(codegate.Go))
if err != nil {
	panic(err)
}

fragment, err := editor.ReadSymbol(ctx, codegate.SymbolSelector{
	Name: "hello",
	Kind: codegate.SymbolFunction,
})
if err != nil {
	panic(err)
}

changes := editor.NewChangeSet()
err = changes.Apply(ctx, codegate.ReplaceFunction{
	Target: codegate.SymbolSelector{ID: fragment.Symbol.ID},
	Source: "func hello() string { return \"hi\" }",
})
```

## Agentruntime Integration

Use `adapter/agentruntime` to bridge workspace read and walk functions without adding an agentruntime dependency to core:

```go
source, err := agentruntime.NewWalkSource(readFile, walkFiles)
if err != nil {
	panic(err)
}

engine, err := codegate.New().
	WithSource(source).
	WithLanguage(golang.New(golang.Config{})).
	Build(ctx)
```

## Explicit Validation Adapters

Core validation remains parse/typecheck or structural only. For disk-backed Go workspaces, typecheck validation uses the Go package loader; in-memory sources and pending overlays fall back to AST-only type checking. If a host application wants build, test, or policy checks, it can register a named validation adapter and explicitly request it:

```go
adapter := codegate.NewValidationAdapter("unit", func(ctx context.Context, source codegate.Source, opts codegate.ValidationOptions) (codegate.ValidationResult, error) {
	// The host owns any build/test command execution and maps results back into codegate diagnostics.
	return codegate.ValidationResult{Passed: true, Complete: true, Kinds: []codegate.ValidationKind{codegate.ValidationExternal}}, nil
})

engine, err := codegate.New().
	WithFS(fsys).
	WithLanguage(golang.New(golang.Config{})).
	WithValidationAdapter(adapter).
	Build(ctx)
```

Adapters only run when named in `ValidationOptions.External`. This keeps shell commands, tests, builds, and workspace-specific policy checks caller-controlled and auditable.

## Capabilities

`Capabilities()` and `codegate capabilities` report both coarse backend capability levels and operation-level support. Agents can use this to choose safe calls without guessing which language supports which validation modes or edit operations.

Capability output also lists exact assessment support per language: gates, metric IDs, finding kinds, and violation kinds. Metric concepts are generic public strings, but support is declared per backend. For example, Go currently reports metrics such as `max_cyclomatic_complexity`, `ignored_error_count`, and `dynamic_exec_count`, while Markdown reports structural documentation signals and debt marker counts.

Built-in backends:

- Go via `language/golang`
  - lookup and static analysis: advanced
  - editing: advanced for supported structured operations
  - refactoring: basic executable operations plus advisory suggestions
  - validation: parse and package-loader typecheck for disk workspaces, AST-only fallback for in-memory sources
  - operation detail includes symbol/position lookup, architecture gates, typecheck validation, Go edit operations, debt-marker review, and Go refactor kinds
- Markdown via `language/markdown`
  - lookup and static analysis: basic structural support
  - quality reporting: basic
  - editing and refactoring: basic deterministic structural fixes
  - validation: parse/structure checks
  - operation detail includes document/heading/anchor lookup, maintainability/safety/coverage gates, parse validation, Markdown structure edit operations, and debt-marker review

## Safety Model

`codegate` is deliberately explicit:

- no hidden git operations
- no hidden shell commands
- no test or build command execution; Go typecheck validation may use the Go package loader
- no automatic test or build execution
- no persistent writes from core analysis
- CLI writes require explicit `op run --write`
- no opaque rewrites without structured operations
- no commits until callers inspect and commit a change set

## Current Limitations

- Multiple workspace roots are rejected.
- Go analysis is AST-first for indexing and quality metrics; typecheck validation uses package loading only for disk-backed workspaces.
- Dynamic Go dispatch and function-value calls are incomplete.
- Markdown support is structural; only conservative documentation fixes are executable.
- CLI output is JSON-first; `assess` can also render a standalone HTML report.
- External validation adapters are explicit caller-provided hooks; codegate does not ship shelling adapters by default.
- Architecture policies are currently Go-only and AST-backed; tree-sitter-backed policy support for other languages is still upcoming.

## Release Readiness

Before tagging, run the checklist in [`RELEASE.md`](RELEASE.md). The normal test suite includes an external consumer test that imports only public packages and exercises Go lookup, Markdown assessment, validation, capabilities, and the agentruntime adapter.

## Roadmap

1. Replace the existing agentruntime Go language plugin internals with calls into the engine facade.
2. Add reusable architecture policy examples and adapters for common project shapes.
3. Add a tree-sitter-backed backend proof for another code language such as Java or Groovy.
4. Expand type-aware Go facts beyond validation while preserving in-memory source support.
5. Add optional prebuilt validation adapter examples for explicit build/test workflows.
6. Turn more refactor suggestions into executable operations when type-aware or user-guided inputs make them deterministic.
7. Expand Markdown edit/refactor coverage for deterministic documentation fixes.
8. Promote backend-local assessment signals into public `quality`, `security`, `performance`, and `testability` gates once Go plus at least one non-Go backend can produce comparable findings and metrics.
