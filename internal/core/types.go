package core

import "context"

type LanguageID string

const (
	Go       LanguageID = "go"
	Markdown LanguageID = "markdown"
)

type BackendInfo struct {
	Language       LanguageID        `json:"language,omitempty"`
	Name           string            `json:"name,omitempty"`
	ResolutionMode string            `json:"resolution_mode,omitempty"`
	Complete       bool              `json:"complete,omitempty"`
	Diagnostics    []Diagnostic      `json:"diagnostics,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type BackendSpec struct {
	Language       LanguageID          `json:"language"`
	Name           string              `json:"name"`
	FileExtensions []string            `json:"file_extensions"`
	Capabilities   []CapabilitySupport `json:"capabilities,omitempty"`
	Operations     OperationSupport    `json:"operations,omitempty"`
	ResolutionMode string              `json:"resolution_mode"`
}

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
	Capability Capability      `json:"capability"`
	Level      CapabilityLevel `json:"level"`
	Notes      string          `json:"notes,omitempty"`
}

type OperationSupport struct {
	Lookup          []string          `json:"lookup,omitempty"`
	AssessmentGates []AssessmentGate  `json:"assessment_gates,omitempty"`
	Assessment      AssessmentSupport `json:"assessment,omitempty"`
	ValidationKinds []ValidationKind  `json:"validation_kinds,omitempty"`
	EditOperations  []OperationKind   `json:"edit_operations,omitempty"`
	RefactorKinds   []RefactorKind    `json:"refactor_kinds,omitempty"`
	Notes           []string          `json:"notes,omitempty"`
}

type AssessmentSupport struct {
	Gates      []AssessmentGate   `json:"gates,omitempty"`
	Metrics    []MetricSupport    `json:"metrics,omitempty"`
	Findings   []FindingSupport   `json:"findings,omitempty"`
	Violations []ViolationSupport `json:"violations,omitempty"`
}

type MetricSupport struct {
	ID          string          `json:"id"`
	Category    string          `json:"category,omitempty"`
	Level       CapabilityLevel `json:"level,omitempty"`
	Description string          `json:"description,omitempty"`
}

type FindingSupport struct {
	ID          string          `json:"id"`
	Category    string          `json:"category,omitempty"`
	Level       CapabilityLevel `json:"level,omitempty"`
	Description string          `json:"description,omitempty"`
}

type ViolationSupport struct {
	ID          string          `json:"id"`
	Category    string          `json:"category,omitempty"`
	Level       CapabilityLevel `json:"level,omitempty"`
	Description string          `json:"description,omitempty"`
}

type Snapshot interface {
	ListFiles(ctx context.Context, scope Scope) ([]string, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
}

type Backend interface {
	Spec() BackendSpec
	Index(ctx context.Context, snapshot Snapshot, scope Scope) (*Index, error)
	CompileEdit(ctx context.Context, snapshot Snapshot, op Operation) ([]FileEdit, error)
	Format(ctx context.Context, path string, src []byte) ([]byte, error)
	Suggest(ctx context.Context, snapshot Snapshot, scope Scope) ([]Proposal, error)
}

type Index struct {
	Documents   []Document          `json:"documents,omitempty"`
	Packages    []PackageInfo       `json:"packages,omitempty"`
	Symbols     []Symbol            `json:"symbols,omitempty"`
	Occurrences []Occurrence        `json:"occurrences,omitempty"`
	Edges       []Edge              `json:"edges,omitempty"`
	Imports     []ImportEdge        `json:"imports,omitempty"`
	Diagnostics []Diagnostic        `json:"diagnostics,omitempty"`
	ByID        map[SymbolID]Symbol `json:"-"`
	ByName      map[string][]Symbol `json:"-"`
	UnitFiles   map[string][]string `json:"-"`
	FileUnits   map[string]string   `json:"-"`
	FileLOC     map[string]int      `json:"-"`
}

func NewIndex() *Index {
	return &Index{
		ByID:      map[SymbolID]Symbol{},
		ByName:    map[string][]Symbol{},
		UnitFiles: map[string][]string{},
		FileUnits: map[string]string{},
		FileLOC:   map[string]int{},
	}
}

type Document struct {
	URI      string     `json:"uri"`
	Language LanguageID `json:"language"`
	UnitID   string     `json:"unit_id,omitempty"`
	Version  string     `json:"version,omitempty"`
}

type PackageInfo struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Dir         string       `json:"dir,omitempty"`
	Files       []string     `json:"files,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri,omitempty"`
	Range Range  `json:"range,omitempty"`
}

type SymbolID string

type SymbolKind string

const (
	SymbolModule      SymbolKind = "module"
	SymbolPackage     SymbolKind = "package"
	SymbolNamespace   SymbolKind = "namespace"
	SymbolFile        SymbolKind = "file"
	SymbolType        SymbolKind = "type"
	SymbolClass       SymbolKind = "class"
	SymbolStruct      SymbolKind = "struct"
	SymbolInterface   SymbolKind = "interface"
	SymbolEnum        SymbolKind = "enum"
	SymbolFunction    SymbolKind = "function"
	SymbolMethod      SymbolKind = "method"
	SymbolConstructor SymbolKind = "constructor"
	SymbolField       SymbolKind = "field"
	SymbolProperty    SymbolKind = "property"
	SymbolConst       SymbolKind = "const"
	SymbolVar         SymbolKind = "var"
	SymbolImport      SymbolKind = "import"
	SymbolParameter   SymbolKind = "parameter"
	SymbolLocal       SymbolKind = "local"
)

type Symbol struct {
	ID             SymbolID          `json:"id,omitempty"`
	Language       LanguageID        `json:"language,omitempty"`
	Kind           SymbolKind        `json:"kind,omitempty"`
	Name           string            `json:"name"`
	QualifiedName  string            `json:"qualified_name,omitempty"`
	ContainerID    SymbolID          `json:"container_id,omitempty"`
	ContainerName  string            `json:"container_name,omitempty"`
	UnitID         string            `json:"unit_id,omitempty"`
	Location       Location          `json:"location,omitempty"`
	SelectionRange Range             `json:"selection_range,omitempty"`
	BodyRange      Range             `json:"body_range,omitempty"`
	Signature      string            `json:"signature,omitempty"`
	Doc            string            `json:"doc,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	Children       []Symbol          `json:"children,omitempty"`
	Backend        BackendInfo       `json:"backend,omitempty"`
}

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
	SymbolID SymbolID       `json:"symbol_id,omitempty"`
	Kind     OccurrenceKind `json:"kind"`
	Name     string         `json:"name,omitempty"`
	Location Location       `json:"location,omitempty"`
	Preview  string         `json:"preview,omitempty"`
	Evidence []Evidence     `json:"evidence,omitempty"`
}

type EdgeKind string

const (
	EdgeContains   EdgeKind = "contains"
	EdgeDeclares   EdgeKind = "declares"
	EdgeReferences EdgeKind = "references"
	EdgeCalls      EdgeKind = "calls"
	EdgeImports    EdgeKind = "imports"
	EdgeImplements EdgeKind = "implements"
	EdgeOverrides  EdgeKind = "overrides"
	EdgeEmbeds     EdgeKind = "embeds"
)

type Edge struct {
	Kind     EdgeKind   `json:"kind"`
	From     string     `json:"from"`
	To       string     `json:"to"`
	Location Location   `json:"location,omitempty"`
	Weight   int        `json:"weight,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

type Diagnostic struct {
	Location Location `json:"location,omitempty"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
}

type ValidationKind string

const (
	ValidationParse     ValidationKind = "parse"
	ValidationTypecheck ValidationKind = "typecheck"
	ValidationExternal  ValidationKind = "external"
)

type ValidationOptions struct {
	Scope    Scope            `json:"scope,omitempty"`
	Kinds    []ValidationKind `json:"kinds,omitempty"`
	External []string         `json:"external,omitempty"`
}

type ValidationResult struct {
	Passed         bool             `json:"passed"`
	Kinds          []ValidationKind `json:"kinds,omitempty"`
	Diagnostics    []Diagnostic     `json:"diagnostics,omitempty"`
	AffectedPaths  []string         `json:"affected_paths,omitempty"`
	ResolutionMode string           `json:"resolution_mode,omitempty"`
	Complete       bool             `json:"complete"`
}

type Validator interface {
	Validate(ctx context.Context, snapshot Snapshot, opts ValidationOptions) (ValidationResult, error)
}

type ValidationAdapter interface {
	Name() string
	Validate(ctx context.Context, snapshot Snapshot, opts ValidationOptions) (ValidationResult, error)
}

type AssessmentGate string

const (
	AssessmentGateAll             AssessmentGate = "all"
	AssessmentGateArchitecture    AssessmentGate = "architecture"
	AssessmentGateMaintainability AssessmentGate = "maintainability"
	AssessmentGateSafety          AssessmentGate = "safety"
	AssessmentGateCoverage        AssessmentGate = "coverage"
)

type AssessmentOptions struct {
	Scope           Scope              `json:"scope,omitempty"`
	SuggestionLimit int                `json:"suggestion_limit,omitempty"`
	TopUnitLimit    int                `json:"top_unit_limit,omitempty"`
	Gates           []AssessmentGate   `json:"gates,omitempty"`
	Architecture    *ArchitectureRules `json:"architecture,omitempty"`
}

type ArchitectureRuleAction string

const (
	ArchitectureRuleAllow ArchitectureRuleAction = "allow"
	ArchitectureRuleDeny  ArchitectureRuleAction = "deny"
)

type ArchitectureRules struct {
	ModulePath   string                       `json:"module_path,omitempty"`
	Imports      []ArchitectureImportRule     `json:"imports,omitempty"`
	TestImports  []ArchitectureImportRule     `json:"test_imports,omitempty"`
	Layers       []ArchitectureLayer          `json:"layers,omitempty"`
	Dependencies []ArchitectureDependencyRule `json:"dependencies,omitempty"`
	Effects      []ArchitectureEffectRule     `json:"effects,omitempty"`
	Coupling     ArchitectureCouplingRules    `json:"coupling,omitempty"`
	Exceptions   []ArchitectureException      `json:"exceptions,omitempty"`
}

type ArchitectureImportRule struct {
	From   string                 `json:"from,omitempty"`
	To     string                 `json:"to,omitempty"`
	Action ArchitectureRuleAction `json:"action,omitempty"`
	Reason string                 `json:"reason,omitempty"`
}

type ArchitectureLayer struct {
	Name     string   `json:"name"`
	Prefixes []string `json:"prefixes,omitempty"`
}

type ArchitectureDependencyRule struct {
	FromLayer string                 `json:"from_layer"`
	ToLayer   string                 `json:"to_layer"`
	Action    ArchitectureRuleAction `json:"action,omitempty"`
	Reason    string                 `json:"reason,omitempty"`
}

type ArchitectureEffectRule struct {
	Name     string                 `json:"name,omitempty"`
	Scope    ArchitectureScope      `json:"scope,omitempty"`
	Imports  []string               `json:"imports,omitempty"`
	Calls    []ArchitectureCallRule `json:"calls,omitempty"`
	Action   ArchitectureRuleAction `json:"action,omitempty"`
	Severity string                 `json:"severity,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
}

type ArchitectureScope struct {
	Layers   []string `json:"layers,omitempty"`
	Packages []string `json:"packages,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

type ArchitectureCallRule struct {
	Import string `json:"import"`
	Symbol string `json:"symbol"`
}

type ArchitectureCouplingRules struct {
	FanOutThreshold int                       `json:"fan_out_threshold,omitempty"`
	Layers          []string                  `json:"layers,omitempty"`
	ReviewedFanOut  []ArchitecturePackageNote `json:"reviewed_fan_out,omitempty"`
}

type ArchitecturePackageNote struct {
	Package string `json:"package"`
	Reason  string `json:"reason,omitempty"`
}

type ArchitectureException struct {
	Kind      string `json:"kind,omitempty"`
	Package   string `json:"package,omitempty"`
	Import    string `json:"import,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	FromLayer string `json:"from_layer,omitempty"`
	ToLayer   string `json:"to_layer,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type AssessmentReport struct {
	Root        string                 `json:"root"`
	Language    string                 `json:"language"`
	Summary     AssessmentSummary      `json:"summary"`
	Scores      ScoreSet               `json:"scores"`
	Validation  ValidationSummary      `json:"validation"`
	Findings    []Finding              `json:"findings,omitempty"`
	Violations  []Violation            `json:"violations,omitempty"`
	TopUnits    []UnitMetrics          `json:"top_units,omitempty"`
	Suggestions []AssessmentSuggestion `json:"suggestions,omitempty"`
	Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
}

type AssessmentSummary struct {
	Score           int `json:"score"`
	Packages        int `json:"packages"`
	Symbols         int `json:"symbols"`
	Imports         int `json:"imports"`
	Suggestions     int `json:"suggestions"`
	ExecutableFixes int `json:"executable_fixes"`
	Findings        int `json:"findings"`
	Violations      int `json:"violations"`
	Diagnostics     int `json:"diagnostics"`
}

type ScoreSet struct {
	Overall         int     `json:"overall"`
	Boundary        int     `json:"boundary,omitempty"`
	TestBoundary    int     `json:"test_boundary,omitempty"`
	Coupling        int     `json:"coupling,omitempty"`
	SideEffect      int     `json:"side_effect,omitempty"`
	Coverage        int     `json:"coverage,omitempty"`
	Maintainability int     `json:"maintainability"`
	Pressure        float64 `json:"pressure"`
}

type ValidationSummary struct {
	Passed         bool   `json:"passed"`
	ResolutionMode string `json:"resolution_mode"`
	Diagnostics    int    `json:"diagnostics"`
	Files          int    `json:"files"`
	Complete       bool   `json:"complete"`
}

type Finding struct {
	Kind     string     `json:"kind"`
	Severity string     `json:"severity"`
	Location Location   `json:"location,omitempty"`
	Package  string     `json:"package,omitempty"`
	Symbol   string     `json:"symbol,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Allowed  bool       `json:"allowed,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

type Violation struct {
	Kind     string     `json:"kind"`
	Severity string     `json:"severity"`
	Location Location   `json:"location,omitempty"`
	Package  string     `json:"package,omitempty"`
	Symbol   string     `json:"symbol,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

type AssessmentSuggestion struct {
	ID         string             `json:"id"`
	Kind       RefactorKind       `json:"kind"`
	Title      string             `json:"title"`
	Summary    string             `json:"summary,omitempty"`
	Confidence Confidence         `json:"confidence"`
	Risk       RiskLevel          `json:"risk"`
	Operations int                `json:"operations"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Evidence   []Evidence         `json:"evidence,omitempty"`
}

type AssessmentProvider interface {
	Assess(ctx context.Context, snapshot Snapshot, scope Scope, opts AssessmentOptions) (AssessmentReport, error)
}

type Evidence struct {
	Kind     string             `json:"kind"`
	Message  string             `json:"message,omitempty"`
	Location Location           `json:"location,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
}

type Scope struct {
	Root         string     `json:"root,omitempty"`
	Path         string     `json:"path,omitempty"`
	UnitID       string     `json:"unit_id,omitempty"`
	Language     LanguageID `json:"language,omitempty"`
	IncludeTests bool       `json:"include_tests,omitempty"`
	MaxFiles     int        `json:"max_files,omitempty"`
	MaxBytes     int64      `json:"max_bytes,omitempty"`
}

type SymbolSelector struct {
	ID            SymbolID   `json:"id,omitempty"`
	Language      LanguageID `json:"language,omitempty"`
	Name          string     `json:"name,omitempty"`
	QualifiedName string     `json:"qualified_name,omitempty"`
	Kind          SymbolKind `json:"kind,omitempty"`
	Container     string     `json:"container,omitempty"`
	UnitID        string     `json:"unit_id,omitempty"`
	Path          string     `json:"path,omitempty"`
	IncludeTests  *bool      `json:"include_tests,omitempty"`
}

type PositionSelector struct {
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Offset *int   `json:"offset,omitempty"`
}

type ResolveResult struct {
	Matches     []Symbol     `json:"matches,omitempty"`
	Ambiguous   bool         `json:"ambiguous,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type NavigationTarget struct {
	Text            string   `json:"text,omitempty"`
	Name            string   `json:"name,omitempty"`
	NodeKind        string   `json:"node_kind,omitempty"`
	PackageID       string   `json:"package_id,omitempty"`
	Location        Location `json:"location,omitempty"`
	EnclosingSymbol *Symbol  `json:"enclosing_symbol,omitempty"`
}

type NavigationOptions struct {
	Scope             Scope `json:"scope,omitempty"`
	IncludeDocs       bool  `json:"include_docs,omitempty"`
	FallbackEnclosing bool  `json:"fallback_enclosing,omitempty"`
	MaxResults        int   `json:"max_results,omitempty"`
}

type NavigationResult struct {
	Target         NavigationTarget `json:"target"`
	Symbols        []Symbol         `json:"symbols,omitempty"`
	Locations      []Location       `json:"locations,omitempty"`
	Diagnostics    []Diagnostic     `json:"diagnostics,omitempty"`
	ResolutionMode string           `json:"resolution_mode,omitempty"`
	Complete       bool             `json:"complete"`
	Warnings       []string         `json:"warnings,omitempty"`
	Indexed        bool             `json:"indexed,omitempty"`
	Fresh          bool             `json:"fresh,omitempty"`
}

type ReferenceOptions struct {
	Scope              Scope `json:"scope,omitempty"`
	IncludeDeclaration bool  `json:"include_declaration,omitempty"`
	MaxResults         int   `json:"max_results,omitempty"`
}

type ImportDirection string

const (
	ImportDirectionDirect  ImportDirection = "direct"
	ImportDirectionReverse ImportDirection = "reverse"
	ImportDirectionBoth    ImportDirection = "both"
)

type ImportQuery struct {
	// Scope constrains the files indexed before import filtering is applied.
	Scope Scope `json:"scope,omitempty"`
	// Path selects the source file or directory for direct import queries.
	Path string `json:"path,omitempty"`
	// PackageID selects the source package/unit (ImportEdge.FromUnit) for direct import queries.
	PackageID string `json:"package_id,omitempty"`
	// ImportPath selects the target import path for direct and reverse import queries.
	ImportPath  string          `json:"import_path,omitempty"`
	Direction   ImportDirection `json:"direction,omitempty"`
	MaxResults  int             `json:"max_results,omitempty"`
	IncludeTest *bool           `json:"include_test,omitempty"`
}

type ImportResult struct {
	DirectImports    []ImportEdge `json:"direct_imports,omitempty"`
	ReverseImporters []ImportEdge `json:"reverse_importers,omitempty"`
	TargetImportPath string       `json:"target_import_path,omitempty"`
	Diagnostics      []Diagnostic `json:"diagnostics,omitempty"`
	ResolutionMode   string       `json:"resolution_mode,omitempty"`
	Complete         bool         `json:"complete"`
	Warnings         []string     `json:"warnings,omitempty"`
	Indexed          bool         `json:"indexed,omitempty"`
	Fresh            bool         `json:"fresh,omitempty"`
}

type PackageResult struct {
	Packages    []PackageInfo `json:"packages,omitempty"`
	Diagnostics []Diagnostic  `json:"diagnostics,omitempty"`
	Indexed     bool          `json:"indexed,omitempty"`
	Fresh       bool          `json:"fresh,omitempty"`
}

type Outline struct {
	Documents   []Document   `json:"documents,omitempty"`
	Symbols     []Symbol     `json:"symbols,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type SourceFragment struct {
	Symbol   Symbol       `json:"symbol"`
	Source   string       `json:"source,omitempty"`
	Comments string       `json:"comments,omitempty"`
	Imports  []ImportEdge `json:"imports,omitempty"`
	Hash     string       `json:"hash,omitempty"`
}

type ImportEdge struct {
	FromUnit string   `json:"from_unit,omitempty"`
	FromPath string   `json:"from_path,omitempty"`
	Import   string   `json:"import"`
	Alias    string   `json:"alias,omitempty"`
	Location Location `json:"location,omitempty"`
}

type CallEdge struct {
	CallerID string   `json:"caller_id,omitempty"`
	CalleeID string   `json:"callee_id,omitempty"`
	Caller   Symbol   `json:"caller,omitempty"`
	Callee   Symbol   `json:"callee,omitempty"`
	Name     string   `json:"name,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Location Location `json:"location,omitempty"`
	Preview  string   `json:"preview,omitempty"`
}

type Implementation struct {
	Interface Symbol     `json:"interface"`
	Concrete  Symbol     `json:"concrete"`
	Location  Location   `json:"location,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

type UnitMetrics struct {
	UnitID              string     `json:"unit_id"`
	DirectFanIn         int        `json:"direct_fan_in,omitempty"`
	DirectFanOut        int        `json:"direct_fan_out,omitempty"`
	SymbolFanIn         int        `json:"symbol_fan_in,omitempty"`
	SymbolFanOut        int        `json:"symbol_fan_out,omitempty"`
	CallFanIn           int        `json:"call_fan_in,omitempty"`
	CallFanOut          int        `json:"call_fan_out,omitempty"`
	InterfaceCount      int        `json:"interface_count,omitempty"`
	ImplementationCount int        `json:"implementation_count,omitempty"`
	PublicSymbolCount   int        `json:"public_symbol_count,omitempty"`
	FileCount           int        `json:"file_count,omitempty"`
	LOC                 int        `json:"loc,omitempty"`
	PressureScore       float64    `json:"pressure_score,omitempty"`
	Evidence            []Evidence `json:"evidence,omitempty"`
}

type SymbolMetrics struct {
	SymbolID            SymbolID   `json:"symbol_id,omitempty"`
	UnitID              string     `json:"unit_id,omitempty"`
	Kind                SymbolKind `json:"kind,omitempty"`
	Name                string     `json:"name,omitempty"`
	QualifiedName       string     `json:"qualified_name,omitempty"`
	Location            Location   `json:"location,omitempty"`
	ReferenceCount      int        `json:"reference_count,omitempty"`
	CallFanIn           int        `json:"call_fan_in,omitempty"`
	CallFanOut          int        `json:"call_fan_out,omitempty"`
	ImplementationCount int        `json:"implementation_count,omitempty"`
	PressureScore       float64    `json:"pressure_score,omitempty"`
	Evidence            []Evidence `json:"evidence,omitempty"`
}

type Metrics struct {
	Units       []UnitMetrics   `json:"units,omitempty"`
	Symbols     []SymbolMetrics `json:"symbols,omitempty"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type ChangedFile struct {
	Path   string `json:"path"`
	Before []byte `json:"before,omitempty"`
	After  []byte `json:"after,omitempty"`
}

type Operation interface {
	Kind() OperationKind
}

type OperationKind string

const (
	OpRenameSymbol              OperationKind = "rename_symbol"
	OpReplaceSymbol             OperationKind = "replace_symbol"
	OpDeleteSymbol              OperationKind = "delete_symbol"
	OpReadSymbol                OperationKind = "read_symbol"
	OpAppendSymbol              OperationKind = "append_symbol"
	OpReplaceFunction           OperationKind = "replace_function"
	OpAppendFunction            OperationKind = "append_function"
	OpDeleteFunction            OperationKind = "delete_function"
	OpReplaceMethod             OperationKind = "replace_method"
	OpDeleteMethod              OperationKind = "delete_method"
	OpReplaceComment            OperationKind = "replace_comment"
	OpEnsureStructTag           OperationKind = "go_struct_tag_ensure"
	OpRemoveStructTag           OperationKind = "go_struct_tag_remove"
	OpEnsureGoImport            OperationKind = "go_import_ensure"
	OpRemoveGoImport            OperationKind = "go_import_remove"
	OpRenameGoImport            OperationKind = "go_import_rename"
	OpMoveSymbol                OperationKind = "move_symbol"
	OpAddGoParameter            OperationKind = "go_parameter_add"
	OpRemoveGoParam             OperationKind = "go_parameter_remove"
	OpRenameGoParam             OperationKind = "go_parameter_rename"
	OpAddGoField                OperationKind = "go_struct_field_add"
	OpRemoveGoField             OperationKind = "go_struct_field_remove"
	OpRenameGoField             OperationKind = "go_struct_field_rename"
	OpChangeGoParam             OperationKind = "go_parameter_type_change"
	OpChangeGoResult            OperationKind = "go_result_type_change"
	OpRenameGoRecv              OperationKind = "go_receiver_rename"
	OpAddGoIfaceMeth            OperationKind = "go_interface_method_add"
	OpRemoveGoIface             OperationKind = "go_interface_method_remove"
	OpExtractGoFunc             OperationKind = "go_function_extract"
	OpExtractGoMethod           OperationKind = "go_method_extract"
	OpMarkdownEnsureH1          OperationKind = "markdown_h1_ensure"
	OpMarkdownSetHeadingLevel   OperationKind = "markdown_heading_level_set"
	OpMarkdownInsertSectionBody OperationKind = "markdown_section_body_insert"
	OpMarkdownRenameHeading     OperationKind = "markdown_heading_rename"
)

type TextEdit struct {
	Path        string `json:"path"`
	Range       Range  `json:"range"`
	Replacement string `json:"replacement"`
}

type FileEdit struct {
	Path  string     `json:"path"`
	Edits []TextEdit `json:"edits,omitempty"`
}

type ReplaceFunction struct {
	Target       SymbolSelector
	Source       string
	ExpectedHash string
}

func (ReplaceFunction) Kind() OperationKind { return OpReplaceFunction }

type ReplaceSymbol struct {
	Target       SymbolSelector
	Source       string
	ExpectedHash string
}

func (ReplaceSymbol) Kind() OperationKind { return OpReplaceSymbol }

type RenameSymbol struct {
	Target       SymbolSelector
	NewName      string
	ExpectedHash string
}

func (RenameSymbol) Kind() OperationKind { return OpRenameSymbol }

type AppendFunction struct {
	Path   string
	UnitID string
	Source string
}

func (AppendFunction) Kind() OperationKind { return OpAppendFunction }

type AppendSymbol struct {
	Path   string
	UnitID string
	Source string
}

func (AppendSymbol) Kind() OperationKind { return OpAppendSymbol }

type DeleteSymbol struct {
	Target       SymbolSelector
	ExpectedHash string
}

func (DeleteSymbol) Kind() OperationKind { return OpDeleteSymbol }

type DeleteFunction struct {
	Target       SymbolSelector
	ExpectedHash string
}

func (DeleteFunction) Kind() OperationKind { return OpDeleteFunction }

type ReplaceMethod struct {
	Target       SymbolSelector
	Source       string
	ExpectedHash string
}

func (ReplaceMethod) Kind() OperationKind { return OpReplaceMethod }

type DeleteMethod struct {
	Target       SymbolSelector
	ExpectedHash string
}

func (DeleteMethod) Kind() OperationKind { return OpDeleteMethod }

type ReplaceComment struct {
	Target       SymbolSelector
	Text         string
	Style        string
	ExpectedHash string
}

func (ReplaceComment) Kind() OperationKind { return OpReplaceComment }

type EnsureGoStructTag struct {
	Struct  SymbolSelector
	Field   string
	Key     string
	Value   string
	Options []string
}

func (EnsureGoStructTag) Kind() OperationKind { return OpEnsureStructTag }

type RemoveGoStructTag struct {
	Struct SymbolSelector
	Field  string
	Key    string
}

func (RemoveGoStructTag) Kind() OperationKind { return OpRemoveStructTag }

type EnsureGoImport struct {
	Path       string
	UnitID     string
	ImportPath string
	Alias      string
}

func (EnsureGoImport) Kind() OperationKind { return OpEnsureGoImport }

type RemoveGoImport struct {
	Path       string
	UnitID     string
	ImportPath string
	Alias      string
}

func (RemoveGoImport) Kind() OperationKind { return OpRemoveGoImport }

type RenameGoImport struct {
	Path       string
	UnitID     string
	ImportPath string
	Alias      string
}

func (RenameGoImport) Kind() OperationKind { return OpRenameGoImport }

type MoveSymbol struct {
	Target           SymbolSelector
	ToPath           string
	ExpectedHash     string
	ReconcileImports bool
}

func (MoveSymbol) Kind() OperationKind { return OpMoveSymbol }

type AddGoParameter struct {
	Target       SymbolSelector
	Name         string
	Type         string
	DefaultValue string
	Position     int
	ExpectedHash string
}

func (AddGoParameter) Kind() OperationKind { return OpAddGoParameter }

type RemoveGoParameter struct {
	Target       SymbolSelector
	Name         string
	ExpectedHash string
}

func (RemoveGoParameter) Kind() OperationKind { return OpRemoveGoParam }

type RenameGoParameter struct {
	Target       SymbolSelector
	OldName      string
	NewName      string
	ExpectedHash string
}

func (RenameGoParameter) Kind() OperationKind { return OpRenameGoParam }

type AddGoStructField struct {
	Struct       SymbolSelector
	Name         string
	Type         string
	Tag          string
	Comment      string
	Position     int
	ExpectedHash string
}

func (AddGoStructField) Kind() OperationKind { return OpAddGoField }

type RemoveGoStructField struct {
	Struct       SymbolSelector
	Field        string
	ExpectedHash string
}

func (RemoveGoStructField) Kind() OperationKind { return OpRemoveGoField }

type RenameGoStructField struct {
	Struct          SymbolSelector
	OldName         string
	NewName         string
	UpdateSelectors bool
	ExpectedHash    string
}

func (RenameGoStructField) Kind() OperationKind { return OpRenameGoField }

type ChangeGoParameterType struct {
	Target       SymbolSelector
	Name         string
	Type         string
	ExpectedHash string
}

func (ChangeGoParameterType) Kind() OperationKind { return OpChangeGoParam }

type ChangeGoResultType struct {
	Target       SymbolSelector
	Name         string
	Position     int
	Type         string
	ExpectedHash string
}

func (ChangeGoResultType) Kind() OperationKind { return OpChangeGoResult }

type RenameGoReceiver struct {
	Target       SymbolSelector
	NewName      string
	ExpectedHash string
}

func (RenameGoReceiver) Kind() OperationKind { return OpRenameGoRecv }

type AddGoInterfaceMethod struct {
	Interface    SymbolSelector
	Method       string
	Position     int
	ExpectedHash string
}

func (AddGoInterfaceMethod) Kind() OperationKind { return OpAddGoIfaceMeth }

type RemoveGoInterfaceMethod struct {
	Interface    SymbolSelector
	Method       string
	ExpectedHash string
}

func (RemoveGoInterfaceMethod) Kind() OperationKind { return OpRemoveGoIface }

type ExtractGoFunction struct {
	Path            string
	Range           Range
	Name            string
	Params          string
	Results         string
	InsertAfter     SymbolSelector
	ReplaceWithCall string
}

func (ExtractGoFunction) Kind() OperationKind { return OpExtractGoFunc }

type ExtractGoMethod struct {
	Path            string
	Range           Range
	Receiver        string
	Name            string
	Params          string
	Results         string
	InsertAfter     SymbolSelector
	ReplaceWithCall string
}

func (ExtractGoMethod) Kind() OperationKind { return OpExtractGoMethod }

type EnsureMarkdownH1 struct {
	Path  string
	Title string
}

func (EnsureMarkdownH1) Kind() OperationKind { return OpMarkdownEnsureH1 }

type SetMarkdownHeadingLevel struct {
	Path   string
	Offset int
	Level  int
}

func (SetMarkdownHeadingLevel) Kind() OperationKind { return OpMarkdownSetHeadingLevel }

type InsertMarkdownSectionBody struct {
	Path   string
	Offset int
	Text   string
}

func (InsertMarkdownSectionBody) Kind() OperationKind { return OpMarkdownInsertSectionBody }

type RenameMarkdownHeading struct {
	Path    string
	Offset  int
	NewText string
}

func (RenameMarkdownHeading) Kind() OperationKind { return OpMarkdownRenameHeading }

type RefactorKind string

const (
	RefactorDeleteSymbol         RefactorKind = "delete_symbol"
	RefactorExtractFunction      RefactorKind = "extract_function"
	RefactorIntroduceConfig      RefactorKind = "introduce_config"
	RefactorSplitFunction        RefactorKind = "split_function"
	RefactorSplitPackage         RefactorKind = "split_package"
	RefactorReplaceFlagArgument  RefactorKind = "replace_flag_argument"
	RefactorFixMarkdownStructure RefactorKind = "fix_markdown_structure"
	RefactorReviewDebtMarkers    RefactorKind = "review_debt_markers"
)

type Confidence string

const (
	LowConfidence    Confidence = "low"
	MediumConfidence Confidence = "medium"
	HighConfidence   Confidence = "high"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Proposal struct {
	ID         string             `json:"id"`
	Kind       RefactorKind       `json:"kind"`
	Title      string             `json:"title"`
	Summary    string             `json:"summary,omitempty"`
	Confidence Confidence         `json:"confidence"`
	Risk       RiskLevel          `json:"risk"`
	Targets    []Symbol           `json:"targets,omitempty"`
	Evidence   []Evidence         `json:"evidence,omitempty"`
	Operations []Operation        `json:"operations,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
}

type SuggestOption func(*SuggestOptions)

type SuggestOptions struct {
	Scope Scope
}

func WithSuggestScope(scope Scope) SuggestOption {
	return func(opts *SuggestOptions) { opts.Scope = scope }
}
