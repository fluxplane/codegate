# codegate

`codegate` gives agents and automation a structured way to understand and improve a codebase. It can look up code facts, assess quality, suggest improvements, apply explicit operations in memory, validate the result, and show a diff before anything is committed.

It is built for bots first, but the API is plain Go:

```text
github.com/codewandler/codegate
```

## Why Use It

- Agents get typed facts instead of scraping text.
- Review bots can produce repeatable quality reports with scores and evidence.
- Refactoring flows use explicit operations, not opaque rewrites.
- Changes stay in memory until callers inspect, validate, and commit them.
- Language support is explicit through registered backends.
- Core analysis does not run git, shells, tests, builds, or hidden disk writes.

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

	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
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

## Architecture Rules

Go assessment can take explicit architecture rules. At the simplest level, rules are prefix matched against the importing unit, package directory, or source path and the imported path. More specific rules win, so a narrow `allow` can override a broader `deny`.

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

See [`examples/agentruntime-architecture.rules.json`](examples/agentruntime-architecture.rules.json) for a larger policy modeled after a layered agent runtime. It is an example of consumer-defined layer names and rules, not a built-in codegate architecture.

## Markdown Support

Markdown is supported through a structural backend:

```go
engine, err := codegate.New().
	WithFS(fsys).
	WithLanguage(markdown.New(markdown.Config{})).
	Build(ctx)
```

It indexes documents, headings, anchors, and sections, then reports quality issues such as missing H1 titles, heading-level jumps, duplicate anchors, oversized sections, empty sections, and broken local heading links.

## CLI as an Agent Skill

The `cmd/codegate` CLI exposes the same engine loop as JSON-first commands:

```sh
go run ./cmd/codegate --root . --language go capabilities
go run ./cmd/codegate --root . --language go lookup --name Target --kind function
go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3
go run ./cmd/codegate --root . --language go assess --gate architecture --rules codegate.rules.json
go run ./cmd/codegate --root . --language go assess --gate architecture --rules codegate.rules.json --fail-on boundary,effects,unknown
go run ./cmd/codegate --root . --language go suggest --executable
go run ./cmd/codegate --root . --language go cycle
go run ./cmd/codegate --root . --language markdown assess --gate maintainability
```

`assess --fail-on` prints the normal JSON report first, then exits non-zero if a selected category has an unallowed violation. Supported categories are `boundary`, `test-boundary`, `effects`, `unknown`, and `all`.

Use `cycle --apply-first` only when you want the first executable suggestion applied to an in-memory change set and returned as a diff.

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

## Capabilities

Built-in backends:

- Go via `language/golang`
  - lookup and static analysis: advanced
  - editing: advanced for supported structured operations
  - refactoring: basic executable operations plus advisory suggestions
  - validation: parse and best-effort typecheck
- Markdown via `language/markdown`
  - lookup and static analysis: basic structural support
  - quality reporting: basic
  - editing and refactoring: not implemented yet
  - validation: parse/structure checks

## Safety Model

`codegate` is deliberately explicit:

- no hidden git operations
- no hidden shell commands
- no automatic test or build execution
- no persistent writes from core analysis
- no opaque rewrites without structured operations
- no commits until callers inspect and commit a change set

## Current Limitations

- Multiple workspace roots are rejected.
- Go analysis is AST-first with best-effort typecheck validation, not full package loading.
- Dynamic Go dispatch and function-value calls are incomplete.
- Markdown support is structural and read-only.
- CLI output is JSON-only.
- External validation adapters for build/test workflows are not implemented yet.
- Architecture policies are currently Go-only and AST-backed; tree-sitter-backed policy support for other languages is still upcoming.

## Release Readiness

Before tagging, run the checklist in [`RELEASE.md`](RELEASE.md). The normal test suite includes an external consumer test that imports only public packages and exercises Go lookup, Markdown assessment, validation, capabilities, and the agentruntime adapter.

## Roadmap

1. Replace the existing agentruntime Go language plugin internals with calls into the engine facade.
2. Add reusable architecture policy examples and adapters for common project shapes.
3. Add a tree-sitter-backed backend proof for another code language such as Java or Groovy.
4. Add adapter-backed type-aware Go analysis without making core depend on local disk paths.
5. Add validation adapters for explicit build/test workflows.
6. Turn more refactor suggestions into executable operations when type-aware or user-guided inputs make them deterministic.
7. Add Markdown edit/refactor operations for deterministic documentation fixes.
