package editor

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
	roots    []string
	source   Source
	fsys     fs.FS
	backends []Backend
	language LanguageID
	err      error
}

// NewEngine creates the public codegate-style builder. The existing New
// constructor remains for compatibility with the lower-level Editor API.
func NewEngine() *EngineBuilder {
	return &EngineBuilder{roots: []string{"."}, language: Go}
}

func (b *EngineBuilder) Roots(roots ...string) *EngineBuilder {
	if len(roots) == 0 {
		b.err = errors.New("editor: Roots requires at least one root")
		return b
	}
	b.roots = append([]string(nil), roots...)
	return b
}

func (b *EngineBuilder) WithSource(source Source) *EngineBuilder {
	if source == nil {
		b.err = errors.New("editor: nil source")
		return b
	}
	b.source = source
	return b
}

func (b *EngineBuilder) WithFS(fsys fs.FS) *EngineBuilder {
	if fsys == nil {
		b.err = errors.New("editor: nil fs.FS")
		return b
	}
	b.fsys = fsys
	return b
}

func (b *EngineBuilder) WithLanguage(backend Backend) *EngineBuilder {
	if backend == nil {
		b.err = errors.New("editor: nil language backend")
		return b
	}
	b.backends = append(b.backends, backend)
	spec := backend.Spec()
	if b.language == "" || b.language == Go {
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
		return nil, fmt.Errorf("editor: engine currently supports exactly one root, got %d", len(b.roots))
	}
	root := b.roots[0]
	if b.source == nil && b.fsys == nil {
		return nil, errors.New("editor: engine requires WithSource or WithFS")
	}
	opts := []Option{WithLanguage(b.language)}
	if b.source != nil {
		opts = append(opts, WithSource(b.source))
	} else {
		opts = append(opts, WithFS(b.fsys))
	}
	for _, backend := range b.backends {
		opts = append(opts, WithBackend(backend))
	}
	ed, err := New(root, opts...)
	if err != nil {
		return nil, err
	}
	return &engine{root: root, editor: ed}, nil
}

type engine struct {
	root   string
	editor *Editor
}

func (e *engine) Lookup(ctx context.Context, query LookupQuery) (LookupResult, error) {
	scope := query.Scope
	if scope.Language == "" {
		scope.Language = query.Language
	}
	if scope.Language == "" {
		scope.Language = Go
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
		scope.Language = Go
	}
	packages, err := e.editor.Packages(ctx, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	includeTests := scope.IncludeTests
	symbols, err := e.editor.FindSymbols(ctx, SymbolSelector{Language: scope.Language, IncludeTests: &includeTests})
	if err != nil {
		return AssessmentReport{}, err
	}
	imports, err := e.editor.Imports(ctx, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	metrics, err := e.editor.Metrics(ctx, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	validation, err := e.editor.Validate(ctx, ValidationOptions{
		Scope: scope,
		Kinds: []ValidationKind{ValidationParse, ValidationTypecheck},
	})
	if err != nil {
		return AssessmentReport{}, err
	}
	proposals, err := e.editor.SuggestRefactorings(ctx, WithSuggestScope(scope))
	if err != nil {
		return AssessmentReport{}, err
	}
	executable := 0
	for _, proposal := range proposals {
		if HasOperations(proposal) {
			executable++
		}
	}
	top := topAssessmentUnits(metrics.Units, optionLimit(opts.TopUnitLimit, 8))
	pressure := 0.0
	if len(top) > 0 {
		pressure = top[0].PressureScore
	}
	diagnostics := append([]Diagnostic(nil), packages.Diagnostics...)
	diagnostics = append(diagnostics, metrics.Diagnostics...)
	diagnostics = append(diagnostics, validation.Diagnostics...)
	return AssessmentReport{
		Root:     e.root,
		Language: string(scope.Language),
		Summary: AssessmentSummary{
			Packages:        len(packages.Packages),
			Symbols:         len(symbols),
			Imports:         len(imports),
			Suggestions:     len(proposals),
			ExecutableFixes: executable,
			Diagnostics:     len(diagnostics),
			Score:           coarseAssessmentScore(validation.Passed, len(proposals), pressure),
		},
		Scores: ScoreSet{
			Overall:         coarseAssessmentScore(validation.Passed, len(proposals), pressure),
			Maintainability: coarseAssessmentScore(true, len(proposals), pressure),
			Coverage:        100,
			Pressure:        pressure,
		},
		Validation: ValidationSummary{
			Passed:         validation.Passed,
			ResolutionMode: validation.ResolutionMode,
			Diagnostics:    len(validation.Diagnostics),
			Files:          len(validation.AffectedPaths),
			Complete:       validation.Complete,
		},
		TopUnits:    top,
		Suggestions: summarizeAssessmentSuggestions(proposals, opts.SuggestionLimit),
		Diagnostics: diagnostics,
		Metrics: map[string]interface{}{
			"score_model": "skeleton",
			"note":        "assessment scoring is public and will be expanded with architecture gates next",
		},
	}, nil
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
	specs := make([]BackendSpec, 0, len(e.editor.backends))
	for _, backend := range e.editor.backends {
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

func topAssessmentUnits(units []UnitMetrics, limit int) []UnitMetrics {
	units = append([]UnitMetrics(nil), units...)
	sort.Slice(units, func(i, j int) bool {
		if units[i].PressureScore == units[j].PressureScore {
			return units[i].UnitID < units[j].UnitID
		}
		return units[i].PressureScore > units[j].PressureScore
	})
	if limit > 0 && len(units) > limit {
		return units[:limit]
	}
	return units
}

func summarizeAssessmentSuggestions(proposals []Proposal, limit int) []AssessmentSuggestion {
	out := make([]AssessmentSuggestion, 0, len(proposals))
	for _, proposal := range proposals {
		out = append(out, AssessmentSuggestion{
			ID:         proposal.ID,
			Kind:       proposal.Kind,
			Title:      proposal.Title,
			Summary:    proposal.Summary,
			Confidence: proposal.Confidence,
			Risk:       proposal.Risk,
			Operations: len(proposal.Operations),
			Metrics:    proposal.Metrics,
			Evidence:   proposal.Evidence,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func coarseAssessmentScore(validationPassed bool, suggestions int, pressure float64) int {
	if !validationPassed {
		return 40
	}
	score := 100 - minAssessmentInt(40, suggestions/5) - minAssessmentInt(20, int(pressure/100))
	return maxAssessmentInt(50, score)
}

func optionLimit(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func minAssessmentInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxAssessmentInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
