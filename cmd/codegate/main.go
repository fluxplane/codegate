package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/codewandler/editor"
	"github.com/spf13/cobra"
)

type cliConfig struct {
	root         string
	language     string
	includeTests bool
	format       string
}

type app struct {
	cfg cliConfig
	out io.Writer
	err io.Writer
}

type assessmentReport struct {
	Root        string                 `json:"root"`
	Language    string                 `json:"language"`
	Summary     assessmentSummary      `json:"summary"`
	Scores      assessmentScores       `json:"scores"`
	Validation  validationSummary      `json:"validation"`
	TopUnits    []editor.UnitMetrics   `json:"top_units,omitempty"`
	Suggestions []suggestionSummary    `json:"suggestions,omitempty"`
	Diagnostics []editor.Diagnostic    `json:"diagnostics,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
}

type assessmentSummary struct {
	Packages        int `json:"packages"`
	Symbols         int `json:"symbols"`
	Imports         int `json:"imports"`
	Suggestions     int `json:"suggestions"`
	ExecutableFixes int `json:"executable_fixes"`
}

type assessmentScores struct {
	Overall         int     `json:"overall"`
	Maintainability int     `json:"maintainability"`
	Pressure        float64 `json:"pressure"`
}

type validationSummary struct {
	Passed         bool   `json:"passed"`
	ResolutionMode string `json:"resolution_mode"`
	Diagnostics    int    `json:"diagnostics"`
	Files          int    `json:"files"`
}

type suggestionSummary struct {
	ID         string                 `json:"id"`
	Kind       editor.RefactorKind    `json:"kind"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary,omitempty"`
	Confidence editor.Confidence      `json:"confidence"`
	Risk       editor.RiskLevel       `json:"risk"`
	Operations int                    `json:"operations"`
	Metrics    map[string]float64     `json:"metrics,omitempty"`
	Evidence   []editor.Evidence      `json:"evidence,omitempty"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type lookupResult struct {
	Query       lookupQuery             `json:"query"`
	Target      editor.NavigationTarget `json:"target"`
	Symbols     []editor.Symbol         `json:"symbols,omitempty"`
	Locations   []editor.Location       `json:"locations,omitempty"`
	Occurrences []editor.Occurrence     `json:"occurrences,omitempty"`
	Callers     []editor.CallEdge       `json:"callers,omitempty"`
	Callees     []editor.CallEdge       `json:"callees,omitempty"`
	Diagnostics []editor.Diagnostic     `json:"diagnostics,omitempty"`
	Ambiguous   bool                    `json:"ambiguous,omitempty"`
	Complete    bool                    `json:"complete"`
	Warnings    []string                `json:"warnings,omitempty"`
}

type lookupQuery struct {
	Path           string `json:"path,omitempty"`
	Offset         *int   `json:"offset,omitempty"`
	Line           int    `json:"line,omitempty"`
	Column         int    `json:"column,omitempty"`
	Name           string `json:"name,omitempty"`
	QualifiedName  string `json:"qualified_name,omitempty"`
	Kind           string `json:"kind,omitempty"`
	IncludeRefs    bool   `json:"include_refs,omitempty"`
	IncludeCallers bool   `json:"include_callers,omitempty"`
}

type cycleResult struct {
	Assessment assessmentReport   `json:"assessment"`
	Selected   *suggestionSummary `json:"selected,omitempty"`
	Applied    bool               `json:"applied"`
	Validation *validationSummary `json:"validation,omitempty"`
	Diff       string             `json:"diff,omitempty"`
	Message    string             `json:"message,omitempty"`
}

func main() {
	a := &app{out: os.Stdout, err: os.Stderr}
	if err := a.rootCommand().Execute(); err != nil {
		fmt.Fprintf(a.err, "codegate: %v\n", err)
		os.Exit(1)
	}
}

func (a *app) rootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegate",
		Short: "Agent-oriented code analysis and improvement loop",
		Long: strings.TrimSpace(`codegate is a CLI skeleton for agent workflows.

It exposes the intended public cycle:

  lookup -> assess -> suggest -> apply -> validate -> reassess

The current implementation is backed by the existing editor Go APIs while the
public codegate engine facade is still being designed.`),
	}
	cmd.PersistentFlags().StringVar(&a.cfg.root, "root", ".", "workspace root")
	cmd.PersistentFlags().StringVar(&a.cfg.language, "language", "go", "language backend")
	cmd.PersistentFlags().BoolVar(&a.cfg.includeTests, "tests", false, "include test files")
	cmd.PersistentFlags().StringVar(&a.cfg.format, "format", "json", "output format: json")
	cmd.AddCommand(
		a.lookupCommand(),
		a.assessCommand(),
		a.suggestCommand(),
		a.validateCommand(),
		a.cycleCommand(),
	)
	return cmd
}

func (a *app) lookupCommand() *cobra.Command {
	var q lookupQuery
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Resolve a symbol, source position, or structural target",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ed, scope, err := a.editor()
			if err != nil {
				return err
			}
			scope.Path = q.Path
			var out lookupResult
			out.Query = q
			if q.Path != "" && (q.Offset != nil || q.Line > 0 || q.Column > 0) {
				nav, err := ed.Navigate(ctx, editor.PositionSelector{Path: q.Path, Offset: q.Offset, Line: q.Line, Column: q.Column}, editor.NavigationOptions{Scope: scope, FallbackEnclosing: true})
				if err != nil {
					return err
				}
				out.Target = nav.Target
				out.Symbols = nav.Symbols
				out.Locations = nav.Locations
				out.Diagnostics = nav.Diagnostics
				out.Complete = nav.Complete
				out.Warnings = nav.Warnings
				if len(nav.Symbols) > 0 {
					out.Ambiguous = len(nav.Symbols) > 1
				}
			} else {
				sel := editor.SymbolSelector{
					Language:      editor.LanguageID(a.cfg.language),
					Name:          q.Name,
					QualifiedName: q.QualifiedName,
					Kind:          editor.SymbolKind(q.Kind),
					Path:          q.Path,
					IncludeTests:  &a.cfg.includeTests,
				}
				symbols, err := ed.FindSymbols(ctx, sel)
				if err != nil {
					return err
				}
				out.Symbols = symbols
				out.Ambiguous = len(symbols) > 1
				out.Complete = false
				for _, sym := range symbols {
					out.Locations = append(out.Locations, sym.Location)
				}
				if q.IncludeRefs && len(symbols) > 0 {
					refs, err := ed.References(ctx, editor.SymbolSelector{ID: symbols[0].ID, IncludeTests: &a.cfg.includeTests})
					if err != nil {
						return err
					}
					out.Occurrences = refs
				}
				if q.IncludeCallers && len(symbols) > 0 {
					callers, err := ed.Callers(ctx, editor.SymbolSelector{ID: symbols[0].ID, IncludeTests: &a.cfg.includeTests})
					if err != nil {
						return err
					}
					callees, err := ed.Callees(ctx, editor.SymbolSelector{ID: symbols[0].ID, IncludeTests: &a.cfg.includeTests})
					if err != nil {
						return err
					}
					out.Callers = callers
					out.Callees = callees
				}
			}
			return a.print(out)
		},
	}
	cmd.Flags().StringVar(&q.Path, "path", "", "workspace-relative path")
	cmd.Flags().Int("offset", -1, "byte offset")
	cmd.Flags().IntVar(&q.Line, "line", 0, "1-indexed line")
	cmd.Flags().IntVar(&q.Column, "column", 0, "1-indexed byte column")
	cmd.Flags().StringVar(&q.Name, "name", "", "symbol or structural name")
	cmd.Flags().StringVar(&q.QualifiedName, "qualified-name", "", "qualified symbol name")
	cmd.Flags().StringVar(&q.Kind, "kind", "", "symbol kind")
	cmd.Flags().BoolVar(&q.IncludeRefs, "refs", false, "include references for symbol lookup")
	cmd.Flags().BoolVar(&q.IncludeCallers, "callers", false, "include callers/callees for symbol lookup")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		offset, err := cmd.Flags().GetInt("offset")
		if err != nil {
			return err
		}
		if offset >= 0 {
			q.Offset = &offset
		}
		if q.Path == "" && q.Name == "" && q.QualifiedName == "" {
			return errors.New("lookup requires --path with a position or --name/--qualified-name")
		}
		return nil
	}
	return cmd
}

func (a *app) assessCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Create an agent-readable quality assessment",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := a.assess(cmd.Context(), limit)
			if err != nil {
				return err
			}
			return a.print(report)
		},
	}
	cmd.Flags().IntVar(&limit, "suggestions", 10, "maximum suggestions to include")
	return cmd
}

func (a *app) suggestCommand() *cobra.Command {
	var executableOnly bool
	var limit int
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "List improvement suggestions",
		RunE: func(cmd *cobra.Command, args []string) error {
			ed, scope, err := a.editor()
			if err != nil {
				return err
			}
			proposals, err := ed.SuggestRefactorings(cmd.Context(), editor.WithSuggestScope(scope))
			if err != nil {
				return err
			}
			out := summarizeSuggestions(proposals, executableOnly, limit)
			return a.print(out)
		},
	}
	cmd.Flags().BoolVar(&executableOnly, "executable", false, "only include suggestions with operations")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum suggestions")
	return cmd
}

func (a *app) validateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run explicit validation checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			ed, scope, err := a.editor()
			if err != nil {
				return err
			}
			result, err := ed.Validate(cmd.Context(), editor.ValidationOptions{
				Scope: scope,
				Kinds: []editor.ValidationKind{editor.ValidationParse, editor.ValidationTypecheck},
			})
			if err != nil {
				return err
			}
			return a.print(result)
		},
	}
	return cmd
}

func (a *app) cycleCommand() *cobra.Command {
	var applyFirst bool
	cmd := &cobra.Command{
		Use:   "cycle",
		Short: "Run assess -> suggest -> optionally apply -> validate",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			assessment, err := a.assess(ctx, 20)
			if err != nil {
				return err
			}
			result := cycleResult{Assessment: assessment}
			var selected *suggestionSummary
			for i := range assessment.Suggestions {
				if assessment.Suggestions[i].Operations > 0 {
					next := assessment.Suggestions[i]
					selected = &next
					break
				}
			}
			result.Selected = selected
			if selected == nil {
				result.Message = "no executable suggestion available"
				return a.print(result)
			}
			if !applyFirst {
				result.Message = "dry run: pass --apply-first to apply the first executable suggestion to an in-memory changeset"
				return a.print(result)
			}
			ed, scope, err := a.editor()
			if err != nil {
				return err
			}
			proposals, err := ed.SuggestRefactorings(ctx, editor.WithSuggestScope(scope))
			if err != nil {
				return err
			}
			var proposal editor.Proposal
			for _, candidate := range proposals {
				if candidate.ID == selected.ID {
					proposal = candidate
					break
				}
			}
			if len(proposal.Operations) == 0 {
				return fmt.Errorf("selected suggestion %s has no operations", selected.ID)
			}
			changes := ed.NewChangeSet()
			if err := changes.Apply(ctx, proposal.Operations...); err != nil {
				return err
			}
			validation, err := changes.Validate(ctx, editor.ValidationOptions{
				Scope: scope,
				Kinds: []editor.ValidationKind{editor.ValidationParse, editor.ValidationTypecheck},
			})
			if err != nil {
				return err
			}
			diff, err := changes.Diff(ctx)
			if err != nil {
				return err
			}
			result.Applied = true
			result.Validation = &validationSummary{
				Passed:         validation.Passed,
				ResolutionMode: validation.ResolutionMode,
				Diagnostics:    len(validation.Diagnostics),
				Files:          len(validation.AffectedPaths),
			}
			result.Diff = diff
			return a.print(result)
		},
	}
	cmd.Flags().BoolVar(&applyFirst, "apply-first", false, "apply the first executable suggestion to an in-memory changeset and print the diff")
	return cmd
}

func (a *app) editor() (*editor.Editor, editor.Scope, error) {
	if a.cfg.language != string(editor.Go) {
		return nil, editor.Scope{}, fmt.Errorf("language %q is not wired yet; current skeleton supports go", a.cfg.language)
	}
	source := dirSource{fsys: os.DirFS(a.cfg.root)}
	ed, err := editor.New(".", editor.WithSource(source), editor.WithLanguage(editor.Go))
	if err != nil {
		return nil, editor.Scope{}, err
	}
	return ed, editor.Scope{Language: editor.Go, IncludeTests: a.cfg.includeTests}, nil
}

func (a *app) assess(ctx context.Context, limit int) (assessmentReport, error) {
	ed, scope, err := a.editor()
	if err != nil {
		return assessmentReport{}, err
	}
	packages, err := ed.Packages(ctx, scope)
	if err != nil {
		return assessmentReport{}, err
	}
	symbols, err := ed.FindSymbols(ctx, editor.SymbolSelector{Language: editor.Go, IncludeTests: &a.cfg.includeTests})
	if err != nil {
		return assessmentReport{}, err
	}
	imports, err := ed.Imports(ctx, scope)
	if err != nil {
		return assessmentReport{}, err
	}
	metrics, err := ed.Metrics(ctx, scope)
	if err != nil {
		return assessmentReport{}, err
	}
	validation, err := ed.Validate(ctx, editor.ValidationOptions{
		Scope: scope,
		Kinds: []editor.ValidationKind{editor.ValidationParse, editor.ValidationTypecheck},
	})
	if err != nil {
		return assessmentReport{}, err
	}
	proposals, err := ed.SuggestRefactorings(ctx, editor.WithSuggestScope(scope))
	if err != nil {
		return assessmentReport{}, err
	}
	executable := 0
	for _, proposal := range proposals {
		if editor.HasOperations(proposal) {
			executable++
		}
	}
	topUnits := topUnits(metrics.Units, 8)
	pressure := 0.0
	if len(topUnits) > 0 {
		pressure = topUnits[0].PressureScore
	}
	return assessmentReport{
		Root:     a.cfg.root,
		Language: a.cfg.language,
		Summary: assessmentSummary{
			Packages:        len(packages.Packages),
			Symbols:         len(symbols),
			Imports:         len(imports),
			Suggestions:     len(proposals),
			ExecutableFixes: executable,
		},
		Scores: assessmentScores{
			Overall:         coarseScore(validation.Passed, len(proposals), pressure),
			Maintainability: coarseScore(true, len(proposals), pressure),
			Pressure:        pressure,
		},
		Validation: validationSummary{
			Passed:         validation.Passed,
			ResolutionMode: validation.ResolutionMode,
			Diagnostics:    len(validation.Diagnostics),
			Files:          len(validation.AffectedPaths),
		},
		TopUnits:    topUnits,
		Suggestions: summarizeSuggestions(proposals, false, limit),
		Diagnostics: append(packages.Diagnostics, metrics.Diagnostics...),
		Metrics: map[string]interface{}{
			"score_model": "skeleton",
			"note":        "assessment scoring will move to codegate.Assess once the public engine API lands",
		},
	}, nil
}

func (a *app) print(v interface{}) error {
	if a.cfg.format != "json" {
		return fmt.Errorf("unsupported format %q", a.cfg.format)
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type dirSource struct {
	fsys fs.FS
}

func (s dirSource) ListFiles(ctx context.Context, scope editor.Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	root := scope.Path
	if root == "" {
		root = scope.Root
	}
	if root == "" {
		root = "."
	}
	var files []string
	err := fs.WalkDir(s.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", "vendor":
				return fs.SkipDir
			default:
				return nil
			}
		}
		files = append(files, p)
		return nil
	})
	return files, err
}

func (s dirSource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return fs.ReadFile(s.fsys, path)
}

func summarizeSuggestions(proposals []editor.Proposal, executableOnly bool, limit int) []suggestionSummary {
	out := make([]suggestionSummary, 0, len(proposals))
	for _, proposal := range proposals {
		if executableOnly && !editor.HasOperations(proposal) {
			continue
		}
		out = append(out, suggestionSummary{
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

func topUnits(units []editor.UnitMetrics, limit int) []editor.UnitMetrics {
	units = append([]editor.UnitMetrics(nil), units...)
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

func coarseScore(validationPassed bool, suggestions int, pressure float64) int {
	if !validationPassed {
		return 40
	}
	score := 100 - minInt(40, suggestions/5) - minInt(20, int(pressure/100))
	return maxInt(50, score)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
