package goast

import "github.com/codewandler/codegate/internal/core"

type (
	BackendSpec                = core.BackendSpec
	Capability                 = core.Capability
	CapabilityLevel            = core.CapabilityLevel
	CapabilitySupport          = core.CapabilitySupport
	OperationSupport           = core.OperationSupport
	AssessmentSupport          = core.AssessmentSupport
	MetricSupport              = core.MetricSupport
	FindingSupport             = core.FindingSupport
	ViolationSupport           = core.ViolationSupport
	Snapshot                   = core.Snapshot
	Index                      = core.Index
	Document                   = core.Document
	PackageInfo                = core.PackageInfo
	Position                   = core.Position
	Range                      = core.Range
	Location                   = core.Location
	SymbolID                   = core.SymbolID
	SymbolKind                 = core.SymbolKind
	Symbol                     = core.Symbol
	OccurrenceKind             = core.OccurrenceKind
	Occurrence                 = core.Occurrence
	Edge                       = core.Edge
	Diagnostic                 = core.Diagnostic
	ValidationKind             = core.ValidationKind
	ValidationOptions          = core.ValidationOptions
	ValidationResult           = core.ValidationResult
	AssessmentGate             = core.AssessmentGate
	AssessmentOptions          = core.AssessmentOptions
	ArchitectureRuleAction     = core.ArchitectureRuleAction
	ArchitectureRules          = core.ArchitectureRules
	ArchitectureImportRule     = core.ArchitectureImportRule
	ArchitectureLayer          = core.ArchitectureLayer
	ArchitectureDependencyRule = core.ArchitectureDependencyRule
	ArchitectureEffectRule     = core.ArchitectureEffectRule
	ArchitectureScope          = core.ArchitectureScope
	ArchitectureCallRule       = core.ArchitectureCallRule
	ArchitectureCouplingRules  = core.ArchitectureCouplingRules
	ArchitecturePackageNote    = core.ArchitecturePackageNote
	ArchitectureException      = core.ArchitectureException
	AssessmentReport           = core.AssessmentReport
	AssessmentSummary          = core.AssessmentSummary
	ScoreSet                   = core.ScoreSet
	ValidationSummary          = core.ValidationSummary
	Finding                    = core.Finding
	Violation                  = core.Violation
	AssessmentSuggestion       = core.AssessmentSuggestion
	Evidence                   = core.Evidence
	Scope                      = core.Scope
	SymbolSelector             = core.SymbolSelector
	BackendInfo                = core.BackendInfo
	ImportEdge                 = core.ImportEdge
	ImportDirection            = core.ImportDirection
	ImportQuery                = core.ImportQuery
	ImportResult               = core.ImportResult
	PackageResult              = core.PackageResult
	UnitMetrics                = core.UnitMetrics
	SymbolMetrics              = core.SymbolMetrics
	Operation                  = core.Operation
	OperationKind              = core.OperationKind
	RefactorKind               = core.RefactorKind
	FileEdit                   = core.FileEdit
	TextEdit                   = core.TextEdit
	ReplaceFunction            = core.ReplaceFunction
	ReplaceSymbol              = core.ReplaceSymbol
	RenameSymbol               = core.RenameSymbol
	AppendFunction             = core.AppendFunction
	AppendSymbol               = core.AppendSymbol
	DeleteSymbol               = core.DeleteSymbol
	DeleteFunction             = core.DeleteFunction
	ReplaceMethod              = core.ReplaceMethod
	DeleteMethod               = core.DeleteMethod
	ReplaceComment             = core.ReplaceComment
	EnsureGoStructTag          = core.EnsureGoStructTag
	RemoveGoStructTag          = core.RemoveGoStructTag
	EnsureGoImport             = core.EnsureGoImport
	RemoveGoImport             = core.RemoveGoImport
	RenameGoImport             = core.RenameGoImport
	MoveSymbol                 = core.MoveSymbol
	AddGoParameter             = core.AddGoParameter
	RemoveGoParameter          = core.RemoveGoParameter
	RenameGoParameter          = core.RenameGoParameter
	AddGoStructField           = core.AddGoStructField
	RemoveGoStructField        = core.RemoveGoStructField
	RenameGoStructField        = core.RenameGoStructField
	ChangeGoParameterType      = core.ChangeGoParameterType
	ChangeGoResultType         = core.ChangeGoResultType
	RenameGoReceiver           = core.RenameGoReceiver
	AddGoInterfaceMethod       = core.AddGoInterfaceMethod
	RemoveGoInterfaceMethod    = core.RemoveGoInterfaceMethod
	ExtractGoFunction          = core.ExtractGoFunction
	ExtractGoMethod            = core.ExtractGoMethod
	Proposal                   = core.Proposal
)

const Go = core.Go

const (
	CapabilityLookup         = core.CapabilityLookup
	CapabilityStaticAnalysis = core.CapabilityStaticAnalysis
	CapabilityQuality        = core.CapabilityQuality
	CapabilityEditing        = core.CapabilityEditing
	CapabilityRefactoring    = core.CapabilityRefactoring
	CapabilityValidation     = core.CapabilityValidation
	CapabilityReporting      = core.CapabilityReporting
)

const (
	CapabilityNone         = core.CapabilityNone
	CapabilityBasic        = core.CapabilityBasic
	CapabilityAdvanced     = core.CapabilityAdvanced
	CapabilityExperimental = core.CapabilityExperimental
)

const (
	ValidationParse     = core.ValidationParse
	ValidationTypecheck = core.ValidationTypecheck
)

const (
	AssessmentGateAll             = core.AssessmentGateAll
	AssessmentGateArchitecture    = core.AssessmentGateArchitecture
	AssessmentGateMaintainability = core.AssessmentGateMaintainability
	AssessmentGateSafety          = core.AssessmentGateSafety
	AssessmentGateCoverage        = core.AssessmentGateCoverage
	ArchitectureRuleAllow         = core.ArchitectureRuleAllow
	ArchitectureRuleDeny          = core.ArchitectureRuleDeny
)

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
	RefactorReviewDebtMarkers   = core.RefactorReviewDebtMarkers
)

const (
	LowConfidence    = core.LowConfidence
	MediumConfidence = core.MediumConfidence
	HighConfidence   = core.HighConfidence
	RiskLow          = core.RiskLow
	RiskMedium       = core.RiskMedium
	RiskHigh         = core.RiskHigh
)

const (
	OpRenameSymbol    = core.OpRenameSymbol
	OpReplaceSymbol   = core.OpReplaceSymbol
	OpAppendSymbol    = core.OpAppendSymbol
	OpReplaceFunction = core.OpReplaceFunction
	OpAppendFunction  = core.OpAppendFunction
	OpDeleteFunction  = core.OpDeleteFunction
	OpReplaceMethod   = core.OpReplaceMethod
	OpDeleteMethod    = core.OpDeleteMethod
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
