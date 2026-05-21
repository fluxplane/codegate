package codegate

import "github.com/codewandler/codegate/internal/core"

type LookupQuery struct {
	Path              string     `json:"path,omitempty"`
	Offset            *int       `json:"offset,omitempty"`
	Line              int        `json:"line,omitempty"`
	Column            int        `json:"column,omitempty"`
	Name              string     `json:"name,omitempty"`
	QualifiedName     string     `json:"qualified_name,omitempty"`
	Kind              SymbolKind `json:"kind,omitempty"`
	Language          LanguageID `json:"language,omitempty"`
	Scope             Scope      `json:"scope,omitempty"`
	IncludeDocs       bool       `json:"include_docs,omitempty"`
	IncludeRefs       bool       `json:"include_refs,omitempty"`
	IncludeCallers    bool       `json:"include_callers,omitempty"`
	IncludeTests      *bool      `json:"include_tests,omitempty"`
	FallbackEnclosing bool       `json:"fallback_enclosing,omitempty"`
	StrictPosition    bool       `json:"strict_position,omitempty"`
	MaxResults        int        `json:"max_results,omitempty"`
}

type LookupResult struct {
	Query          LookupQuery      `json:"query"`
	Target         NavigationTarget `json:"target"`
	Symbols        []Symbol         `json:"symbols,omitempty"`
	Locations      []Location       `json:"locations,omitempty"`
	Occurrences    []Occurrence     `json:"occurrences,omitempty"`
	Callers        []CallEdge       `json:"callers,omitempty"`
	Callees        []CallEdge       `json:"callees,omitempty"`
	Diagnostics    []Diagnostic     `json:"diagnostics,omitempty"`
	Ambiguous      bool             `json:"ambiguous,omitempty"`
	Complete       bool             `json:"complete"`
	Confidence     Confidence       `json:"confidence"`
	ResolutionMode string           `json:"resolution_mode,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type (
	AssessmentGate       = core.AssessmentGate
	AssessmentOptions    = core.AssessmentOptions
	AssessmentReport     = core.AssessmentReport
	AssessmentSummary    = core.AssessmentSummary
	ScoreSet             = core.ScoreSet
	ValidationSummary    = core.ValidationSummary
	Finding              = core.Finding
	Violation            = core.Violation
	AssessmentSuggestion = core.AssessmentSuggestion
	AssessmentProvider   = core.AssessmentProvider
)

const (
	AssessmentGateAll             = core.AssessmentGateAll
	AssessmentGateArchitecture    = core.AssessmentGateArchitecture
	AssessmentGateMaintainability = core.AssessmentGateMaintainability
	AssessmentGateSafety          = core.AssessmentGateSafety
	AssessmentGateCoverage        = core.AssessmentGateCoverage
)
