# DESIGN.md

## Continuous Go Refactoring Toolkit

## 1. Purpose

This document describes a Go library for continuously detecting and applying high-confidence refactor opportunities in Go modules. The library is intended to be embedded into developer tools, CLIs, IDE extensions, code review bots, and autonomous coding agents.

The system combines static analysis, type information, call graphs, optional git/change-impact analysis, and LLM-guided planning to identify and execute refactors such as renaming symbols, moving code, extracting functions, splitting interfaces, changing signatures, and reorganizing packages.

This is not a linter. Linters primarily emit diagnostics. This library is designed around structured refactor proposals and mechanically applicable transformations.

---

## 2. Goals

### 2.1 Primary Goals

- Provide a Go-native library for AST-aware refactoring.
- Detect clear refactor opportunities using static structure and type information.
- Expose refactor proposals as structured data.
- Allow callers to preview, rank, approve, and apply refactors.
- Support multi-file and multi-package transformations.
- Preserve buildability and type correctness.
- Provide extension points for custom detectors, planners, and rewrite operations.
- Support optional LLM involvement without making the core library dependent on any specific LLM provider.

### 2.2 Non-Goals

- Replace `go fmt`, `go vet`, `staticcheck`, or `gopls`.
- Act as a generic code-generation framework.
- Perform unsafe semantic rewrites without validation.
- Hide large architectural decisions behind opaque automation.
- Require git history, tests, or LLMs to be present.

---

## 3. Core Concepts

The library revolves around five primary concepts:

1. **Workspace** — loaded Go module or multi-module repository.
2. **Detector** — finds a possible refactor opportunity.
3. **Proposal** — structured description of a refactor opportunity.
4. **Planner** — turns a proposal into concrete rewrite operations.
5. **Applier** — applies rewrites, validates the result, and reports changed files.

```text
Workspace -> Detectors -> Proposals -> Planner -> Patch -> Validation -> Apply
```

---

## 4. High-Level Architecture

```text
refactor/
  workspace/       // module loading, package graph, type information
  detect/          // detector interfaces and built-in detectors
  proposal/        // proposal schema, scoring, ranking
  plan/            // converts proposals into rewrite plans
  rewrite/         // AST/token/text patching engine
  validate/        // typecheck/build/test validation
  impact/          // blast-radius simulation and dependency analysis
  llm/             // optional LLM planning interface
  report/          // JSON/SARIF/Markdown reporting
```

The package may expose a root facade package for common usage:

```go
import "github.com/acme/go-refactor"
```

and lower-level packages for advanced users:

```go
import "github.com/acme/go-refactor/detect"
import "github.com/acme/go-refactor/rewrite"
import "github.com/acme/go-refactor/impact"
```

---

## 5. Public API Sketch

### 5.1 Loading a Workspace

```go
type Workspace struct {
    Root       string
    Modules    []*Module
    Packages   []*Package
    Fset       *token.FileSet
    Graph      *PackageGraph
    TypeInfo   *TypeIndex
    Symbols    *SymbolIndex
}

type LoadOptions struct {
    Root              string
    IncludeTests      bool
    BuildTags         []string
    Env               []string
    Overlay           map[string][]byte
    AllowPartialLoad  bool
}

func Load(ctx context.Context, opts LoadOptions) (*Workspace, error)
```

The workspace loader should be based on `golang.org/x/tools/go/packages` and should preserve enough source mapping to support reliable rewrites.

---

### 5.2 Running Detectors

```go
type Detector interface {
    ID() string
    Name() string
    Detect(ctx context.Context, ws *Workspace) ([]Proposal, error)
}

type Engine struct {
    Detectors []Detector
    Ranker    ProposalRanker
}

func NewEngine(opts EngineOptions) *Engine

func (e *Engine) Analyze(ctx context.Context, ws *Workspace) (*AnalysisResult, error)
```

Example:

```go
ws, err := refactor.Load(ctx, refactor.LoadOptions{
    Root:         ".",
    IncludeTests: true,
})

engine := refactor.NewEngine(refactor.EngineOptions{
    Detectors: refactor.DefaultDetectors(),
})

result, err := engine.Analyze(ctx, ws)
```

---

### 5.3 Proposals

```go
type Proposal struct {
    ID          string
    DetectorID  string
    Kind        RefactorKind
    Title       string
    Summary     string
    Rationale   string
    Confidence  Confidence
    Impact      ImpactEstimate
    Targets     []Target
    Evidence    []Evidence
    Tags        []string
    Risk        RiskLevel
    Metadata    map[string]any
}

type RefactorKind string

const (
    RenameSymbol        RefactorKind = "rename_symbol"
    MoveSymbol          RefactorKind = "move_symbol"
    MovePackage         RefactorKind = "move_package"
    ExtractFunction     RefactorKind = "extract_function"
    InlineFunction      RefactorKind = "inline_function"
    ChangeSignature     RefactorKind = "change_signature"
    ExtractInterface    RefactorKind = "extract_interface"
    SplitInterface      RefactorKind = "split_interface"
    InlineInterface     RefactorKind = "inline_interface"
    ExtractStruct       RefactorKind = "extract_struct"
    IntroduceGeneric    RefactorKind = "introduce_generic"
    IntroduceNamedType  RefactorKind = "introduce_named_type"
    ExtractPackage      RefactorKind = "extract_package"
)
```

A proposal is intentionally not a patch. It is a structured recommendation with supporting evidence.

---

### 5.4 Planning and Applying Refactors

```go
type Planner interface {
    Supports(kind RefactorKind) bool
    Plan(ctx context.Context, ws *Workspace, p Proposal) (*Plan, error)
}

type Plan struct {
    ID          string
    ProposalID  string
    Operations  []Operation
    Preview     PatchPreview
    Risk        RiskLevel
    Requires    []ValidationKind
}

type Applier interface {
    Apply(ctx context.Context, ws *Workspace, plan *Plan, opts ApplyOptions) (*ApplyResult, error)
}
```

Example:

```go
proposal := result.Proposals[0]
plan, err := refactor.DefaultPlanner().Plan(ctx, ws, proposal)

applied, err := refactor.Apply(ctx, ws, plan, refactor.ApplyOptions{
    DryRun: true,
})
```

---

## 6. Refactor Operations

Operations are low-level transformation primitives. Planners compose operations to create full refactors.

```go
type Operation interface {
    Kind() OperationKind
    Validate(ctx context.Context, ws *Workspace) error
    Apply(ctx context.Context, rw *Rewriter) error
}

type OperationKind string

const (
    OpRenameSymbol       OperationKind = "rename_symbol"
    OpMoveDecl           OperationKind = "move_decl"
    OpMoveFile           OperationKind = "move_file"
    OpRewriteImport      OperationKind = "rewrite_import"
    OpAddImport          OperationKind = "add_import"
    OpRemoveImport       OperationKind = "remove_import"
    OpReplaceExpr        OperationKind = "replace_expr"
    OpReplaceStmt        OperationKind = "replace_stmt"
    OpInsertDecl         OperationKind = "insert_decl"
    OpDeleteDecl         OperationKind = "delete_decl"
    OpChangeSignature    OperationKind = "change_signature"
    OpUpdateCallSites    OperationKind = "update_call_sites"
    OpExtractFunction    OperationKind = "extract_function"
)
```

The public operation layer should be stable and serializable so external tools can inspect and approve proposed changes.

---

## 7. Built-In Detectors

### 7.1 Duplication Detectors

#### Duplicate Function Detector

Finds structurally similar function bodies using normalized AST fingerprints.

Signals:

- Similar statement sequence.
- Same control-flow shape.
- Differences limited to identifiers, literals, or selector names.

Potential refactors:

- Extract helper function.
- Introduce generic function.
- Parameterize literals.

#### Repeated Parameter Group Detector

Finds recurring parameter bundles across functions.

Example:

```go
ctx context.Context, db *sql.DB, logger *zap.Logger
```

Potential refactors:

- Introduce dependency struct.
- Introduce receiver object.
- Introduce options/config struct.

#### Repeated Field Group Detector

Finds structs with repeated field clusters.

Potential refactors:

- Extract embedded struct.
- Introduce shared domain type.

---

### 7.2 Complexity Detectors

#### Large Function Detector

Signals:

- High AST node count.
- High cyclomatic complexity.
- Long statement list.
- Many local variables.

Potential refactors:

- Extract function.
- Split phases.
- Introduce guard clauses.

#### Deep Nesting Detector

Signals:

- Nested `if`, `for`, and `switch` blocks.

Potential refactors:

- Invert conditions.
- Introduce early returns.
- Extract condition helper.

---

### 7.3 API Shape Detectors

#### Large Parameter List Detector

Signals:

- Function or method has more than configured number of parameters.
- Related functions share similar parameter prefixes or suffixes.

Potential refactors:

- Introduce config struct.
- Introduce options struct.
- Introduce receiver type.

#### Boolean Flag Parameter Detector

Signals:

```go
func Render(w io.Writer, compact bool)
```

Potential refactors:

- Split into two functions.
- Replace with options struct.
- Introduce named behavior.

#### Primitive Obsession Detector

Signals:

- Repeated `string`, `int`, or `map[string]any` values used in semantically meaningful positions.
- Parameter names suggest domain concepts such as `userID`, `tenantID`, `orderID`.

Potential refactors:

- Introduce named type.
- Introduce small struct.

---

### 7.4 Interface Detectors

#### Single-Implementation Interface Detector

Signals:

- Interface has exactly one implementation.
- Interface is declared in provider package rather than consumer package.
- Interface is not used by tests or external packages.

Potential refactors:

- Inline interface.
- Replace with concrete type.
- Move interface to consumer package.

#### Fat Interface Detector

Signals:

- Consumers use disjoint subsets of interface methods.
- Method usage clusters are separable.

Potential refactors:

- Split interface.
- Compose smaller interfaces.
- Move minimal interfaces to consumers.

---

### 7.5 Package Architecture Detectors

#### Low Cohesion Package Detector

Signals:

- Weak internal reference density.
- Multiple file clusters with few shared symbols.
- Strong external affinities to different packages.

Potential refactors:

- Split package.
- Move files or symbols.

#### Main Package Bloat Detector

Signals:

- Business logic, persistence logic, or domain types in `main`.
- Large number of non-entrypoint functions in `cmd/...`.

Potential refactors:

- Extract library package.
- Move domain logic to internal package.

#### High Fan-In Package Detector

Signals:

- Many packages import one package.
- Public API surface is large.
- Simulated change impact is high.

Potential refactors:

- Split package.
- Hide internals.
- Introduce narrower API packages.

---

### 7.6 Impact Detectors

#### Blast Radius Detector

Simulates API mutations to estimate coupling.

Candidate mutations:

- Rename exported symbol.
- Remove interface method.
- Change function parameter.
- Make exported field private.
- Move package import path.

Metrics:

- Number of files requiring edits.
- Number of packages failing to typecheck.
- Number of call sites affected.
- Number of interface implementations affected.
- Number of tests affected.

Potential refactors:

- Encapsulate fields.
- Narrow interfaces.
- Split packages.
- Introduce adapters.

#### Optional Git Co-Change Detector

Signals:

- Files or symbols frequently changed together.
- Repeated bug fixes touch the same API boundary.
- High churn in high fan-in packages.

Potential refactors:

- Extract abstraction.
- Consolidate duplicated behavior.
- Move unstable code behind stable interface.

---

## 8. LLM Integration

The core library should not directly depend on an LLM provider. Instead, it exposes structured context and receives structured decisions.

### 8.1 LLM Interface

```go
type Advisor interface {
    Advise(ctx context.Context, req AdviceRequest) (*AdviceResponse, error)
}

type AdviceRequest struct {
    WorkspaceSummary WorkspaceSummary
    Proposal          Proposal
    Evidence          []Evidence
    CandidatePlans    []PlanSummary
    Constraints       []Constraint
}

type AdviceResponse struct {
    Decision       Decision
    Rationale      string
    SuggestedName  string
    SelectedPlanID string
    Edits          []SuggestedEdit
}
```

### 8.2 LLM Responsibilities

Good uses for an LLM:

- Naming extracted functions, packages, structs, and interfaces.
- Choosing among competing valid refactor plans.
- Explaining rationale for a proposal.
- Detecting semantic mismatch between names and behavior.
- Suggesting package boundaries from code clusters.

Bad uses for an LLM:

- Directly editing raw source without validation.
- Guessing type correctness.
- Bypassing the operation planner.
- Applying large architectural changes without explicit approval.

The recommended model is:

```text
LLM proposes intent and names.
Refactor engine performs mechanical rewrites.
Validator proves the result still builds.
```

---

## 9. Safety Model

### 9.1 Validation Stages

Every applied plan should pass a configurable validation pipeline.

```go
type Validator interface {
    Validate(ctx context.Context, ws *Workspace, patch *Patch) (*ValidationResult, error)
}
```

Built-in validators:

- Parse validation.
- Typecheck validation.
- Package load validation.
- `go test` validation.
- Public API compatibility validation.
- Import cycle validation.
- Generated file exclusion validation.

### 9.2 Risk Levels

```go
type RiskLevel string

const (
    RiskLow      RiskLevel = "low"
    RiskMedium   RiskLevel = "medium"
    RiskHigh     RiskLevel = "high"
)
```

Example risk classification:

| Refactor | Default Risk |
|---|---|
| Rename local variable | Low |
| Rename unexported function | Low |
| Rename exported symbol | Medium |
| Extract function | Medium |
| Move symbol across package | Medium/High |
| Split package | High |
| Change public function signature | High |

### 9.3 Approval Policy

```go
type ApprovalPolicy interface {
    Approve(ctx context.Context, p Proposal, plan *Plan) Decision
}
```

Common policies:

- Apply only low-risk changes.
- Apply only changes with test coverage.
- Require human approval for public API changes.
- Require human approval for package moves.
- Never edit generated files.

---

## 10. Change Impact Simulation

The library should support mutation-based impact analysis independent of git history.

### 10.1 API

```go
type ImpactAnalyzer interface {
    AnalyzeMutation(ctx context.Context, ws *Workspace, m Mutation) (*ImpactReport, error)
}

type Mutation struct {
    Kind   MutationKind
    Target Target
    Params map[string]any
}

type ImpactReport struct {
    Mutation        Mutation
    BrokenPackages  []PackageID
    BrokenFiles     []string
    AffectedSymbols []SymbolID
    CallSites       []CallSite
    Score           float64
}
```

### 10.2 Example Mutations

```go
Mutation{Kind: RenameExportedSymbol, Target: Symbol("auth.User")}
Mutation{Kind: RemoveInterfaceMethod, Target: Symbol("storage.Repository.Save")}
Mutation{Kind: PrivatizeStructField, Target: Symbol("config.Config.DB")}
Mutation{Kind: MovePackage, Target: Package("internal/auth")}
```

### 10.3 Use Cases

- Find leaky public fields.
- Identify overly central packages.
- Detect high-coupling interfaces.
- Estimate cost of package moves.
- Prioritize refactors based on blast radius.

---

## 11. Workspace Indexes

The library should build reusable indexes to avoid every detector walking the entire AST independently.

### 11.1 Symbol Index

Maps declarations to uses.

```go
type SymbolIndex struct {
    Decls map[SymbolID]DeclInfo
    Uses  map[SymbolID][]UseInfo
}
```

### 11.2 Call Graph Index

Tracks function and method calls.

```go
type CallGraph struct {
    Nodes []Callable
    Edges []CallEdge
}
```

### 11.3 Package Graph Index

Tracks imports and reverse imports.

```go
type PackageGraph struct {
    Imports    map[PackageID][]PackageID
    Importers  map[PackageID][]PackageID
}
```

### 11.4 Structural Fingerprint Index

Stores normalized AST hashes for duplicate detection.

```go
type FingerprintIndex struct {
    Funcs map[FuncID]Fingerprint
    Blocks map[BlockID]Fingerprint
}
```

---

## 12. Patch and Rewrite System

The rewrite system should support both AST-aware and text-aware operations.

### 12.1 Requirements

- Preserve comments where possible.
- Preserve stable formatting by running `gofmt` only on changed files as a final normalization step.
- Avoid editing generated files by default.
- Avoid overlapping edits.
- Support dry runs.
- Support unified diff output.
- Support source overlays for IDE integration.

### 12.2 Patch Model

```go
type Patch struct {
    FileEdits []FileEdit
}

type FileEdit struct {
    Path  string
    Edits []TextEdit
}

type TextEdit struct {
    Start token.Pos
    End   token.Pos
    New   []byte
}
```

Although the library works with AST and type information, the final patch should be represented as text edits so it can integrate with LSPs, editors, and code review systems.

---

## 13. Example Built-In Workflow

```go
func main() {
    ctx := context.Background()

    ws, err := refactor.Load(ctx, refactor.LoadOptions{
        Root:         ".",
        IncludeTests: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    engine := refactor.NewEngine(refactor.EngineOptions{
        Detectors: refactor.DefaultDetectors(),
    })

    analysis, err := engine.Analyze(ctx, ws)
    if err != nil {
        log.Fatal(err)
    }

    proposals := analysis.Top(10)

    for _, p := range proposals {
        fmt.Printf("%s: %s\n", p.Kind, p.Title)
    }
}
```

Applying one refactor:

```go
planner := refactor.DefaultPlanner()
plan, err := planner.Plan(ctx, ws, proposals[0])
if err != nil {
    log.Fatal(err)
}

result, err := refactor.Apply(ctx, ws, plan, refactor.ApplyOptions{
    DryRun:   false,
    Validate: refactor.ValidationStrict,
})
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Diff)
```

---

## 14. CLI Built on the Library

A CLI is not the primary product, but should be easy to build on top.

Example commands:

```bash
goref analyze ./...
goref propose --kind duplicate-function
goref plan <proposal-id>
goref apply <proposal-id>
goref impact symbol github.com/acme/app/auth.User
goref daemon
```

Output formats:

- Human-readable table.
- JSON.
- Markdown.
- SARIF.
- Unified diff.

---

## 15. Example Proposal JSON

```json
{
  "id": "prop_123",
  "detector_id": "repeated_parameter_group",
  "kind": "extract_struct",
  "title": "Extract repeated dependency group into service struct",
  "summary": "8 functions accept the same ctx/db/logger/config parameter group.",
  "confidence": "high",
  "risk": "medium",
  "targets": [
    { "kind": "function", "name": "CreateUser", "file": "internal/user/create.go" },
    { "kind": "function", "name": "DeleteUser", "file": "internal/user/delete.go" }
  ],
  "evidence": [
    {
      "kind": "repeated_parameter_group",
      "message": "Parameter group appears in 8 functions."
    }
  ],
  "impact": {
    "files": 6,
    "packages": 1,
    "call_sites": 14
  }
}
```

---

## 16. Detector Scoring

Each detector should return evidence and a confidence score, but final prioritization should happen separately.

Suggested scoring inputs:

- Structural certainty.
- Number of affected files.
- Number of call sites.
- Type safety of the rewrite.
- Whether the symbol is exported.
- Whether tests cover affected packages.
- Whether git history shows churn.
- Whether simulated blast radius is high.
- Whether the change can be automatically validated.

Example:

```go
type ProposalRanker interface {
    Rank(ctx context.Context, proposals []Proposal) ([]Proposal, error)
}
```

---

## 17. Configuration

```go
type Config struct {
    GeneratedFilePolicy GeneratedFilePolicy
    MaxRiskAutoApply    RiskLevel
    IncludeTests        bool
    EnableLLM           bool
    Detectors           DetectorConfig
    Validation          ValidationConfig
}
```

Example YAML:

```yaml
generated_files: skip
max_risk_auto_apply: low
include_tests: true
validation:
  typecheck: true
  go_test: true
detectors:
  duplicate_function:
    enabled: true
    min_similarity: 0.82
  large_function:
    enabled: true
    max_ast_nodes: 180
  repeated_parameter_group:
    enabled: true
    min_occurrences: 3
```

---

## 18. Extensibility

Third-party users should be able to add:

- Custom detectors.
- Custom proposal rankers.
- Custom planners.
- Custom validators.
- Custom approval policies.
- Custom LLM advisors.
- Custom reporters.

Minimal detector example:

```go
type MyDetector struct{}

func (d MyDetector) ID() string { return "my_detector" }
func (d MyDetector) Name() string { return "My Detector" }

func (d MyDetector) Detect(ctx context.Context, ws *refactor.Workspace) ([]refactor.Proposal, error) {
    // Inspect ws.Packages, ws.Symbols, ws.TypeInfo, etc.
    return proposals, nil
}
```

---

## 19. Generated Code Policy

Generated files should be skipped by default.

Generated file detection:

- Standard `Code generated ... DO NOT EDIT.` comment.
- Common generated extensions or names.
- Configurable include/exclude patterns.

Users may opt in to generated-code refactors for controlled environments.

---

## 20. Public API Compatibility

Public API changes require special handling.

The library should support:

- Detecting exported symbol changes.
- Emitting compatibility warnings.
- Generating migration patches across the same workspace.
- Marking changes as breaking if downstream consumers are unknown.

Default rule:

> Never auto-apply breaking public API changes unless explicitly configured.

---

## 21. Testing Strategy

### 21.1 Unit Tests

- Detector fixtures.
- Rewrite operation fixtures.
- Proposal scoring tests.
- AST fingerprint tests.

### 21.2 Golden Tests

Each refactor operation should have before/after golden files.

```text
testdata/
  rename_symbol/
    input/
    expected/
  extract_function/
    input/
    expected/
```

### 21.3 Integration Tests

- Real open-source repositories.
- Multi-module repositories.
- Repositories with build tags.
- Repositories with generated code.

### 21.4 Validation Tests

Every applied refactor should verify:

- Source parses.
- Packages typecheck.
- Imports resolve.
- Optional tests pass.

---

## 22. MVP Scope

A practical MVP should focus on high-confidence, high-value refactors.

### MVP Detectors

- Repeated parameter group.
- Duplicate function bodies.
- Large parameter list.
- Boolean flag parameter.
- Single-implementation interface.
- Fat interface usage subsets.
- Main package bloat.
- High fan-in package.
- Blast-radius simulation for exported symbols.

### MVP Operations

- Rename symbol.
- Move declaration between files in same package.
- Rewrite imports.
- Change function signature and update call sites.
- Extract struct from parameter group.
- Split interface.
- Inline interface.

### MVP Validators

- Parse.
- Typecheck.
- Import cycle check.
- Optional `go test ./...`.

---

## 23. Future Work

- Semantic package splitting.
- Cross-module migration planning.
- Git-history-aware co-change analysis.
- Test coverage-aware risk scoring.
- IDE/LSP integration.
- Code review bot integration.
- Autonomous low-risk cleanup mode.
- Human approval workflow for high-risk changes.
- Repository-specific learned conventions.
- Refactor regression detection.

---

## 24. Summary

The proposed library provides a structured foundation for continuous Go code improvement. Its central idea is to separate detection, planning, rewriting, and validation.

The most important design principle is:

> Let analysis and LLMs decide what should change, but let typed, deterministic rewrite operations decide how code changes.

This keeps the system useful for autonomous agents while preserving the safety expectations of Go developers.

