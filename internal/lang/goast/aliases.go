package goast

import "github.com/codewandler/editor/internal/core"

type (
	BackendSpec             = core.BackendSpec
	Snapshot                = core.Snapshot
	Index                   = core.Index
	Document                = core.Document
	PackageInfo             = core.PackageInfo
	Position                = core.Position
	Range                   = core.Range
	Location                = core.Location
	SymbolID                = core.SymbolID
	SymbolKind              = core.SymbolKind
	Symbol                  = core.Symbol
	OccurrenceKind          = core.OccurrenceKind
	Occurrence              = core.Occurrence
	Edge                    = core.Edge
	Diagnostic              = core.Diagnostic
	Evidence                = core.Evidence
	Scope                   = core.Scope
	SymbolSelector          = core.SymbolSelector
	BackendInfo             = core.BackendInfo
	ImportEdge              = core.ImportEdge
	ImportDirection         = core.ImportDirection
	ImportQuery             = core.ImportQuery
	ImportResult            = core.ImportResult
	PackageResult           = core.PackageResult
	UnitMetrics             = core.UnitMetrics
	SymbolMetrics           = core.SymbolMetrics
	Operation               = core.Operation
	OperationKind           = core.OperationKind
	FileEdit                = core.FileEdit
	TextEdit                = core.TextEdit
	ReplaceFunction         = core.ReplaceFunction
	ReplaceSymbol           = core.ReplaceSymbol
	RenameSymbol            = core.RenameSymbol
	AppendFunction          = core.AppendFunction
	AppendSymbol            = core.AppendSymbol
	DeleteSymbol            = core.DeleteSymbol
	DeleteFunction          = core.DeleteFunction
	ReplaceMethod           = core.ReplaceMethod
	DeleteMethod            = core.DeleteMethod
	ReplaceComment          = core.ReplaceComment
	EnsureGoStructTag       = core.EnsureGoStructTag
	RemoveGoStructTag       = core.RemoveGoStructTag
	EnsureGoImport          = core.EnsureGoImport
	RemoveGoImport          = core.RemoveGoImport
	RenameGoImport          = core.RenameGoImport
	MoveSymbol              = core.MoveSymbol
	AddGoParameter          = core.AddGoParameter
	RemoveGoParameter       = core.RemoveGoParameter
	RenameGoParameter       = core.RenameGoParameter
	AddGoStructField        = core.AddGoStructField
	RemoveGoStructField     = core.RemoveGoStructField
	RenameGoStructField     = core.RenameGoStructField
	ChangeGoParameterType   = core.ChangeGoParameterType
	ChangeGoResultType      = core.ChangeGoResultType
	RenameGoReceiver        = core.RenameGoReceiver
	AddGoInterfaceMethod    = core.AddGoInterfaceMethod
	RemoveGoInterfaceMethod = core.RemoveGoInterfaceMethod
	ExtractGoFunction       = core.ExtractGoFunction
	ExtractGoMethod         = core.ExtractGoMethod
	Proposal                = core.Proposal
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
	SymbolImport    = core.SymbolImport
)

const (
	OccurrenceDeclaration = core.OccurrenceDeclaration
	OccurrenceReference   = core.OccurrenceReference
	OccurrenceRead        = core.OccurrenceRead
	OccurrenceWrite       = core.OccurrenceWrite
	OccurrenceCall        = core.OccurrenceCall
	OccurrenceImport      = core.OccurrenceImport
	OccurrenceDoc         = core.OccurrenceDoc
)

const (
	EdgeContains   = core.EdgeContains
	EdgeReferences = core.EdgeReferences
	EdgeCalls      = core.EdgeCalls
	EdgeImports    = core.EdgeImports
	EdgeImplements = core.EdgeImplements
)

const (
	RefactorDeleteSymbol        = core.RefactorDeleteSymbol
	RefactorExtractFunction     = core.RefactorExtractFunction
	RefactorIntroduceConfig     = core.RefactorIntroduceConfig
	RefactorSplitFunction       = core.RefactorSplitFunction
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
	OpRenameSymbol    = core.OpRenameSymbol
	OpReplaceFunction = core.OpReplaceFunction
	OpAppendFunction  = core.OpAppendFunction
	OpDeleteSymbol    = core.OpDeleteSymbol
	OpReplaceComment  = core.OpReplaceComment
	OpEnsureStructTag = core.OpEnsureStructTag
	OpRemoveStructTag = core.OpRemoveStructTag
	OpEnsureGoImport  = core.OpEnsureGoImport
	OpRemoveGoImport  = core.OpRemoveGoImport
	OpRenameGoImport  = core.OpRenameGoImport
	OpMoveSymbol      = core.OpMoveSymbol
	OpAddGoParameter  = core.OpAddGoParameter
	OpRemoveGoParam   = core.OpRemoveGoParam
	OpRenameGoParam   = core.OpRenameGoParam
	OpAddGoField      = core.OpAddGoField
	OpRemoveGoField   = core.OpRemoveGoField
	OpRenameGoField   = core.OpRenameGoField
	OpChangeGoParam   = core.OpChangeGoParam
	OpChangeGoResult  = core.OpChangeGoResult
	OpRenameGoRecv    = core.OpRenameGoRecv
	OpAddGoIfaceMeth  = core.OpAddGoIfaceMeth
	OpRemoveGoIface   = core.OpRemoveGoIface
	OpExtractGoFunc   = core.OpExtractGoFunc
	OpExtractGoMethod = core.OpExtractGoMethod
)
