# editor

`editor` is a small Go library for source-aware navigation and controlled code edits.

The module is:

```text
github.com/codewandler/editor
```

The core API is language-agnostic: callers work with symbols, ranges, occurrences, imports, call edges, proposals, and explicit change sets. Go is the first backend, implemented under `internal/lang/goast`, and other languages can be added by registering another backend.

## Features

- `fs.FS`-backed workspaces with in-memory overlay commits.
- No hidden disk writes, git commands, shell execution, or local path assumptions in core.
- Pluggable language backend contract via `editor.Backend`.
- Go AST backend for:
  - package discovery
  - outlines and symbol search
  - position-based navigation and symbol info
  - references from selectors or source positions
  - direct and reverse imports
  - direct callers and callees
  - best-effort implementation matching
  - package pressure metrics
- Semantic Go edits through `ChangeSet`:
  - replace or append functions
  - delete symbols
  - replace doc comments
  - ensure or remove struct tags
- Unified diff preview before commit.
- Refactoring proposals for simple AST-derived signals:
  - unused private symbols
  - large functions
  - large parameter lists
  - boolean flag parameters
  - high fan-in packages

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

## Architecture

The root package exposes the public facade and language-neutral model. The shared internal model and backend contract live in `internal/core`; the Go implementation lives in `internal/lang/goast`.

Core responsibilities:

- hold a logical workspace root
- read source from an explicit `fs.FS`
- maintain in-memory overlays
- dispatch language operations to registered backends
- apply language-neutral text edits
- generate diffs and commit overlays

Backend responsibilities:

- parse/index language files
- map declarations and uses into shared symbols, occurrences, edges, and imports
- compile supported semantic operations into text edits
- format changed files when supported
- report diagnostics, limitations, and resolution mode

## Go Support Status

The Go backend is AST-only. It is useful for source navigation and deterministic edits, but it does not yet provide full `go/types` precision.

Current limitations:

- no external dependency resolution
- no build tag or cgo variant handling
- no precise method-set or interface dispatch semantics
- no function-value call resolution
- no module-aware import path resolution through `go list`
- no hidden execution of `go test`, `go build`, `go fmt`, or other toolchain commands

These limitations are intentional in core. Toolchain execution and disk persistence should be explicit adapters.

## Roadmap

Upcoming work:

1. Add optional type-aware Go backend support using `golang.org/x/tools/go/packages`.
2. Add an explicit OS filesystem adapter for durable commits outside core.
3. Add an agentruntime adapter so existing Go parser tools can become thin wrappers over `editor`.
4. Improve Go call/reference precision for selectors, methods, fields, and package-qualified symbols.
5. Add richer refactor operations such as rename symbol, update call sites, import rewrites, and move declarations.
6. Add validation adapters for parse/typecheck/build/test workflows.
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
