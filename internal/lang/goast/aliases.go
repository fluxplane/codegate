package goast

import "github.com/codewandler/editor/internal/core"

type (
	BackendSpec       = core.BackendSpec
	Snapshot          = core.Snapshot
	Index             = core.Index
	Document          = core.Document
	PackageInfo       = core.PackageInfo
	Position          = core.Position
	Range             = core.Range
	Location          = core.Location
	SymbolID          = core.SymbolID
	SymbolKind        = core.SymbolKind
	Symbol            = core.Symbol
	Occurrence        = core.Occurrence
	Edge              = core.Edge
	Diagnostic        = core.Diagnostic
	Evidence          = core.Evidence
	Scope             = core.Scope
	SymbolSelector    = core.SymbolSelector
	BackendInfo       = core.BackendInfo
	ImportEdge        = core.ImportEdge
	ImportDirection   = core.ImportDirection
	ImportQuery       = core.ImportQuery
	ImportResult      = core.ImportResult
	PackageResult     = core.PackageResult
	UnitMetrics       = core.UnitMetrics
	Operation         = core.Operation
	OperationKind     = core.OperationKind
	FileEdit          = core.FileEdit
	TextEdit          = core.TextEdit
	ReplaceFunction   = core.ReplaceFunction
	AppendFunction    = core.AppendFunction
	DeleteSymbol      = core.DeleteSymbol
	ReplaceComment    = core.ReplaceComment
	EnsureGoStructTag = core.EnsureGoStructTag
	RemoveGoStructTag = core.RemoveGoStructTag
	Proposal          = core.Proposal
)

const Go = core.Go

const (
	SymbolPackage   = core.SymbolPackage
	SymbolType      = core.SymbolType
	SymbolStruct    = core.SymbolStruct
	SymbolInterface = core.SymbolInterface
	SymbolFunction  = core.SymbolFunction
	SymbolMethod    = core.SymbolMethod
	SymbolField     = core.SymbolField
	SymbolConst     = core.SymbolConst
	SymbolVar       = core.SymbolVar
)

const (
	OccurrenceDeclaration = core.OccurrenceDeclaration
	OccurrenceReference   = core.OccurrenceReference
	OccurrenceCall        = core.OccurrenceCall
)

const (
	EdgeContains   = core.EdgeContains
	EdgeCalls      = core.EdgeCalls
	EdgeImports    = core.EdgeImports
	EdgeImplements = core.EdgeImplements
)

const (
	RefactorDeleteSymbol        = core.RefactorDeleteSymbol
	RefactorExtractFunction     = core.RefactorExtractFunction
	RefactorIntroduceConfig     = core.RefactorIntroduceConfig
	RefactorSplitPackage        = core.RefactorSplitPackage
	RefactorReplaceFlagArgument = core.RefactorReplaceFlagArgument
)

const (
	LowConfidence    = core.LowConfidence
	MediumConfidence = core.MediumConfidence
	RiskLow          = core.RiskLow
	RiskMedium       = core.RiskMedium
	RiskHigh         = core.RiskHigh
)

const (
	OpReplaceFunction = core.OpReplaceFunction
	OpAppendFunction  = core.OpAppendFunction
	OpDeleteSymbol    = core.OpDeleteSymbol
	OpReplaceComment  = core.OpReplaceComment
	OpEnsureStructTag = core.OpEnsureStructTag
	OpRemoveStructTag = core.OpRemoveStructTag
)
