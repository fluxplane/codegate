package codegate

import "github.com/fluxplane/codegate/internal/core"

// Core model aliases expose the shared codegate data model from the public
// package while keeping the implementation types in internal/core.
type (
	LanguageID        = core.LanguageID
	BackendInfo       = core.BackendInfo
	BackendSpec       = core.BackendSpec
	Capability        = core.Capability
	CapabilityLevel   = core.CapabilityLevel
	CapabilitySupport = core.CapabilitySupport
	OperationSupport  = core.OperationSupport
	AssessmentSupport = core.AssessmentSupport
	MetricSupport     = core.MetricSupport
	FindingSupport    = core.FindingSupport
	ViolationSupport  = core.ViolationSupport
	Backend           = core.Backend
	Source            = core.Snapshot
	Document          = core.Document
	PackageInfo       = core.PackageInfo
	Position          = core.Position
	Range             = core.Range
	Location          = core.Location
	SymbolID          = core.SymbolID
	SymbolKind        = core.SymbolKind
	Symbol            = core.Symbol
	OccurrenceKind    = core.OccurrenceKind
	Occurrence        = core.Occurrence
	EdgeKind          = core.EdgeKind
	Edge              = core.Edge
	Diagnostic        = core.Diagnostic
	ValidationKind    = core.ValidationKind
	ValidationOptions = core.ValidationOptions
	ValidationResult  = core.ValidationResult
	Validator         = core.Validator
	ValidationAdapter = core.ValidationAdapter
	Evidence          = core.Evidence
	Scope             = core.Scope
	SymbolSelector    = core.SymbolSelector
	PositionSelector  = core.PositionSelector
	ResolveResult     = core.ResolveResult
	NavigationTarget  = core.NavigationTarget
	NavigationOptions = core.NavigationOptions
	NavigationResult  = core.NavigationResult
	ReferenceOptions  = core.ReferenceOptions
	ImportDirection   = core.ImportDirection
	ImportQuery       = core.ImportQuery
	ImportResult      = core.ImportResult
	PackageResult     = core.PackageResult
	Outline           = core.Outline
	SourceFragment    = core.SourceFragment
	ImportEdge        = core.ImportEdge
	CallEdge          = core.CallEdge
	Implementation    = core.Implementation
	UnitMetrics       = core.UnitMetrics
	SymbolMetrics     = core.SymbolMetrics
	Metrics           = core.Metrics
	ChangedFile       = core.ChangedFile
)

// Validation kind aliases identify the validation passes a backend or adapter
// can perform.
const (
	ValidationParse     = core.ValidationParse
	ValidationTypecheck = core.ValidationTypecheck
	ValidationExternal  = core.ValidationExternal
)

// Import direction aliases select direct, reverse, or combined dependency
// queries.
const (
	ImportDirectionDirect  = core.ImportDirectionDirect
	ImportDirectionReverse = core.ImportDirectionReverse
	ImportDirectionBoth    = core.ImportDirectionBoth
)

// Language aliases identify the language backend used for a request.
const (
	Go       = core.Go
	Markdown = core.Markdown
)

// Capability aliases describe backend feature categories.
const (
	CapabilityLookup         = core.CapabilityLookup
	CapabilityStaticAnalysis = core.CapabilityStaticAnalysis
	CapabilityQuality        = core.CapabilityQuality
	CapabilityEditing        = core.CapabilityEditing
	CapabilityRefactoring    = core.CapabilityRefactoring
	CapabilityValidation     = core.CapabilityValidation
	CapabilityReporting      = core.CapabilityReporting
)

// Capability level aliases describe how complete a backend feature is.
const (
	CapabilityNone         = core.CapabilityNone
	CapabilityBasic        = core.CapabilityBasic
	CapabilityAdvanced     = core.CapabilityAdvanced
	CapabilityExperimental = core.CapabilityExperimental
)

// Symbol kind aliases classify indexed declarations and references.
const (
	SymbolModule      = core.SymbolModule
	SymbolPackage     = core.SymbolPackage
	SymbolNamespace   = core.SymbolNamespace
	SymbolFile        = core.SymbolFile
	SymbolType        = core.SymbolType
	SymbolClass       = core.SymbolClass
	SymbolStruct      = core.SymbolStruct
	SymbolInterface   = core.SymbolInterface
	SymbolEnum        = core.SymbolEnum
	SymbolFunction    = core.SymbolFunction
	SymbolMethod      = core.SymbolMethod
	SymbolConstructor = core.SymbolConstructor
	SymbolField       = core.SymbolField
	SymbolProperty    = core.SymbolProperty
	SymbolConst       = core.SymbolConst
	SymbolVar         = core.SymbolVar
	SymbolImport      = core.SymbolImport
	SymbolParameter   = core.SymbolParameter
	SymbolLocal       = core.SymbolLocal
)

// Occurrence kind aliases classify how a symbol appears at a location.
const (
	OccurrenceDeclaration = core.OccurrenceDeclaration
	OccurrenceDefinition  = core.OccurrenceDefinition
	OccurrenceReference   = core.OccurrenceReference
	OccurrenceRead        = core.OccurrenceRead
	OccurrenceWrite       = core.OccurrenceWrite
	OccurrenceCall        = core.OccurrenceCall
	OccurrenceImport      = core.OccurrenceImport
	OccurrenceImplement   = core.OccurrenceImplement
	OccurrenceDoc         = core.OccurrenceDoc
)

// Edge kind aliases classify relationships between indexed symbols.
const (
	EdgeContains   = core.EdgeContains
	EdgeDeclares   = core.EdgeDeclares
	EdgeReferences = core.EdgeReferences
	EdgeCalls      = core.EdgeCalls
	EdgeImports    = core.EdgeImports
	EdgeImplements = core.EdgeImplements
	EdgeOverrides  = core.EdgeOverrides
	EdgeEmbeds     = core.EdgeEmbeds
)
