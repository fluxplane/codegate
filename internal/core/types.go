package core

import "context"

type LanguageID string

const Go LanguageID = "go"

type BackendInfo struct {
	Language       LanguageID
	Name           string
	ResolutionMode string
	Complete       bool
	Diagnostics    []Diagnostic
	Metadata       map[string]string
}

type BackendSpec struct {
	Language       LanguageID
	Name           string
	FileExtensions []string
	Capabilities   []string
	ResolutionMode string
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
	Documents   []Document
	Packages    []PackageInfo
	Symbols     []Symbol
	Occurrences []Occurrence
	Edges       []Edge
	Imports     []ImportEdge
	Diagnostics []Diagnostic
	ByID        map[SymbolID]Symbol
	ByName      map[string][]Symbol
	UnitFiles   map[string][]string
	FileUnits   map[string]string
	FileLOC     map[string]int
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
	URI      string
	Language LanguageID
	UnitID   string
	Version  string
}

type PackageInfo struct {
	ID          string
	Name        string
	Dir         string
	Files       []string
	Diagnostics []Diagnostic
}

type Position struct {
	Line   int
	Column int
	Offset int
}

type Range struct {
	Start Position
	End   Position
}

type Location struct {
	URI   string
	Range Range
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
	ID             SymbolID
	Language       LanguageID
	Kind           SymbolKind
	Name           string
	QualifiedName  string
	ContainerID    SymbolID
	ContainerName  string
	UnitID         string
	Location       Location
	SelectionRange Range
	BodyRange      Range
	Signature      string
	Doc            string
	Tags           map[string]string
	Children       []Symbol
	Backend        BackendInfo
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
	SymbolID SymbolID
	Kind     OccurrenceKind
	Name     string
	Location Location
	Preview  string
	Evidence []Evidence
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
	Kind     EdgeKind
	From     string
	To       string
	Location Location
	Weight   int
	Evidence []Evidence
}

type Diagnostic struct {
	Location Location
	Severity string
	Message  string
}

type Evidence struct {
	Kind     string
	Message  string
	Location Location
	Metrics  map[string]float64
}

type Scope struct {
	Root         string
	Path         string
	UnitID       string
	Language     LanguageID
	IncludeTests bool
	MaxFiles     int
	MaxBytes     int64
}

type SymbolSelector struct {
	ID            SymbolID
	Language      LanguageID
	Name          string
	QualifiedName string
	Kind          SymbolKind
	Container     string
	UnitID        string
	Path          string
	IncludeTests  *bool
}

type PositionSelector struct {
	Path   string
	Line   int
	Column int
	Offset *int
}

type ResolveResult struct {
	Matches     []Symbol
	Ambiguous   bool
	Diagnostics []Diagnostic
}

type NavigationTarget struct {
	Text            string
	Name            string
	NodeKind        string
	PackageID       string
	Location        Location
	EnclosingSymbol *Symbol
}

type NavigationOptions struct {
	Scope             Scope
	IncludeDocs       bool
	FallbackEnclosing bool
	MaxResults        int
}

type NavigationResult struct {
	Target         NavigationTarget
	Symbols        []Symbol
	Locations      []Location
	Diagnostics    []Diagnostic
	ResolutionMode string
	Complete       bool
	Warnings       []string
	Indexed        bool
	Fresh          bool
}

type ReferenceOptions struct {
	Scope              Scope
	IncludeDeclaration bool
	MaxResults         int
}

type ImportDirection string

const (
	ImportDirectionDirect  ImportDirection = "direct"
	ImportDirectionReverse ImportDirection = "reverse"
	ImportDirectionBoth    ImportDirection = "both"
)

type ImportQuery struct {
	// Scope constrains the files indexed before import filtering is applied.
	Scope Scope
	// Path selects the source file or directory for direct import queries.
	Path string
	// PackageID selects the source package/unit (ImportEdge.FromUnit) for direct import queries.
	PackageID string
	// ImportPath selects the target import path for direct and reverse import queries.
	ImportPath  string
	Direction   ImportDirection
	MaxResults  int
	IncludeTest *bool
}

type ImportResult struct {
	DirectImports    []ImportEdge
	ReverseImporters []ImportEdge
	TargetImportPath string
	Diagnostics      []Diagnostic
	ResolutionMode   string
	Complete         bool
	Warnings         []string
	Indexed          bool
	Fresh            bool
}

type PackageResult struct {
	Packages    []PackageInfo
	Diagnostics []Diagnostic
	Indexed     bool
	Fresh       bool
}

type Outline struct {
	Documents   []Document
	Symbols     []Symbol
	Diagnostics []Diagnostic
}

type SourceFragment struct {
	Symbol   Symbol
	Source   string
	Comments string
	Imports  []ImportEdge
	Hash     string
}

type ImportEdge struct {
	FromUnit string
	FromPath string
	Import   string
	Alias    string
	Location Location
}

type CallEdge struct {
	CallerID string
	CalleeID string
	Caller   Symbol
	Callee   Symbol
	Name     string
	Kind     string
	Location Location
	Preview  string
}

type Implementation struct {
	Interface Symbol
	Concrete  Symbol
	Location  Location
	Evidence  []Evidence
}

type UnitMetrics struct {
	UnitID              string
	DirectFanIn         int
	DirectFanOut        int
	SymbolFanIn         int
	SymbolFanOut        int
	CallFanIn           int
	CallFanOut          int
	InterfaceCount      int
	ImplementationCount int
	PublicSymbolCount   int
	FileCount           int
	LOC                 int
	PressureScore       float64
	Evidence            []Evidence
}

type SymbolMetrics struct {
	SymbolID            SymbolID
	UnitID              string
	Kind                SymbolKind
	Name                string
	QualifiedName       string
	Location            Location
	ReferenceCount      int
	CallFanIn           int
	CallFanOut          int
	ImplementationCount int
	PressureScore       float64
	Evidence            []Evidence
}

type Metrics struct {
	Units       []UnitMetrics
	Symbols     []SymbolMetrics
	Diagnostics []Diagnostic
}

type ChangedFile struct {
	Path   string
	Before []byte
	After  []byte
}

type Operation interface {
	Kind() OperationKind
}

type OperationKind string

const (
	OpRenameSymbol    OperationKind = "rename_symbol"
	OpReplaceSymbol   OperationKind = "replace_symbol"
	OpDeleteSymbol    OperationKind = "delete_symbol"
	OpReadSymbol      OperationKind = "read_symbol"
	OpAppendSymbol    OperationKind = "append_symbol"
	OpReplaceFunction OperationKind = "replace_function"
	OpAppendFunction  OperationKind = "append_function"
	OpDeleteFunction  OperationKind = "delete_function"
	OpReplaceMethod   OperationKind = "replace_method"
	OpDeleteMethod    OperationKind = "delete_method"
	OpReplaceComment  OperationKind = "replace_comment"
	OpEnsureStructTag OperationKind = "go_struct_tag_ensure"
	OpRemoveStructTag OperationKind = "go_struct_tag_remove"
)

type TextEdit struct {
	Path        string
	Range       Range
	Replacement string
}

type FileEdit struct {
	Path  string
	Edits []TextEdit
}

type ReplaceFunction struct {
	Target       SymbolSelector
	Source       string
	ExpectedHash string
}

func (ReplaceFunction) Kind() OperationKind { return OpReplaceFunction }

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

type DeleteSymbol struct {
	Target       SymbolSelector
	ExpectedHash string
}

func (DeleteSymbol) Kind() OperationKind { return OpDeleteSymbol }

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

type RefactorKind string

const (
	RefactorDeleteSymbol        RefactorKind = "delete_symbol"
	RefactorExtractFunction     RefactorKind = "extract_function"
	RefactorIntroduceConfig     RefactorKind = "introduce_config"
	RefactorSplitFunction       RefactorKind = "split_function"
	RefactorSplitPackage        RefactorKind = "split_package"
	RefactorReplaceFlagArgument RefactorKind = "replace_flag_argument"
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
	ID         string
	Kind       RefactorKind
	Title      string
	Summary    string
	Confidence Confidence
	Risk       RiskLevel
	Targets    []Symbol
	Evidence   []Evidence
	Operations []Operation
	Metrics    map[string]float64
}

type SuggestOption func(*SuggestOptions)

type SuggestOptions struct {
	Scope Scope
}

func WithSuggestScope(scope Scope) SuggestOption {
	return func(opts *SuggestOptions) { opts.Scope = scope }
}
