# editor

`editor` is a small Go library for source-aware navigation and controlled code edits.

The module is:

```text
github.com/codewandler/editor
```

The core API is language-agnostic: callers work with symbols, ranges, occurrences, imports, call edges, proposals, and explicit change sets. Go is the first backend, implemented under `internal/lang/goast`, and other languages can be added by registering another backend.

## Features

- `fs.FS`-backed workspaces with in-memory overlay commits.
- Direct source/workspace integration via `editor.WithSource`.
- Agentruntime workspace integration helpers via `adapter/agentruntime`.
- No hidden disk writes, git commands, shell execution, or local path assumptions in core.
- Pluggable language backend contract via `editor.Backend`.
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
- Conservative safety guards for generated files, shadowing-prone parameter edits, ambiguous field selectors, and unsupported signature call sites.
- Refactoring proposals for simple AST-derived signals:
  - unused private symbols
  - large functions
  - large parameter lists
  - boolean flag parameters
  - high fan-in packages
  - executable operation payloads for safe suggestions; advisory evidence for suggestions that need user intent

## Position coordinates

`Position` and `PositionSelector` use 1-indexed line and column coordinates. For Go source, columns are byte columns, matching `go/token.Position.Column`, not Unicode rune columns. This keeps line/column navigation consistent with stored Go ranges and offsets when UTF-8 text appears before a target on the same line. Offset-based selectors continue to use zero-indexed byte offsets.

## Example

```go
package main

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/codewandler/editor"
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

	ed, err := editor.New(".", editor.WithFS(fsys), editor.WithLanguage(editor.Go))
	if err != nil {
		panic(err)
	}

	fragment, err := ed.ReadSymbol(ctx, editor.SymbolSelector{
		Name: "hello",
		Kind: editor.SymbolFunction,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(fragment.Source)

	changes := ed.NewChangeSet()
	err = changes.Apply(ctx, editor.ReplaceFunction{
		Target: editor.SymbolSelector{ID: fragment.Symbol.ID},
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
ed, err := editor.New(".", editor.WithSource(source), editor.WithLanguage(editor.Go))
```

## Architecture

The root package exposes the public facade and language-neutral model. The shared internal model and backend contract live in `internal/core`; the Go implementation lives in `internal/lang/goast`.

Core responsibilities:

- hold a logical workspace root
- read source from an explicit `fs.FS` or `editor.Source`
- maintain in-memory overlays
- dispatch language operations to registered backends
- apply language-neutral text edits
- generate diffs and commit overlays

Adapters live outside core. They translate host-specific workspace APIs into `editor.Source` without making the editor depend on those hosts.

Backend responsibilities:

- parse/index language files
- map declarations and uses into shared symbols, occurrences, edges, and imports
- compile supported semantic operations into text edits
- format changed files when supported
- report diagnostics, limitations, and resolution mode

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
- parse/typecheck validation and AST limitation summaries through `editor.Validate` and `cmd/gocheck`
- agentruntime-style source integration through context-aware reads and workspace walks

Current limitations:

- no function-value call resolution
- no complete interface dispatch or dynamic call graph
- no typechecked package loading in core
- no module-aware import graph through `go list`
- no hidden execution of `go test`, `go build`, `go fmt`, or other toolchain commands

These limitations are intentional in core. Toolchain execution and disk persistence should be explicit adapters.

## Roadmap

Upcoming work:

1. Replace the existing agentruntime Go language plugin internals with calls into this library.
2. Add an explicit OS filesystem adapter for durable commits outside core.
3. Add adapter-backed type-aware Go analysis without making core depend on local disk paths.
4. Improve dynamic call limitations and type-aware selector classification.
5. Add validation adapters for explicit build/test workflows.
6. Turn more refactor suggestions into executable operations when type-aware or user-guided inputs make them deterministic.
7. Add another language backend, likely tree-sitter-backed, to prove the language-neutral model.

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
