package editor

import "github.com/codewandler/editor/internal/core"

type (
	LanguageID        = core.LanguageID
	BackendInfo       = core.BackendInfo
	BackendSpec       = core.BackendSpec
	Backend           = core.Backend
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
	Metrics           = core.Metrics
	ChangedFile       = core.ChangedFile
)

const (
	ImportDirectionDirect  = core.ImportDirectionDirect
	ImportDirectionReverse = core.ImportDirectionReverse
	ImportDirectionBoth    = core.ImportDirectionBoth
)

const Go = core.Go

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
