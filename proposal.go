package codegate

import "github.com/codewandler/codegate/internal/core"

type (
	RefactorKind   = core.RefactorKind
	Confidence     = core.Confidence
	RiskLevel      = core.RiskLevel
	Proposal       = core.Proposal
	SuggestOption  = core.SuggestOption
	SuggestOptions = core.SuggestOptions
)

const (
	RefactorDeleteSymbol         = core.RefactorDeleteSymbol
	RefactorExtractFunction      = core.RefactorExtractFunction
	RefactorIntroduceConfig      = core.RefactorIntroduceConfig
	RefactorSplitFunction        = core.RefactorSplitFunction
	RefactorSplitPackage         = core.RefactorSplitPackage
	RefactorReplaceFlagArgument  = core.RefactorReplaceFlagArgument
	RefactorFixMarkdownStructure = core.RefactorFixMarkdownStructure
	RefactorReviewDebtMarkers    = core.RefactorReviewDebtMarkers
)

const (
	LowConfidence    = core.LowConfidence
	MediumConfidence = core.MediumConfidence
	HighConfidence   = core.HighConfidence
)

const (
	RiskLow    = core.RiskLow
	RiskMedium = core.RiskMedium
	RiskHigh   = core.RiskHigh
)

func WithSuggestScope(scope Scope) SuggestOption {
	return core.WithSuggestScope(scope)
}
