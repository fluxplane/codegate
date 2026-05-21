# Language-Agnostic Editor Model and Go AST Backend

## 1. Context

This repository, `github.com/codewandler/codegate`, should become a small embeddable editing and refactoring library.

The first concrete use case is Go source editing. Later, the same model should support additional languages, likely through tree-sitter-backed parsers or language-specific AST/type backends.

The library is intended to be embedded in `~/projects/fluxplane/agentruntime` later. The existing agentruntime Go language plugin already provides many read-side operations that we can learn from:

- `go_packages`
- `go_outline`
- `go_symbol`
- `go_definition`
- `go_symbol_info`
- `go_references`
- `go_imports`
- `go_implementations`
- `go_callers`
- `go_callees`

Those operations are currently exposed as agentruntime tools and mostly use parser/AST-only Go analysis. They provide a useful prototype for the model, but `editor` should extract a reusable, IO-free library core with language-agnostic concepts and explicit edit/change-set semantics.

## 2. What exists in agentruntime today

The current Go plugin has two relevant model layers.

### 2.1 Shared language models

`agentruntime/core/language/language.go` defines generic concepts such as:

- `LanguageID`
- `ProviderSpec`
- `Capability`
- `Position`
- `Range`
- `Location`
- `Package`
- `Document`
- `Outline`
- `Diagnostic`
- `Symbol`
- `SymbolKind`
- `Import`
- `Reference`

This is already close to the language-agnostic model we need. Important existing fields:

```go
type Symbol struct {
    ID             string
    Language       LanguageID
    Kind           SymbolKind
    Name           string
    Container      string
    PackageID      string
    Location       Location
    Range          Range
    SelectionRange Range
    Signature      string
    Doc            string
    Children       []Symbol
}
```

The existing symbol kinds are also broadly reusable:

- module
- package
- type
- struct
- interface
- function
- method
- field
- const
- var
- import
- namespace

### 2.2 Go-specific query models

`agentruntime/core/language/golang/golang.go` adds Go tool/query contracts around the generic models:

- `NavigationQuery` / `NavigationResult`
- `ReferenceQuery` / `ReferenceResult`
- `ImportQuery` / `ImportResult`
- `ImplementationQuery` / `ImplementationResult`
- `CallQuery` / `CallResult`
- `OutlineQuery` / `OutlineResult`
- `SymbolQuery` / `SymbolResult`

The Go plugin implementation maps these queries to AST scans:

- parses Go files with `go/parser`
- builds outlines from `*ast.FuncDecl` and `*ast.GenDecl`
- records symbols with location, selection range, signature, docs, and children
- resolves local/package-level definitions best-effort
- scans references in package/file scope
- scans imports and reverse imports
- scans call edges for direct calls
- estimates implementation relationships for interfaces and concrete types

The current plugin is intentionally transparent about limitations. Many operations report warnings such as AST-only resolution with no full type checking, no external dependency resolution, no cgo/build-tag semantics, no interface dispatch, and no function-value dispatch.

That limitation model is useful and should remain explicit in `editor`.

## 3. Target stories

The library should support read-side navigation stories:

- Tell me where interface `X` is defined.
- Tell me where interface `X` is used.
- Tell me which concrete types implement interface `X`.
- Find symbol `X` in this workspace/package/file.
- Read function `F` without line-number addressing.
- Read method `T.M` without line-number addressing.
- Show all callers of `F`.
- Show all callees of `F`.
- Show direct and reverse package imports.
- Find high-pressure packages.
- Compute fan-in/fan-out counts.
- Identify symbols with high reference/call pressure.

And write-side/refactoring stories:

- Rename symbol `X` to `Y`.
- Replace function `F`.
- Replace method `T.M`.
- Append a new function to a file or package.
- Delete function `F`.
- Delete method `T.M`.
- Delete symbol `X`.
- Edit or replace the doc comment for a symbol.
- Ensure/remove/update Go struct tags.
- Build a change set, show diff, then commit.

The critical LLM-facing goal is that callers should not need to calculate line numbers. They should express intent in source terms:

```text
op=function_edit target=CreateUser
op=method_edit target=*Store.Get
op=comment_edit target=UserService
op=go_struct_tag_ensure target=User.Email tag=json value=email
op=rename target=UserService new_name=AccountService
```

The library resolves those requests to precise source ranges and edit operations.

## 4. Core architectural constraints

### 4.1 No direct IO in core

The core `editor` package must not directly use:

- `os.Open`
- `os.ReadFile`
- `os.WriteFile`
- `exec.Command`
- git commands
- local path assumptions beyond logical slash-separated workspace paths

The core should operate on a provided workspace abstraction, with `fs.FS` as the minimum read-side base.

### 4.2 Explicit write backend

`fs.FS` is read-only. Editing requires a separate write/commit abstraction.

Core should distinguish:

- source snapshot reads
- in-memory overlay writes
- diff generation
- explicit commit

Disk writes, git commits, agentruntime workspace writes, and remote workspace persistence are adapter concerns.

### 4.3 Language backends are pluggable

Go is the first backend. Additional backends should implement the same language-neutral interfaces.

A backend may be:

- parser-only
- AST-aware
- type-aware
- tree-sitter-backed
- LSP-backed
- hybrid

Every result should declare its `ResolutionMode` and `Complete` status.

## 5. Proposed package layout

```text
editor/                  root facade package
model/                   public language-neutral model types
query/                   public query/filter types, if separated
edit/                    public edit operation and diff model
refactor/                public refactor proposal model
internal/workspace/      fs.FS snapshot, overlay, commit plumbing
internal/textedit/       range edits, conflict detection, diff generation
internal/graph/          symbol/import/call/reference graph helpers
internal/lang/goast/     Go parser/AST backend
adapter/osfs/            explicit local filesystem adapter
adapter/agentruntime/    future agentruntime workspace adapter
```

The root facade remains simple:

```go
ed, err := editor.New(path, editor.WithFS(fsys), editor.WithLanguage(editor.Go))
```

## 6. Language-agnostic model

### 6.1 Workspace and documents

```go
type LanguageID string

type Workspace struct {
    Root      string
    Languages []LanguageID
}

type Document struct {
    URI      string // logical workspace path, not necessarily OS path
    Language LanguageID
    UnitID   string // package/module/namespace id, if known
    Version  string // optional snapshot/overlay version
}
```

Use `URI` or `Path` consistently. Internally, paths should be clean slash-separated workspace-relative paths.

### 6.2 Source positions and ranges

The model should keep both machine-stable offsets and display-friendly line/column positions.

```go
type Position struct {
    Line   int // 1-indexed, display-oriented
    Column int // 1-indexed, byte column unless documented otherwise
    Offset int // 0-indexed byte offset; optional when unknown
}

type Range struct {
    Start Position
    End   Position
}

type Location struct {
    URI   string
    Range Range
}
```

Line/column are useful for humans and diffs. Offsets are useful for precise edit application. A backend should preserve offsets whenever possible.

### 6.3 Symbols

A symbol is a declared language entity.

```go
type SymbolID string

type Symbol struct {
    ID             SymbolID
    Language       LanguageID
    Kind           SymbolKind
    Name           string
    QualifiedName  string
    ContainerID    SymbolID
    ContainerName  string
    UnitID         string
    Location       Location // whole declaration
    SelectionRange Range    // identifier/name range
    BodyRange      Range    // function/method/type body when meaningful
    Signature      string
    Doc            string
    Tags           map[string]string
    Children       []Symbol
    Backend        BackendInfo
}
```

`QualifiedName` should be the primary LLM-friendly address, for example:

- `CreateUser`
- `Store.Get`
- `*Store.Get`
- `User.Email`
- `github.com/acme/app/internal/user.Service`

The exact representation can vary by language, but each backend should produce stable, human-readable qualified names.

### 6.4 Symbol kinds

The base language-neutral set should cover common programming languages:

```go
type SymbolKind string

const (
    SymbolModule     SymbolKind = "module"
    SymbolPackage    SymbolKind = "package"
    SymbolNamespace  SymbolKind = "namespace"
    SymbolFile       SymbolKind = "file"
    SymbolType       SymbolKind = "type"
    SymbolClass      SymbolKind = "class"
    SymbolStruct     SymbolKind = "struct"
    SymbolInterface  SymbolKind = "interface"
    SymbolEnum       SymbolKind = "enum"
    SymbolFunction   SymbolKind = "function"
    SymbolMethod     SymbolKind = "method"
    SymbolConstructor SymbolKind = "constructor"
    SymbolField      SymbolKind = "field"
    SymbolProperty   SymbolKind = "property"
    SymbolConst      SymbolKind = "const"
    SymbolVar        SymbolKind = "var"
    SymbolImport     SymbolKind = "import"
    SymbolParameter  SymbolKind = "parameter"
    SymbolLocal      SymbolKind = "local"
)
```

Backends may attach language-specific details in `Tags` or a typed extension map, but the core should prefer shared fields.

### 6.5 Occurrences and references

References should distinguish declaration, definition, read, write, call, import, implementation, and documentation occurrences.

```go
type OccurrenceKind string

const (
    OccurrenceDeclaration OccurrenceKind = "declaration"
    OccurrenceDefinition  OccurrenceKind = "definition"
    OccurrenceReference   OccurrenceKind = "reference"
    OccurrenceRead        OccurrenceKind = "read"
    OccurrenceWrite       OccurrenceKind = "write"
    OccurrenceCall        OccurrenceKind = "call"
    OccurrenceImport      OccurrenceKind = "import"
    OccurrenceImplement   OccurrenceKind = "implement"
    OccurrenceDoc         OccurrenceKind = "doc"
)

type Occurrence struct {
    SymbolID SymbolID
    Kind     OccurrenceKind
    Name     string
    Location Location
    Preview  string
    Evidence []Evidence
}
```

The existing agentruntime `Reference` maps to this directly, with a richer `Kind` enum.

### 6.6 Relationships / graph edges

A language index should be queryable as a graph. This supports fan-in/fan-out and high-pressure package analysis.

```go
type EdgeKind string

const (
    EdgeContains    EdgeKind = "contains"
    EdgeDeclares    EdgeKind = "declares"
    EdgeReferences  EdgeKind = "references"
    EdgeCalls       EdgeKind = "calls"
    EdgeImports     EdgeKind = "imports"
    EdgeImplements  EdgeKind = "implements"
    EdgeOverrides   EdgeKind = "overrides"
    EdgeEmbeds      EdgeKind = "embeds"
)

type Edge struct {
    Kind     EdgeKind
    From     string // SymbolID, UnitID, or Document URI
    To       string
    Location Location
    Weight   int
    Evidence []Evidence
}
```

## 7. Query model

Queries should support both precise editor-style positions and LLM-friendly selectors.

### 7.1 Symbol selector

```go
type SymbolSelector struct {
    Language      LanguageID
    Name          string
    QualifiedName string
    Kind          SymbolKind
    Container     string
    UnitID        string
    Path          string
    IncludeTests  *bool
}
```

Selectors may be ambiguous. The API must return ambiguity instead of guessing silently.

```go
type ResolveResult struct {
    Matches     []Symbol
    Ambiguous   bool
    Diagnostics []Diagnostic
}
```

### 7.2 Position selector

```go
type PositionSelector struct {
    Path   string
    Line   int
    Column int
    Offset *int
}
```

Position selectors remain useful for IDE-style tools, but should not be required for high-level edit operations.

### 7.3 Scope

```go
type Scope struct {
    Root       string
    Path       string
    UnitID     string
    Language   LanguageID
    IncludeTests bool
    MaxFiles   int
    MaxBytes   int64
}
```

## 8. Public read API shape

```go
type Editor struct { /* hidden */ }

func New(root string, opts ...Option) (*Editor, error)

func (e *Editor) Outline(ctx context.Context, scope Scope) (Outline, error)
func (e *Editor) FindSymbols(ctx context.Context, sel SymbolSelector) ([]Symbol, error)
func (e *Editor) Definition(ctx context.Context, sel SymbolSelector) (ResolveResult, error)
func (e *Editor) DefinitionAt(ctx context.Context, pos PositionSelector) (ResolveResult, error)
func (e *Editor) References(ctx context.Context, sel SymbolSelector) ([]Occurrence, error)
func (e *Editor) Implementations(ctx context.Context, sel SymbolSelector) ([]Implementation, error)
func (e *Editor) Callers(ctx context.Context, sel SymbolSelector) ([]CallEdge, error)
func (e *Editor) Callees(ctx context.Context, sel SymbolSelector) ([]CallEdge, error)
func (e *Editor) Imports(ctx context.Context, scope Scope) ([]ImportEdge, error)
func (e *Editor) Metrics(ctx context.Context, scope Scope) (Metrics, error)
```

`SuggestRefactorings` remains the higher-level proposal API:

```go
func (e *Editor) SuggestRefactorings(ctx context.Context, opts ...SuggestOption) ([]Proposal, error)
```

## 9. Metrics model for high-pressure packages

A high-pressure package is a package or unit with high structural importance or high change risk.

Suggested package metrics:

```go
type UnitMetrics struct {
    UnitID           string
    DirectFanIn      int // reverse imports
    DirectFanOut     int // imports
    SymbolFanIn      int // references/calls into unit symbols
    SymbolFanOut     int // references/calls out of unit symbols
    CallFanIn        int
    CallFanOut       int
    InterfaceCount   int
    ImplementationCount int
    PublicSymbolCount int
    FileCount        int
    LOC              int
    PressureScore    float64
    Evidence         []Evidence
}
```

Initial scoring can be simple and explainable:

```text
PressureScore =
  3.0 * normalized(reverse_import_count) +
  2.0 * normalized(call_fan_in) +
  1.5 * normalized(public_symbol_count) +
  1.0 * normalized(file_count) +
  1.0 * normalized(interface_implementation_edges)
```

The important part is not the exact formula; it is that evidence is returned so agents can explain why a package is high-pressure.

## 10. Edit model

### 10.1 Low-level text edits

All high-level operations compile to deterministic low-level edits.

```go
type TextEdit struct {
    Path        string
    Range       Range
    Replacement string
}

type FileEdit struct {
    Path string
    Edits []TextEdit
}
```

The edit engine must detect overlapping edits and stale ranges.

### 10.2 High-level semantic operations

LLM-facing operations should be semantic and selector-based.

```go
type Operation interface {
    Kind() OperationKind
}

type OperationKind string

const (
    OpRenameSymbol       OperationKind = "rename_symbol"
    OpReplaceSymbol      OperationKind = "replace_symbol"
    OpDeleteSymbol       OperationKind = "delete_symbol"
    OpReadSymbol         OperationKind = "read_symbol"
    OpAppendSymbol       OperationKind = "append_symbol"
    OpReplaceFunction    OperationKind = "replace_function"
    OpAppendFunction     OperationKind = "append_function"
    OpDeleteFunction     OperationKind = "delete_function"
    OpReplaceMethod      OperationKind = "replace_method"
    OpDeleteMethod       OperationKind = "delete_method"
    OpReplaceComment     OperationKind = "replace_comment"
    OpEnsureStructTag    OperationKind = "go_struct_tag_ensure"
    OpRemoveStructTag    OperationKind = "go_struct_tag_remove"
)
```

Example operation payloads:

```go
type ReplaceFunction struct {
    Target SymbolSelector
    Source string // complete function declaration
}

type AppendFunction struct {
    Path   string // or UnitID
    Source string
}

type ReplaceComment struct {
    Target SymbolSelector
    Text   string
    Style  string // doc, line, block; backend may default
}

type EnsureGoStructTag struct {
    Struct SymbolSelector
    Field  string
    Key    string // json, yaml, db, etc.
    Value  string
    Options []string // omitempty, inline, etc.
}
```

These operations are language-aware, but still compile to language-neutral text edits.

## 11. ChangeSet API

The editing flow should remain explicit:

```go
changes := ed.NewChangeSet()

err := changes.Apply(ctx, editor.ReplaceFunction{
    Target: editor.SymbolSelector{Name: "CreateUser", Kind: editor.SymbolFunction},
    Source: `func CreateUser(ctx context.Context, input CreateUserInput) (*User, error) {
        // ...
    }`,
})

diff, err := changes.Diff(ctx)
err = changes.Commit(ctx)
```

A `ChangeSet` should:

- resolve semantic operations against a snapshot
- produce text edits
- apply edits to an overlay
- validate conflicts
- optionally format affected files through backend hooks
- generate unified diff
- commit only when requested

```go
type ChangeSet interface {
    Apply(ctx context.Context, ops ...Operation) error
    Read(ctx context.Context, sel SymbolSelector) (SourceFragment, error)
    Diff(ctx context.Context) (string, error)
    Files(ctx context.Context) ([]ChangedFile, error)
    Commit(ctx context.Context) error
    Discard() error
}
```

`Read` on a change set should read from the overlay, not only the base snapshot.

## 12. Source fragments

Semantic read operations should return source fragments with enough metadata for safe follow-up writes.

```go
type SourceFragment struct {
    Symbol   Symbol
    Source   string
    Comments string
    Imports  []Import
    Hash     string // optional content hash for optimistic concurrency
}
```

This enables tool flows such as:

1. read function by name
2. LLM rewrites function body
3. replace function with stale-hash check
4. show diff
5. commit

## 13. Refactor proposals

`SuggestRefactorings` should return proposals, not patches.

```go
type Proposal struct {
    ID          string
    Kind        RefactorKind
    Title       string
    Summary     string
    Confidence  Confidence
    Risk        RiskLevel
    Targets     []Symbol
    Evidence    []Evidence
    Operations  []Operation
    Metrics     map[string]float64
}
```

Examples:

- high fan-in package: suggest interface extraction or dependency inversion
- repeated parameter group: suggest struct extraction
- unused private symbol: suggest delete symbol
- duplicated function shape: suggest extract function
- narrow interface usage: suggest split interface

The caller chooses whether to apply the proposal operations.

## 14. Backend interface

Language backends translate source files into symbols/edges and semantic operations into text edits.

```go
type Backend interface {
    Spec() BackendSpec
    Index(ctx context.Context, ws Snapshot, scope Scope) (*Index, error)
    Resolve(ctx context.Context, idx *Index, sel SymbolSelector) (ResolveResult, error)
    CompileEdit(ctx context.Context, idx *Index, op Operation) ([]FileEdit, error)
    Format(ctx context.Context, files map[string][]byte) (map[string][]byte, error)
}

type BackendSpec struct {
    Language       LanguageID
    Name           string
    Capabilities   []Capability
    ResolutionMode string // ast, typecheck, treesitter, lsp, hybrid
}
```

The `Index` should be language-neutral:

```go
type Index struct {
    Documents   []Document
    Units       []Unit
    Symbols     []Symbol
    Occurrences []Occurrence
    Edges       []Edge
    Diagnostics []Diagnostic
    Complete    bool
    Backend     BackendInfo
}
```

## 15. Go AST backend plan

The first backend should be `internal/lang/goast`.

It can reuse the logic proven in agentruntime:

- parse files with `go/parser.ParseFile` and `parser.ParseComments`
- map `*ast.FuncDecl` to function/method symbols
- map `*ast.TypeSpec` to type/struct/interface symbols
- map struct/interface fields to child symbols
- map const/var specs
- collect imports with `parser.ImportsOnly` where sufficient
- resolve local identifiers best-effort
- resolve package-level declarations within file/package scope
- collect references by AST inspection
- collect direct call edges
- collect best-effort interface implementation edges

### 15.1 Improvements over the current plugin

The extracted backend should improve on the current plugin in several ways:

1. Separate parser/indexing code from agentruntime operation rendering.
2. Use the shared `Snapshot` abstraction instead of `system.Workspace()` directly.
3. Store offsets as well as line/column ranges.
4. Return typed results directly, not only `operation.Rendered` payloads.
5. Make ambiguity explicit when selector-based lookup finds multiple symbols.
6. Make AST-only limitations explicit in `BackendInfo` and diagnostics.
7. Compile semantic edits into change-set text edits instead of writing files.

### 15.2 Go symbol naming

Suggested qualified names:

- function: `CreateUser`
- method: `Store.Get` or `*Store.Get` with normalized receiver metadata
- struct field: `User.Email`
- interface method: `Reader.Read`
- package symbol: `github.com/acme/app/internal/user.UserService` when module path is known

Keep both raw receiver (`*Store`) and normalized receiver (`Store`) in backend metadata so method lookup can accept either.

### 15.3 Go-specific edit compilation

Initial Go semantic edits:

- `ReplaceFunction`
  - resolve exactly one `function` symbol
  - replace whole declaration range
  - run `go/format` on the file in memory

- `ReplaceMethod`
  - resolve method by receiver + method name
  - replace whole declaration range
  - format in memory

- `AppendFunction`
  - append to file or package-selected file
  - ensure newline separation
  - format in memory

- `DeleteSymbol`
  - delete whole declaration range
  - for grouped const/var/type specs, delete only the selected spec where possible
  - format in memory

- `ReplaceComment`
  - locate declaration doc comment range
  - insert or replace a doc comment immediately before declaration
  - format in memory

- `EnsureGoStructTag`
  - resolve struct symbol
  - locate field by name
  - parse existing tag literal with `reflect.StructTag`-compatible behavior
  - add/update key while preserving unrelated tags
  - format in memory

- `RemoveGoStructTag`
  - resolve struct field
  - remove key and clean empty tag literal when appropriate
  - format in memory

### 15.4 Later Go type-aware backend

A later backend can add `go/types` or `golang.org/x/tools/go/packages` support for:

- cross-package symbol resolution
- precise interface implementation
- precise method sets
- aliases and generics
- build tags
- package variants
- module-aware import path resolution

This can be a capability upgrade without changing the public model.

## 16. Mapping to agentruntime tools later

Once `editor` exists, agentruntime Go language operations can become thin adapters.

Current operation | Future library call
--- | ---
`go_outline` | `ed.Outline(ctx, scope)`
`go_symbol` | `ed.FindSymbols(ctx, selector)`
`go_definition` | `ed.DefinitionAt(ctx, position)` or `ed.Definition(ctx, selector)`
`go_symbol_info` | `ed.Read/Resolve` with enclosing fallback
`go_references` | `ed.References(ctx, selector)`
`go_imports` | `ed.Imports(ctx, scope)`
`go_implementations` | `ed.Implementations(ctx, selector)`
`go_callers` | `ed.Callers(ctx, selector)`
`go_callees` | `ed.Callees(ctx, selector)`
future `go_function_read` | `changes.Read(ctx, function selector)` or `ed.ReadSymbol`
future `go_function_edit` | `changes.Apply(ctx, ReplaceFunction{...})`
future `go_struct_tag_ensure` | `changes.Apply(ctx, EnsureGoStructTag{...})`

Agentruntime should keep operation rendering and schema contracts. `editor` should provide typed core data and edit primitives.

## 17. Example LLM-oriented workflows

### 17.1 Find interface definition and usages

```go
matches, err := ed.FindSymbols(ctx, editor.SymbolSelector{
    Name: "Store",
    Kind: editor.SymbolInterface,
})

refs, err := ed.References(ctx, editor.SymbolSelector{
    QualifiedName: matches[0].QualifiedName,
})
```

### 17.2 Edit a function without line numbers

```go
changes := ed.NewChangeSet()

fragment, err := changes.Read(ctx, editor.SymbolSelector{
    Name: "CreateUser",
    Kind: editor.SymbolFunction,
})

err = changes.Apply(ctx, editor.ReplaceFunction{
    Target: editor.SymbolSelector{ID: fragment.Symbol.ID},
    Source: newSource,
})

diff, err := changes.Diff(ctx)
```

### 17.3 Ensure a JSON tag

```go
err := changes.Apply(ctx, editor.EnsureGoStructTag{
    Struct: editor.SymbolSelector{Name: "User", Kind: editor.SymbolStruct},
    Field:  "Email",
    Key:    "json",
    Value:  "email",
    Options: []string{"omitempty"},
})
```

## 18. Open design questions

1. Should public types live in the root package or subpackages such as `model` and `edit`?
2. Should `Location` use `Path` or `URI` naming?
3. How stable must `SymbolID` be across edits?
4. Should method qualified names preserve pointer receivers (`*T.M`) or normalize them (`T.M`) and store pointer-ness separately?
5. How much should the core know about formatting, versus backend-provided format hooks?
6. Should `Commit` be part of core `ChangeSet`, or should it return a patch that adapters commit?
7. How should tree-sitter node types be represented without leaking them into the common model?

## 19. Proposed first implementation milestone

1. Create core public model types for symbols, locations, occurrences, edges, diagnostics, and selectors.
2. Create `Snapshot` and overlay `ChangeSet` abstractions over `fs.FS`.
3. Implement low-level text edits, conflict detection, and unified diff generation.
4. Implement Go AST indexing for outline/symbol search.
5. Implement selector-based `ReadSymbol`.
6. Implement `ReplaceFunction`, `AppendFunction`, and `DeleteSymbol` for Go.
7. Implement references, imports, callers/callees, and simple package metrics using the same index.
8. Add agentruntime adapter design after the core API stabilizes.
