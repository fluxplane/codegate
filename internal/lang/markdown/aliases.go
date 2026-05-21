package markdown

import "github.com/codewandler/codegate/internal/core"

type (
	BackendSpec               = core.BackendSpec
	CapabilitySupport         = core.CapabilitySupport
	OperationSupport          = core.OperationSupport
	AssessmentSupport         = core.AssessmentSupport
	MetricSupport             = core.MetricSupport
	FindingSupport            = core.FindingSupport
	ViolationSupport          = core.ViolationSupport
	Snapshot                  = core.Snapshot
	Index                     = core.Index
	Document                  = core.Document
	PackageInfo               = core.PackageInfo
	Position                  = core.Position
	Range                     = core.Range
	Location                  = core.Location
	SymbolID                  = core.SymbolID
	SymbolKind                = core.SymbolKind
	Symbol                    = core.Symbol
	Occurrence                = core.Occurrence
	OccurrenceKind            = core.OccurrenceKind
	Diagnostic                = core.Diagnostic
	ValidationKind            = core.ValidationKind
	ValidationOptions         = core.ValidationOptions
	ValidationResult          = core.ValidationResult
	AssessmentGate            = core.AssessmentGate
	AssessmentOptions         = core.AssessmentOptions
	AssessmentReport          = core.AssessmentReport
	AssessmentSummary         = core.AssessmentSummary
	ScoreSet                  = core.ScoreSet
	ValidationSummary         = core.ValidationSummary
	Finding                   = core.Finding
	Violation                 = core.Violation
	AssessmentSuggestion      = core.AssessmentSuggestion
	Evidence                  = core.Evidence
	Scope                     = core.Scope
	BackendInfo               = core.BackendInfo
	UnitMetrics               = core.UnitMetrics
	Operation                 = core.Operation
	FileEdit                  = core.FileEdit
	TextEdit                  = core.TextEdit
	Proposal                  = core.Proposal
	OperationKind             = core.OperationKind
	RefactorKind              = core.RefactorKind
	EnsureMarkdownH1          = core.EnsureMarkdownH1
	SetMarkdownHeadingLevel   = core.SetMarkdownHeadingLevel
	InsertMarkdownSectionBody = core.InsertMarkdownSectionBody
	RenameMarkdownHeading     = core.RenameMarkdownHeading
)

const Markdown = core.Markdown

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
	CapabilityNone     = core.CapabilityNone
	CapabilityBasic    = core.CapabilityBasic
	CapabilityAdvanced = core.CapabilityAdvanced
)

const (
	SymbolFile      = core.SymbolFile
	SymbolNamespace = core.SymbolNamespace
)

const OccurrenceDeclaration = core.OccurrenceDeclaration

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
)

const RefactorFixMarkdownStructure = core.RefactorFixMarkdownStructure
const RefactorReviewDebtMarkers = core.RefactorReviewDebtMarkers

const (
	OpMarkdownEnsureH1          = core.OpMarkdownEnsureH1
	OpMarkdownSetHeadingLevel   = core.OpMarkdownSetHeadingLevel
	OpMarkdownInsertSectionBody = core.OpMarkdownInsertSectionBody
	OpMarkdownRenameHeading     = core.OpMarkdownRenameHeading
)
