# codegate

`codegate` is an agent-oriented code improvement engine for source-aware lookup, quality assessment, suggestions, explicit edits, validation, and reassessment.

The module is:

```text
github.com/codewandler/codegate
```

The core API is language-agnostic: callers work with symbols, ranges, occurrences, imports, call edges, proposals, and explicit change sets. Languages are registered explicitly through backend packages such as `language/golang` and `language/markdown`.

## Features

- `fs.FS`-backed workspaces with in-memory overlay commits.
- Direct source/workspace integration via `codegate.WithSource`.
- Public engine facade via `codegate.New()` for agent loops.
- Agent-readable `Lookup`, `Assess`, `Suggest`, `Validate`, and capability reporting APIs.
- Language-neutral assessment provider contract with architecture, maintainability, safety, and coverage gates.
- Cobra CLI skeleton in `cmd/codegate` for JSON-first agent/LLM skill usage.
- Agentruntime workspace integration helpers via `adapter/agentruntime`.
- No hidden disk writes, git commands, shell execution, or local path assumptions in core.
- Pluggable language backend contract via `codegate.Backend`.
- Built-in language backends for Go and Markdown.
- Optional validation through parse/typecheck-capable backends.
- Go AST backend for:
  - package discovery
  - outlines and symbol search
  - position-based navigation and symbol info
  - read/write/call/import/doc occurrence classification
  - references from selectors or source positions
  - direct and reverse imports
  - direct callers and callees
  - parse and opt-in typecheck validation
  - best-effort implementation matching
  - package pressure metrics
- Semantic Go edits through `ChangeSet`:
  - rename, replace, append, delete, and move symbols
  - replace or append functions and methods
  - replace doc comments
  - ensure, remove, or rename imports
  - reconcile imports while moving symbols between files
  - ensure or remove struct tags
  - add, remove, or rename Go parameters with direct call-site updates
  - add, remove, or rename Go struct fields, with reference guards for removals
  - change Go parameter and result types
  - rename Go method receivers
  - add or remove Go interface methods
  - extract Go functions or methods from explicit source ranges
- Unified diff preview before commit.
- Pending change-set validation before commit.
- Conservative safety guards for generated files, shadowing-prone parameter edits, ambiguous field selectors, and unsupported signature call sites.
- Refactoring proposals for simple AST-derived signals:
  - unused private symbols
  - large functions
  - large parameter lists
  - boolean flag parameters
  - high fan-in packages
  - executable operation payloads for safe suggestions; advisory evidence for suggestions that need user intent
- Goldmark-backed Markdown structure support for:
  - document and heading lookup
  - stable generated anchors
  - enclosing section navigation
  - parse/structure validation
  - quality findings for missing or repeated H1s, heading-level jumps, duplicate anchors, oversized or empty sections, and broken local heading links

## Position coordinates

`Position` and `PositionSelector` use 1-indexed line and column coordinates. For Go source, columns are byte columns, matching `go/token.Position.Column`, not Unicode rune columns. This keeps line/column navigation consistent with stored Go ranges and offsets when UTF-8 text appears before a target on the same line. Offset-based selectors continue to use zero-indexed byte offsets.

## Example

```go
package main

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/codewandler/codegate"
)

func main() {
	ctx := context.Background()

	fsys := fstest.MapFS{
		"main.go": &fstest.MapFile{Data: []byte(`package main

func hello() string {
	return "hello"
}
`)},
	}

	ed, err := codegate.NewEditor(".", codegate.WithFS(fsys), codegate.WithLanguage(codegate.Go))
	if err != nil {
		panic(err)
	}

	fragment, err := ed.ReadSymbol(ctx, codegate.SymbolSelector{
		Name: "hello",
		Kind: codegate.SymbolFunction,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(fragment.Source)

	changes := ed.NewChangeSet()
	err = changes.Apply(ctx, codegate.ReplaceFunction{
		Target: codegate.SymbolSelector{ID: fragment.Symbol.ID},
		Source: `func hello() string {
	return "hi"
}`,
	})
	if err != nil {
		panic(err)
	}

	diff, err := changes.Diff(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(diff)
}
```

## Engine Example

```go
import (
	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
)

engine, err := codegate.New().
	Roots(".").
	WithSource(source).
	WithLanguage(golang.New(golang.Config{})).
	Build(ctx)
if err != nil {
	panic(err)
}

report, err := engine.Assess(ctx, codegate.AssessmentOptions{
	Scope: codegate.Scope{Language: codegate.Go},
})
if err != nil {
	panic(err)
}

_ = report.Summary.Score
```

The lower-level `codegate.NewEditor` API remains available for direct source editing. The engine facade is the preferred public shape for agent workflows.

The CLI exposes the same assessment gates for agent skills:

```sh
go run ./cmd/codegate --root . assess --gate architecture --suggestions 3
go run ./cmd/codegate --root . --language markdown assess --gate maintainability
go run ./cmd/codegate --root . --language markdown lookup --name "Architecture" --kind namespace
```

## Agentruntime Integration

Use `adapter/agentruntime` to bridge an agentruntime workspace without adding an agentruntime dependency to core:

```go
source, err := agentruntime.NewWalkSource(
	func(ctx context.Context, path string, maxBytes int64) ([]byte, bool, error) {
		data, truncated, _, err := workspace.ReadFile(ctx, path, maxBytes)
		return data, truncated, err
	},
	func(ctx context.Context, root string, opts agentruntime.WalkOptions) ([]agentruntime.WalkEntry, bool, error) {
		entries, _, truncated, err := workspace.Walk(ctx, root, system.WalkOptions{
			Depth:      opts.Depth,
			ShowHidden: opts.ShowHidden,
			MaxEntries: opts.MaxEntries,
			FilesOnly:  opts.FilesOnly,
			SkipDirs:   opts.SkipDirs,
		})
		out := make([]agentruntime.WalkEntry, 0, len(entries))
		for _, entry := range entries {
			out = append(out, agentruntime.WalkEntry{Path: entry.Path.Rel, Kind: entry.Kind})
		}
		return out, truncated, err
	},
)
ed, err := codegate.NewEditor(".", codegate.WithSource(source), codegate.WithLanguage(codegate.Go))
```

## Architecture

The root package exposes the public facade and language-neutral model. The shared internal model and backend contract live in `internal/core`; language implementations live under `internal/lang/*`.

Core responsibilities:

- hold a logical workspace root
- read source from an explicit `fs.FS` or `codegate.Source`
- maintain in-memory overlays
- dispatch language operations to registered backends
- apply language-neutral text edits
- generate diffs and commit overlays

Adapters live outside core. They translate host-specific workspace APIs into `codegate.Source` without making codegate depend on those hosts.

Backend responsibilities:

- parse/index language files
- map declarations and uses into shared symbols, occurrences, edges, and imports
- compile supported semantic operations into text edits
- format changed files when supported
- report diagnostics, limitations, and resolution mode

The current backend split is deliberate: Go-specific AST and type packages stay under `internal/lang/goast`, while Markdown uses goldmark under `internal/lang/markdown`. A future Java, Groovy, or tree-sitter backend should only need to implement the same backend/provider interfaces and emit the shared model.

## Go Support Status

The Go backend is AST-only. It is useful for source navigation and deterministic edits without requiring local disk access or toolchain loading.

Supported today:

- package discovery from `.go` files
- declarations, references, direct calls, imports, and simple implementation edges
- AST-classified read, write, call, import, and doc occurrences
- source reads by symbol or source position
- deterministic edits for symbols, functions, methods, imports, comments, moves, and struct tags
- deterministic refactoring operations for Go signatures, parameters, receivers, struct fields, interface methods, and range-based function/method extraction
- optional import reconciliation when moving symbols between Go files
- package and symbol metrics plus AST-derived refactoring proposals
- executable refactor proposals for unused private symbols; advisory proposals for larger design-dependent refactors
- parse/typecheck validation and AST limitation summaries through `codegate.Validate` and `cmd/gocheck`
- agentruntime-style source integration through context-aware reads and workspace walks

Current limitations:

- no function-value call resolution
- no complete interface dispatch or dynamic call graph
- no typechecked package loading in core
- no module-aware import graph through `go list`
- no hidden execution of `go test`, `go build`, `go fmt`, or other toolchain commands

These limitations are intentional in core. Toolchain execution and disk persistence should be explicit adapters.

## Markdown Support Status

The Markdown backend is a proof that non-Go languages and non-code documents can use the same engine surface. It is goldmark-backed and structural rather than semantic.

Supported today:

- `.md` and `.markdown` discovery
- document and heading symbols
- anchor lookup by name, qualified name, or generated anchor
- position lookup with enclosing heading fallback
- structural validation
- assessment reports for document quality and agent-readable navigation risks

Current limitations:

- no executable Markdown edit or refactor operations yet
- no cross-file link graph
- no frontmatter or custom extension model yet
- no tree-sitter-style code block analysis

## Roadmap

Upcoming work:

1. Replace the existing agentruntime Go language plugin internals with calls into the engine facade.
2. Deepen Go architecture rules with explicit boundary configuration and stronger violation classification.
3. Add a tree-sitter-backed backend proof for another code language such as Java or Groovy.
4. Add adapter-backed type-aware Go analysis without making core depend on local disk paths.
5. Add validation adapters for explicit build/test workflows.
6. Turn more refactor suggestions into executable operations when type-aware or user-guided inputs make them deterministic.
7. Add Markdown edit/refactor operations for deterministic documentation fixes.

## Non-goals

- No hidden disk IO in core.
- No hidden git dependency in core.
- No mandatory LLM dependency.
- No opaque automatic rewrites without explicit operations.
- No assumption that Go is the only supported language.

## Design Notes

Design documents live in:

```text
.agents/designs/
```
