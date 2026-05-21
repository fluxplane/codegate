package codegate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

// Engine is the agent-facing facade for lookup, assessment, suggestions,
// validation, and safe pending edits.
type Engine interface {
	Lookup(ctx context.Context, query LookupQuery) (LookupResult, error)
	Assess(ctx context.Context, opts AssessmentOptions) (AssessmentReport, error)
	Suggest(ctx context.Context, opts SuggestOptions) ([]Proposal, error)
	Validate(ctx context.Context, opts ValidationOptions) (ValidationResult, error)
	NewChangeSet() *ChangeSet
	Capabilities() []BackendSpec
}

type EngineBuilder struct {
	roots     []string
	source    Source
	fsys      fs.FS
	backends  []Backend
	languages []LanguageID
	language  LanguageID
	err       error
}

func New() *EngineBuilder {
	return &EngineBuilder{roots: []string{"."}}
}

// NewEngine is a compatibility alias for New.
func NewEngine() *EngineBuilder {
	return New()
}

func (b *EngineBuilder) Roots(roots ...string) *EngineBuilder {
	if len(roots) == 0 {
		b.err = errors.New("codegate: Roots requires at least one root")
		return b
	}
	b.roots = append([]string(nil), roots...)
	return b
}

func (b *EngineBuilder) WithSource(source Source) *EngineBuilder {
	if source == nil {
		b.err = errors.New("codegate: nil source")
		return b
	}
	b.source = source
	return b
}

func (b *EngineBuilder) WithFS(fsys fs.FS) *EngineBuilder {
	if fsys == nil {
		b.err = errors.New("codegate: nil fs.FS")
		return b
	}
	b.fsys = fsys
	return b
}

func (b *EngineBuilder) WithLanguage(backend Backend) *EngineBuilder {
	if backend == nil {
		b.err = errors.New("codegate: nil language backend")
		return b
	}
	b.backends = append(b.backends, backend)
	spec := backend.Spec()
	if spec.Language == "" {
		b.err = errors.New("codegate: language backend has empty language")
		return b
	}
	b.languages = append(b.languages, spec.Language)
	if b.language == "" {
		b.language = spec.Language
	}
	return b
}

func (b *EngineBuilder) Build(ctx context.Context) (Engine, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if b.err != nil {
		return nil, b.err
	}
	if len(b.roots) != 1 {
		return nil, fmt.Errorf("codegate: engine currently supports exactly one root, got %d", len(b.roots))
	}
	root := b.roots[0]
	if b.source == nil && b.fsys == nil {
		return nil, errors.New("codegate: engine requires WithSource or WithFS")
	}
	if len(b.backends) == 0 {
		return nil, errors.New("codegate: engine requires at least one language backend")
	}
	opts := []Option{withLanguages(b.languages)}
	if b.source != nil {
		opts = append(opts, WithSource(b.source))
	} else {
		opts = append(opts, WithFS(b.fsys))
	}
	for _, backend := range b.backends {
		opts = append(opts, WithBackend(backend))
	}
	ed, err := NewEditor(root, opts...)
	if err != nil {
		return nil, err
	}
	return &engine{root: root, editor: ed, language: b.language}, nil
}

type engine struct {
	root     string
	editor   *Editor
	language LanguageID
}

func (e *engine) Lookup(ctx context.Context, query LookupQuery) (LookupResult, error) {
	scope := query.Scope
	if scope.Language == "" {
		scope.Language = query.Language
	}
	if scope.Language == "" {
		scope.Language = e.language
	}
	if query.IncludeTests != nil {
		scope.IncludeTests = *query.IncludeTests
	}
	includeTests := query.IncludeTests
	if includeTests == nil {
		includeTests = &scope.IncludeTests
	}
	if query.Path != "" && hasLookupPosition(query) {
		nav, err := e.editor.Navigate(ctx, PositionSelector{
			Path:   query.Path,
			Offset: query.Offset,
			Line:   query.Line,
			Column: query.Column,
		}, NavigationOptions{
			Scope:             scope,
			IncludeDocs:       query.IncludeDocs,
			FallbackEnclosing: query.FallbackEnclosing || !query.StrictPosition,
			MaxResults:        query.MaxResults,
		})
		if err != nil {
			return LookupResult{}, err
		}
		result := LookupResult{
			Query:          query,
			Target:         nav.Target,
			Symbols:        nav.Symbols,
			Locations:      nav.Locations,
			Diagnostics:    nav.Diagnostics,
			Ambiguous:      len(nav.Symbols) > 1,
			Complete:       nav.Complete,
			Confidence:     lookupConfidence(len(nav.Symbols)),
			ResolutionMode: nav.ResolutionMode,
			Warnings:       nav.Warnings,
		}
		if err := e.enrichLookup(ctx, &result, query, includeTests); err != nil {
			return LookupResult{}, err
		}
		return result, nil
	}

	sel := SymbolSelector{
		Language:      scope.Language,
		Name:          query.Name,
		QualifiedName: query.QualifiedName,
		Kind:          query.Kind,
		Path:          query.Path,
		UnitID:        scope.UnitID,
		IncludeTests:  includeTests,
	}
	symbols, err := e.editor.FindSymbols(ctx, sel)
	if err != nil {
		return LookupResult{}, err
	}
	if query.MaxResults > 0 && len(symbols) > query.MaxResults {
		symbols = symbols[:query.MaxResults]
	}
	result := LookupResult{
		Query:      query,
		Symbols:    symbols,
		Ambiguous:  len(symbols) > 1,
		Complete:   false,
		Confidence: lookupConfidence(len(symbols)),
	}
	for _, sym := range symbols {
		result.Locations = append(result.Locations, sym.Location)
	}
	if err := e.enrichLookup(ctx, &result, query, includeTests); err != nil {
		return LookupResult{}, err
	}
	return result, nil
}

func (e *engine) Assess(ctx context.Context, opts AssessmentOptions) (AssessmentReport, error) {
	scope := opts.Scope
	if scope.Language == "" {
		scope.Language = e.language
	}
	opts.Scope = scope
	var reports []AssessmentReport
	snapshot := e.editor.snapshot(nil)
	for _, backend := range e.editor.selectedBackends(scope) {
		provider, ok := backend.(AssessmentProvider)
		if !ok {
			reports = append(reports, unsupportedAssessmentReport(backend.Spec(), scope))
			continue
		}
		report, err := provider.Assess(ctx, snapshot, scope, opts)
		if err != nil {
			return AssessmentReport{}, err
		}
		reports = append(reports, report)
	}
	return e.aggregateAssessmentReports(scope, reports), nil
}

func (e *engine) Suggest(ctx context.Context, opts SuggestOptions) ([]Proposal, error) {
	return e.editor.SuggestRefactorings(ctx, WithSuggestScope(opts.Scope))
}

func (e *engine) Validate(ctx context.Context, opts ValidationOptions) (ValidationResult, error) {
	return e.editor.Validate(ctx, opts)
}

func (e *engine) NewChangeSet() *ChangeSet {
	return e.editor.NewChangeSet()
}

func (e *engine) Capabilities() []BackendSpec {
	backends := e.editor.selectedBackends(Scope{})
	specs := make([]BackendSpec, 0, len(backends))
	for _, backend := range backends {
		specs = append(specs, backend.Spec())
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Language < specs[j].Language
	})
	return specs
}

func (e *engine) enrichLookup(ctx context.Context, result *LookupResult, query LookupQuery, includeTests *bool) error {
	if len(result.Symbols) == 0 {
		return nil
	}
	sel := SymbolSelector{ID: result.Symbols[0].ID, IncludeTests: includeTests}
	if query.IncludeRefs {
		refs, err := e.editor.References(ctx, sel)
		if err != nil {
			return err
		}
		result.Occurrences = refs
	}
	if query.IncludeCallers {
		sel.IncludeTests = includeTests
		callers, err := e.editor.Callers(ctx, sel)
		if err != nil {
			return err
		}
		callees, err := e.editor.Callees(ctx, sel)
		if err != nil {
			return err
		}
		result.Callers = callers
		result.Callees = callees
	}
	return nil
}

func hasLookupPosition(query LookupQuery) bool {
	return query.Offset != nil || query.Line > 0 || query.Column > 0
}

func lookupConfidence(symbols int) Confidence {
	switch {
	case symbols == 1:
		return HighConfidence
	case symbols > 1:
		return MediumConfidence
	default:
		return LowConfidence
	}
}

func (e *engine) aggregateAssessmentReports(scope Scope, reports []AssessmentReport) AssessmentReport {
	out := AssessmentReport{
		Root:     e.root,
		Language: string(scope.Language),
		Validation: ValidationSummary{
			Passed:   true,
			Complete: true,
		},
		Scores: ScoreSet{
			Overall:         100,
			Boundary:        100,
			TestBoundary:    100,
			Coupling:        100,
			SideEffect:      100,
			Coverage:        100,
			Maintainability: 100,
		},
		Metrics: map[string]interface{}{
			"score_model": "aggregate-v0",
		},
	}
	if len(reports) == 0 {
		out.Validation.Passed = false
		out.Validation.Complete = false
		out.Scores.Overall = 0
		out.Scores.Coverage = 0
		out.Findings = append(out.Findings, Finding{Kind: "coverage_no_backend", Severity: "error", Reason: "No backend was selected for assessment."})
		finalizeAssessmentSummary(&out)
		return out
	}
	for _, report := range reports {
		if out.Language == "" {
			out.Language = report.Language
		}
		out.Summary.Packages += report.Summary.Packages
		out.Summary.Symbols += report.Summary.Symbols
		out.Summary.Imports += report.Summary.Imports
		out.Summary.Suggestions += report.Summary.Suggestions
		out.Summary.ExecutableFixes += report.Summary.ExecutableFixes
		out.Findings = append(out.Findings, report.Findings...)
		out.Violations = append(out.Violations, report.Violations...)
		out.TopUnits = append(out.TopUnits, report.TopUnits...)
		out.Suggestions = append(out.Suggestions, report.Suggestions...)
		out.Diagnostics = append(out.Diagnostics, report.Diagnostics...)
		out.Validation.Diagnostics += report.Validation.Diagnostics
		out.Validation.Files += report.Validation.Files
		if report.Validation.ResolutionMode != "" {
			out.Validation.ResolutionMode = report.Validation.ResolutionMode
		}
		if !report.Validation.Passed {
			out.Validation.Passed = false
		}
		if !report.Validation.Complete {
			out.Validation.Complete = false
		}
		out.Scores = minScoreSet(out.Scores, report.Scores)
		if report.Scores.Pressure > out.Scores.Pressure {
			out.Scores.Pressure = report.Scores.Pressure
		}
		if model, ok := report.Metrics["score_model"]; ok {
			out.Metrics["provider_score_model"] = model
		}
		if gates, ok := report.Metrics["gates"]; ok {
			out.Metrics["gates"] = gates
		}
	}
	sortAssessmentOutput(&out)
	finalizeAssessmentSummary(&out)
	return out
}

func unsupportedAssessmentReport(spec BackendSpec, scope Scope) AssessmentReport {
	return AssessmentReport{
		Language: string(spec.Language),
		Summary:  AssessmentSummary{Score: 50},
		Scores: ScoreSet{
			Overall:         50,
			Coverage:        50,
			Maintainability: 50,
		},
		Validation: ValidationSummary{Passed: true, Complete: false},
		Findings: []Finding{{
			Kind:     "coverage_assessment_unsupported",
			Severity: "warning",
			Reason:   "Backend does not implement assessment reporting.",
		}},
		Metrics: map[string]interface{}{
			"score_model": "unsupported",
			"language":    string(scope.Language),
		},
	}
}

func finalizeAssessmentSummary(r *AssessmentReport) {
	r.Summary.Findings = len(r.Findings)
	r.Summary.Violations = len(r.Violations)
	r.Summary.Diagnostics = len(r.Diagnostics)
	r.Summary.Score = r.Scores.Overall
}

func minScoreSet(a, b ScoreSet) ScoreSet {
	if b.Overall != 0 && b.Overall < a.Overall {
		a.Overall = b.Overall
	}
	if b.Boundary != 0 && b.Boundary < a.Boundary {
		a.Boundary = b.Boundary
	}
	if b.TestBoundary != 0 && b.TestBoundary < a.TestBoundary {
		a.TestBoundary = b.TestBoundary
	}
	if b.Coupling != 0 && b.Coupling < a.Coupling {
		a.Coupling = b.Coupling
	}
	if b.SideEffect != 0 && b.SideEffect < a.SideEffect {
		a.SideEffect = b.SideEffect
	}
	if b.Coverage != 0 && b.Coverage < a.Coverage {
		a.Coverage = b.Coverage
	}
	if b.Maintainability != 0 && b.Maintainability < a.Maintainability {
		a.Maintainability = b.Maintainability
	}
	return a
}

func sortAssessmentOutput(report *AssessmentReport) {
	sort.Slice(report.TopUnits, func(i, j int) bool {
		if report.TopUnits[i].PressureScore == report.TopUnits[j].PressureScore {
			return report.TopUnits[i].UnitID < report.TopUnits[j].UnitID
		}
		return report.TopUnits[i].PressureScore > report.TopUnits[j].PressureScore
	})
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Severity != report.Findings[j].Severity {
			return report.Findings[i].Severity > report.Findings[j].Severity
		}
		if report.Findings[i].Kind != report.Findings[j].Kind {
			return report.Findings[i].Kind < report.Findings[j].Kind
		}
		return report.Findings[i].Package < report.Findings[j].Package
	})
	sort.Slice(report.Violations, func(i, j int) bool {
		if report.Violations[i].Severity != report.Violations[j].Severity {
			return report.Violations[i].Severity > report.Violations[j].Severity
		}
		return report.Violations[i].Kind < report.Violations[j].Kind
	})
}
