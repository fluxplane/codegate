package editor

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

type AssessmentOptions struct {
	Scope           Scope `json:"scope,omitempty"`
	SuggestionLimit int   `json:"suggestion_limit,omitempty"`
	TopUnitLimit    int   `json:"top_unit_limit,omitempty"`
}

type AssessmentReport struct {
	Root        string                 `json:"root"`
	Language    string                 `json:"language"`
	Summary     AssessmentSummary      `json:"summary"`
	Scores      ScoreSet               `json:"scores"`
	Validation  ValidationSummary      `json:"validation"`
	Findings    []Finding              `json:"findings,omitempty"`
	Violations  []Violation            `json:"violations,omitempty"`
	TopUnits    []UnitMetrics          `json:"top_units,omitempty"`
	Suggestions []AssessmentSuggestion `json:"suggestions,omitempty"`
	Diagnostics []Diagnostic           `json:"diagnostics,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
}

type AssessmentSummary struct {
	Score           int `json:"score"`
	Packages        int `json:"packages"`
	Symbols         int `json:"symbols"`
	Imports         int `json:"imports"`
	Suggestions     int `json:"suggestions"`
	ExecutableFixes int `json:"executable_fixes"`
	Findings        int `json:"findings"`
	Violations      int `json:"violations"`
	Diagnostics     int `json:"diagnostics"`
}

type ScoreSet struct {
	Overall         int     `json:"overall"`
	Boundary        int     `json:"boundary,omitempty"`
	TestBoundary    int     `json:"test_boundary,omitempty"`
	Coupling        int     `json:"coupling,omitempty"`
	SideEffect      int     `json:"side_effect,omitempty"`
	Coverage        int     `json:"coverage,omitempty"`
	Maintainability int     `json:"maintainability"`
	Pressure        float64 `json:"pressure"`
}

type ValidationSummary struct {
	Passed         bool   `json:"passed"`
	ResolutionMode string `json:"resolution_mode"`
	Diagnostics    int    `json:"diagnostics"`
	Files          int    `json:"files"`
	Complete       bool   `json:"complete"`
}

type Finding struct {
	Kind     string     `json:"kind"`
	Severity string     `json:"severity"`
	Location Location   `json:"location,omitempty"`
	Package  string     `json:"package,omitempty"`
	Symbol   string     `json:"symbol,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Allowed  bool       `json:"allowed,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

type Violation struct {
	Kind     string     `json:"kind"`
	Severity string     `json:"severity"`
	Location Location   `json:"location,omitempty"`
	Package  string     `json:"package,omitempty"`
	Symbol   string     `json:"symbol,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

type AssessmentSuggestion struct {
	ID         string             `json:"id"`
	Kind       RefactorKind       `json:"kind"`
	Title      string             `json:"title"`
	Summary    string             `json:"summary,omitempty"`
	Confidence Confidence         `json:"confidence"`
	Risk       RiskLevel          `json:"risk"`
	Operations int                `json:"operations"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Evidence   []Evidence         `json:"evidence,omitempty"`
}
