# Codegate Engine, Capabilities, and Agent Improvement Loop

## 1. Context

The current repository started as `editor`, a language-agnostic source editing
and refactoring library with a Go AST backend. The direction is broader:
publish it as an agent-oriented code improvement engine, likely named
`codegate`.

The target user is not primarily a human IDE user. The primary consumers are
LLMs, agents, review bots, and automation loops that need structured facts,
quality reports, suggestions, executable fixes, and validation results.

The core loop should be:

```text
lookup -> assess -> suggest -> apply -> validate -> reassess
```

This makes the library more than navigation or editing. It becomes a structured
gate for continuous codebase improvement.

## 2. Public Engine Shape

The public facade should move toward an engine builder:

```go
engine, err := codegate.New().
    Roots("workspace").
    WithLanguage(golang.New(golang.Config{})).
    WithLanguage(markdown.New(markdown.Config{})).
    Build(ctx)
```

The engine should expose a small, agent-friendly surface:

```go
type Engine interface {
    Lookup(ctx context.Context, query LookupQuery) (LookupResult, error)
    Assess(ctx context.Context, opts AssessmentOptions) (AssessmentReport, error)
    Suggest(ctx context.Context, opts SuggestOptions) ([]Suggestion, error)
    NewChangeSet() *ChangeSet
    Validate(ctx context.Context, opts ValidationOptions) (ValidationResult, error)
}
```

The existing `Editor` API can remain as an internal or compatibility layer
during migration, but the public story should be `codegate.Engine`.

## 3. Core Capabilities

Capabilities should be first-class and language-neutral. Backends should declare
which capabilities they support and at what depth.

```go
type Capability string

const (
    CapabilityLookup         Capability = "lookup"
    CapabilityStaticAnalysis Capability = "static_analysis"
    CapabilityQuality        Capability = "quality"
    CapabilityEditing        Capability = "editing"
    CapabilityRefactoring    Capability = "refactoring"
    CapabilityValidation     Capability = "validation"
    CapabilityReporting      Capability = "reporting"
)

type CapabilityLevel string

const (
    CapabilityNone         CapabilityLevel = "none"
    CapabilityBasic        CapabilityLevel = "basic"
    CapabilityAdvanced     CapabilityLevel = "advanced"
    CapabilityExperimental CapabilityLevel = "experimental"
)

type CapabilitySupport struct {
    Capability Capability
    Level      CapabilityLevel
    Notes      string
}
```

Backend specs should expose these capabilities:

```go
type LanguageSpec struct {
    ID             LanguageID
    Name           string
    FileExtensions []string
    Capabilities   []CapabilitySupport
    ResolutionMode  string
}
```

Examples:

- Go AST backend:
  - lookup: advanced
  - static analysis: advanced
  - quality: basic
  - editing: advanced
  - refactoring: basic/advanced depending on operation
  - validation: basic/typecheck
  - reporting: basic
- Markdown backend:
  - lookup: basic
  - static analysis: basic
  - quality: basic
  - editing: none or basic later
  - refactoring: none
  - validation: basic
  - reporting: basic
- Tree-sitter Java/Groovy backend:
  - lookup: basic initially
  - static analysis: basic
  - quality: basic
  - editing/refactoring/validation: none until implemented

## 4. Lookup as a Core Concept

Navigation should be renamed and generalized as `Lookup`.

`Navigate` sounds UI/human-oriented. `Lookup` is a better agent-facing concept:
given a position, symbol selector, name, path, heading, import, or structural
query, resolve the relevant entity.

```go
type LookupQuery struct {
    Path           string
    Offset         *int
    Line           int
    Column         int
    Name           string
    QualifiedName  string
    Kind           SymbolKind
    Language       LanguageID
    Scope          Scope
    IncludeDocs    bool
    IncludeRefs    bool
    IncludeCallers bool
}

type LookupResult struct {
    Target      LookupTarget
    Symbols     []Symbol
    Locations   []Location
    Occurrences []Occurrence
    Callers     []CallEdge
    Callees      []CallEdge
    Diagnostics []Diagnostic
    Ambiguous   bool
    Complete    bool
    Confidence  Confidence
}
```

For Go, `Lookup` resolves definitions, symbols, references, callers, callees,
methods, fields, imports, and position targets.

For Markdown, `Lookup` resolves documents, headings, sections, anchors, and
positions inside sections.

For tree-sitter languages, early `Lookup` can resolve structural nodes and
declarations even before type-aware references exist.

## 5. Assessment and Quality Reports

`Assess` should be the high-level quality and architecture reporting API.

The model should be strongly inspired by agentruntime's architecture evaluator:
it separates hard boundary correctness from softer review signals.

```go
type AssessmentReport struct {
    Summary     AssessmentSummary
    Scores      ScoreSet
    Findings    []Finding
    Violations  []Violation
    Suggestions []Suggestion
    Diagnostics []Diagnostic
}

type AssessmentSummary struct {
    Score          int
    FindingCount   int
    ViolationCount int
    SuggestionCount int
}

type ScoreSet struct {
    Overall      int
    Boundary     int
    TestBoundary int
    Coupling     int
    SideEffect   int
    Coverage     int
    Maintainability int
}

type Finding struct {
    Kind      string
    Severity  string
    Location  Location
    Package   string
    Symbol    string
    Evidence  []Evidence
    Allowed   bool
    Reason    string
}
```

Initial score domains:

- **Architecture**: layering, import/dependency direction, unknown package
  coverage, side-effect boundaries.
- **Maintainability**: fan-in/fan-out, large functions, large sections,
  duplicate structures, high-pressure symbols.
- **Safety**: generated-file boundaries, unsupported refactor risks, hidden
  side effects.
- **Coverage**: skipped languages, unknown roots, files outside model coverage.

## 6. Architecture Scoring

Copy the agentruntime architecture scoring approach conceptually:

- component scores:
  - boundary
  - test boundary
  - coupling
  - side effects
  - coverage
- explicit diagnostics with:
  - kind
  - severity
  - package/file/import/symbol metadata
  - allowed flag
  - reason
- explicit gates:
  - `boundary`
  - `test-boundary`
  - `side-effects`
  - `unknown`
  - `all`

The top-level score should not collapse to zero because of soft coupling
signals when hard boundary violations are absent. Hard boundary failures should
remain obvious and dominant.

For Go, architecture assessment can consume package import edges and symbol
metrics.

For Markdown, architecture assessment is less relevant, but quality assessment
can still report structural issues such as missing H1, duplicate headings,
heading-level jumps, oversized sections, and broken internal anchors.

## 7. Suggestions and Fix Loop

Suggestions are agent work items. They should be structured enough for an agent
to decide whether to act and for `codegate` to apply safe operations when
available.

```go
type Suggestion struct {
    ID          string
    Kind        string
    Title       string
    Summary     string
    Severity    string
    Confidence  Confidence
    Risk        RiskLevel
    Locations   []Location
    Evidence    []Evidence
    Metrics     map[string]float64
    Operations  []Operation
    Requires    []ValidationKind
}
```

Suggestions may be:

- executable: contains concrete operations;
- advisory: contains evidence and expected improvement, but needs user/agent
  intent;
- gated: requires validation before commit;
- language-specific: uses backend-specific operations.

The agent loop:

```go
report, _ := engine.Assess(ctx, codegate.AssessmentOptions{})
suggestions, _ := engine.Suggest(ctx, codegate.SuggestOptions{From: report})

changes := engine.NewChangeSet()
changes.Apply(ctx, suggestions[0].Operations...)
validation, _ := changes.Validate(ctx, codegate.ValidationOptions{Kinds: suggestions[0].Requires})
diff, _ := changes.Diff(ctx)
```

## 8. Language-Agnostic Backend Proof

Before public release, add one non-Go backend to prove the abstraction.

Markdown via `goldmark` is a good proof because it is not code in the usual
sense, but it still has structure and quality signals.

Markdown v1 backend:

- parse `.md` with `goldmark`;
- index documents and headings;
- represent headings as namespace/section symbols;
- support outline and lookup by position or heading text;
- emit declaration occurrences for headings;
- emit diagnostics/suggestions for:
  - missing H1;
  - duplicate heading text;
  - skipped heading levels;
  - oversized sections;
  - broken local anchors if feasible.

No Markdown editing/refactoring is required for the first proof. Unsupported
operations should return clear unsupported errors.

## 9. Tree-Sitter Path

Tree-sitter backends for Java, Groovy, and other languages should map parser
nodes into the shared model:

- declarations -> symbols;
- references/uses -> occurrences;
- imports/dependencies -> import edges;
- containment -> graph edges;
- parser errors -> diagnostics;
- tree-sitter node kinds -> backend metadata, not public enums.

The public model should not leak tree-sitter node names. Backends may attach
metadata such as:

```go
Symbol.Tags["tree_sitter.node"] = "method_declaration"
```

Initial tree-sitter support can be read-only:

- lookup;
- outline;
- basic static analysis;
- parser diagnostics;
- structural quality suggestions.

Editing, refactoring, and validation can remain unsupported until a language has
safe rewrite support.

## 10. Publish Direction

Rename/migration target:

```text
github.com/codewandler/codegate
```

The first public release should emphasize:

- agent-oriented structured analysis;
- language capability declarations;
- lookup as a core public concept;
- assessment reports and quality scoring;
- suggestions with executable fix operations where safe;
- explicit validation and diff-before-commit;
- no hidden disk writes, git commands, shell commands, `go test`, or `go build`
  inside core.

The current `editor` implementation can migrate gradually:

- keep existing internals while introducing the engine facade;
- preserve compatibility during transition where practical;
- document that Go-specific operations are backend-specific extensions;
- add Markdown backend proof before `v0.1.0`.

