package codegate

import "github.com/fluxplane/codegate/internal/core"

// Proposal aliases expose suggestion and risk metadata from the public
// package while keeping the shared model in internal/core.
type (
	RefactorKind   = core.RefactorKind
	Confidence     = core.Confidence
	RiskLevel      = core.RiskLevel
	Proposal       = core.Proposal
	SuggestOption  = core.SuggestOption
	SuggestOptions = core.SuggestOptions
)

// Refactor kind aliases describe the type of improvement suggested by a
// backend.
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

// Confidence aliases describe how strongly codegate recommends a proposal.
const (
	LowConfidence    = core.LowConfidence
	MediumConfidence = core.MediumConfidence
	HighConfidence   = core.HighConfidence
)

// Risk level aliases describe the expected blast radius of applying a
// proposal.
const (
	RiskLow    = core.RiskLow
	RiskMedium = core.RiskMedium
	RiskHigh   = core.RiskHigh
)

// WithSuggestScope returns an option that constrains suggestion generation to
// a scope.
func WithSuggestScope(scope Scope) SuggestOption {
	return core.WithSuggestScope(scope)
}
