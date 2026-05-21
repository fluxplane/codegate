package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
	"github.com/codewandler/codegate/language/markdown"
	"github.com/spf13/cobra"
)

type cliConfig struct {
	root         string
	language     string
	includeTests bool
	format       string
}

type app struct {
	cfg                cliConfig
	out                io.Writer
	err                io.Writer
	validationAdapters []codegate.ValidationAdapter
}

type suggestionSummary struct {
	ID         string                 `json:"id"`
	Kind       codegate.RefactorKind  `json:"kind"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary,omitempty"`
	Confidence codegate.Confidence    `json:"confidence"`
	Risk       codegate.RiskLevel     `json:"risk"`
	Operations int                    `json:"operations"`
	Metrics    map[string]float64     `json:"metrics,omitempty"`
	Evidence   []codegate.Evidence    `json:"evidence,omitempty"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type lookupQuery = codegate.LookupQuery

type cycleResult struct {
	Assessment compactAssessmentReport        `json:"assessment"`
	Selected   *codegate.AssessmentSuggestion `json:"selected,omitempty"`
	Applied    bool                           `json:"applied"`
	Validation *codegate.ValidationSummary    `json:"validation,omitempty"`
	Diff       string                         `json:"diff,omitempty"`
	Message    string                         `json:"message,omitempty"`
}

type compactAssessmentReport struct {
	Root                  string                     `json:"root"`
	Language              string                     `json:"language"`
	Summary               codegate.AssessmentSummary `json:"summary"`
	Scores                codegate.ScoreSet          `json:"scores"`
	Validation            codegate.ValidationSummary `json:"validation"`
	Metrics               map[string]interface{}     `json:"metrics,omitempty"`
	FindingCounts         map[string]int             `json:"finding_counts,omitempty"`
	FindingCategoryCounts map[string]int             `json:"finding_category_counts,omitempty"`
	ViolationCounts       map[string]int             `json:"violation_counts,omitempty"`
	TopFindings           []compactIssue             `json:"top_findings,omitempty"`
	TopViolations         []compactIssue             `json:"top_violations,omitempty"`
	TopUnits              []compactUnit              `json:"top_units,omitempty"`
	Suggestions           compactSuggestionSummary   `json:"suggestions"`
	TopSuggestions        []compactSuggestion        `json:"top_suggestions,omitempty"`
}

type compactSuggestionSummary struct {
	Total      int `json:"total"`
	Executable int `json:"executable"`
}

type compactIssue struct {
	Kind     string            `json:"kind"`
	Severity string            `json:"severity"`
	Location codegate.Location `json:"location,omitempty"`
	Package  string            `json:"package,omitempty"`
	Symbol   string            `json:"symbol,omitempty"`
	Allowed  bool              `json:"allowed,omitempty"`
	Reason   string            `json:"reason,omitempty"`
}

type compactUnit struct {
	UnitID        string  `json:"unit_id"`
	DirectFanIn   int     `json:"direct_fan_in,omitempty"`
	DirectFanOut  int     `json:"direct_fan_out,omitempty"`
	CallFanIn     int     `json:"call_fan_in,omitempty"`
	CallFanOut    int     `json:"call_fan_out,omitempty"`
	FileCount     int     `json:"file_count,omitempty"`
	LOC           int     `json:"loc,omitempty"`
	PressureScore float64 `json:"pressure_score,omitempty"`
}

type compactSuggestion struct {
	ID         string                `json:"id"`
	Kind       codegate.RefactorKind `json:"kind"`
	Title      string                `json:"title"`
	Summary    string                `json:"summary,omitempty"`
	Confidence codegate.Confidence   `json:"confidence"`
	Risk       codegate.RiskLevel    `json:"risk"`
	Operations int                   `json:"operations"`
	Metrics    map[string]float64    `json:"metrics,omitempty"`
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
		Use:          "codegate",
		Short:        "Agent-oriented code analysis and improvement loop",
		SilenceUsage: true,
		Long: strings.TrimSpace(`codegate is a CLI skeleton for agent workflows.

It exposes the intended public cycle:

  lookup -> assess -> suggest -> apply -> validate -> reassess

The current implementation uses the public engine facade so the CLI can serve
as an agent skill and API proof at the same time.`),
	}
	cmd.PersistentFlags().StringVar(&a.cfg.root, "root", ".", "workspace root")
	cmd.PersistentFlags().StringVar(&a.cfg.language, "language", "go", "language backend")
	cmd.PersistentFlags().BoolVar(&a.cfg.includeTests, "tests", false, "include test files")
	cmd.PersistentFlags().StringVar(&a.cfg.format, "format", "json", "output format: json")
	cmd.AddCommand(
		a.capabilitiesCommand(),
		a.lookupCommand(),
		a.assessCommand(),
		a.suggestCommand(),
		a.validateCommand(),
		a.cycleCommand(),
	)
	return cmd
}

func (a *app) capabilitiesCommand() *cobra.Command {
	var metricsOnly bool
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "List language backend capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, _, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			specs := eng.Capabilities()
			specs = filterCapabilitySpecs(specs, codegate.LanguageID(a.cfg.language), cmd.Flag("language").Changed, metricsOnly)
			return a.print(specs)
		},
	}
	cmd.Flags().BoolVar(&metricsOnly, "metrics", false, "show only assessment metric support")
	return cmd
}

func (a *app) lookupCommand() *cobra.Command {
	var q lookupQuery
	var kind string
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Resolve a symbol, source position, or structural target",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			q.Scope = scope
			q.Language = scope.Language
			q.IncludeTests = &a.cfg.includeTests
			out, err := eng.Lookup(cmd.Context(), q)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&kind, "kind", "", "symbol kind")
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
		q.Kind = codegate.SymbolKind(kind)
		if q.Path == "" && q.Name == "" && q.QualifiedName == "" {
			return errors.New("lookup requires --path with a position or --name/--qualified-name")
		}
		return nil
	}
	return cmd
}

func (a *app) assessCommand() *cobra.Command {
	var limit int
	var gates []string
	var rulesPath string
	var failOn []string
	var summaryOnly bool
	var view string
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Create an agent-readable quality assessment",
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedGates, err := parseAssessmentGates(gates)
			if err != nil {
				return err
			}
			rules, err := loadArchitectureRules(rulesPath)
			if err != nil {
				return err
			}
			report, err := a.assess(cmd.Context(), limit, parsedGates, rules)
			if err != nil {
				return err
			}
			if summaryOnly {
				view = "summary"
			}
			output, err := assessmentOutput(report, view)
			if err != nil {
				return err
			}
			if err := a.print(output); err != nil {
				return err
			}
			categories, err := parseAssessmentFailureCategories(failOn)
			if err != nil {
				return err
			}
			if assessmentHasFailures(report, categories, rules) {
				return fmt.Errorf("assessment failed selected gates: %s", strings.Join(categories, ","))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "suggestions", 10, "maximum suggestions to include")
	cmd.Flags().StringSliceVar(&gates, "gate", []string{"all"}, "assessment gate: architecture, maintainability, safety, coverage, or all")
	cmd.Flags().StringVar(&rulesPath, "rules", "", "architecture rules JSON file")
	cmd.Flags().StringSliceVar(&failOn, "fail-on", nil, "comma-separated failure categories: boundary, test-boundary, effects, unknown, or all")
	cmd.Flags().StringVar(&view, "view", "compact", "assessment output view: compact, summary, or full")
	cmd.Flags().BoolVar(&summaryOnly, "summary-only", false, "alias for --view summary")
	return cmd
}

func (a *app) suggestCommand() *cobra.Command {
	var executableOnly bool
	var limit int
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "List improvement suggestions",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			proposals, err := eng.Suggest(cmd.Context(), codegate.SuggestOptions{Scope: scope})
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
	var external []string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run explicit validation checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			result, err := eng.Validate(cmd.Context(), codegate.ValidationOptions{
				Scope:    scope,
				Kinds:    validationKinds(scope.Language),
				External: external,
			})
			if err != nil {
				return err
			}
			return a.print(result)
		},
	}
	cmd.Flags().StringSliceVar(&external, "external", nil, "explicit external validation adapter names to run")
	return cmd
}

func (a *app) cycleCommand() *cobra.Command {
	var applyFirst bool
	cmd := &cobra.Command{
		Use:   "cycle",
		Short: "Run assess -> suggest -> optionally apply -> validate",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			assessment, err := a.assess(ctx, 20, []codegate.AssessmentGate{codegate.AssessmentGateAll}, nil)
			if err != nil {
				return err
			}
			result := cycleResult{Assessment: summarizeAssessmentReport(assessment, "compact")}
			var selected *codegate.AssessmentSuggestion
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
			eng, scope, err := a.engine(ctx)
			if err != nil {
				return err
			}
			proposals, err := eng.Suggest(ctx, codegate.SuggestOptions{Scope: scope})
			if err != nil {
				return err
			}
			var proposal codegate.Proposal
			for _, candidate := range proposals {
				if candidate.ID == selected.ID {
					proposal = candidate
					break
				}
			}
			if len(proposal.Operations) == 0 {
				return fmt.Errorf("selected suggestion %s has no operations", selected.ID)
			}
			changes := eng.NewChangeSet()
			if err := changes.Apply(ctx, proposal.Operations...); err != nil {
				return err
			}
			validation, err := changes.Validate(ctx, codegate.ValidationOptions{
				Scope: scope,
				Kinds: validationKinds(scope.Language),
			})
			if err != nil {
				return err
			}
			diff, err := changes.Diff(ctx)
			if err != nil {
				return err
			}
			result.Applied = true
			result.Validation = &codegate.ValidationSummary{
				Passed:         validation.Passed,
				ResolutionMode: validation.ResolutionMode,
				Diagnostics:    len(validation.Diagnostics),
				Files:          len(validation.AffectedPaths),
				Complete:       validation.Complete,
			}
			result.Diff = diff
			return a.print(result)
		},
	}
	cmd.Flags().BoolVar(&applyFirst, "apply-first", false, "apply the first executable suggestion to an in-memory changeset and print the diff")
	return cmd
}

func (a *app) engine(ctx context.Context) (codegate.Engine, codegate.Scope, error) {
	lang := codegate.LanguageID(a.cfg.language)
	switch lang {
	case codegate.Go, codegate.Markdown:
	default:
		return nil, codegate.Scope{}, fmt.Errorf("language %q is not wired; supported languages: go, markdown", a.cfg.language)
	}
	builder := codegate.New().
		Roots(a.cfg.root).
		WithSource(dirSource{fsys: os.DirFS(a.cfg.root)}).
		WithLanguage(golang.New(golang.Config{})).
		WithLanguage(markdown.New(markdown.Config{}))
	for _, adapter := range a.validationAdapters {
		builder.WithValidationAdapter(adapter)
	}
	eng, err := builder.Build(ctx)
	if err != nil {
		return nil, codegate.Scope{}, err
	}
	return eng, codegate.Scope{Language: lang, IncludeTests: a.cfg.includeTests}, nil
}

func (a *app) assess(ctx context.Context, limit int, gates []codegate.AssessmentGate, rules *codegate.ArchitectureRules) (codegate.AssessmentReport, error) {
	eng, scope, err := a.engine(ctx)
	if err != nil {
		return codegate.AssessmentReport{}, err
	}
	return eng.Assess(ctx, codegate.AssessmentOptions{Scope: scope, SuggestionLimit: limit, Gates: gates, Architecture: rules})
}

func assessmentOutput(report codegate.AssessmentReport, view string) (interface{}, error) {
	switch view {
	case "", "compact":
		return summarizeAssessmentReport(report, "compact"), nil
	case "summary":
		return summarizeAssessmentReport(report, "summary"), nil
	case "full":
		return report, nil
	default:
		return nil, fmt.Errorf("unsupported assessment view %q", view)
	}
}

func summarizeAssessmentReport(report codegate.AssessmentReport, view string) compactAssessmentReport {
	out := compactAssessmentReport{
		Root:                  report.Root,
		Language:              report.Language,
		Summary:               report.Summary,
		Scores:                report.Scores,
		Validation:            report.Validation,
		Metrics:               compactAssessmentMetrics(report.Metrics),
		FindingCounts:         findingCounts(report.Findings),
		FindingCategoryCounts: findingCategoryCounts(report.Findings),
		ViolationCounts:       violationCounts(report.Violations),
		Suggestions: compactSuggestionSummary{
			Total:      report.Summary.Suggestions,
			Executable: report.Summary.ExecutableFixes,
		},
	}
	if view == "compact" {
		out.TopFindings = compactFindings(report.Findings, 10)
		out.TopViolations = compactViolations(report.Violations, 10)
		out.TopUnits = compactUnits(report.TopUnits, 5)
		out.TopSuggestions = compactSuggestions(report.Suggestions, 10)
	}
	return out
}

func compactFindings(findings []codegate.Finding, limit int) []compactIssue {
	out := make([]compactIssue, 0, len(findings))
	for _, finding := range findings {
		out = append(out, compactIssue{
			Kind:     finding.Kind,
			Severity: finding.Severity,
			Location: finding.Location,
			Package:  finding.Package,
			Symbol:   finding.Symbol,
			Allowed:  finding.Allowed,
			Reason:   finding.Reason,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func compactViolations(violations []codegate.Violation, limit int) []compactIssue {
	out := make([]compactIssue, 0, len(violations))
	for _, violation := range violations {
		out = append(out, compactIssue{
			Kind:     violation.Kind,
			Severity: violation.Severity,
			Location: violation.Location,
			Package:  violation.Package,
			Symbol:   violation.Symbol,
			Reason:   violation.Reason,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func compactUnits(units []codegate.UnitMetrics, limit int) []compactUnit {
	out := make([]compactUnit, 0, len(units))
	for _, unit := range units {
		out = append(out, compactUnit{
			UnitID:        unit.UnitID,
			DirectFanIn:   unit.DirectFanIn,
			DirectFanOut:  unit.DirectFanOut,
			CallFanIn:     unit.CallFanIn,
			CallFanOut:    unit.CallFanOut,
			FileCount:     unit.FileCount,
			LOC:           unit.LOC,
			PressureScore: unit.PressureScore,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func compactSuggestions(suggestions []codegate.AssessmentSuggestion, limit int) []compactSuggestion {
	out := make([]compactSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		out = append(out, compactSuggestion{
			ID:         suggestion.ID,
			Kind:       suggestion.Kind,
			Title:      suggestion.Title,
			Summary:    suggestion.Summary,
			Confidence: suggestion.Confidence,
			Risk:       suggestion.Risk,
			Operations: suggestion.Operations,
			Metrics:    suggestion.Metrics,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func compactAssessmentMetrics(metrics map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range metrics {
		switch value.(type) {
		case map[string]int, map[string]interface{}:
			continue
		default:
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findingCounts(findings []codegate.Finding) map[string]int {
	out := map[string]int{}
	for _, finding := range findings {
		out[finding.Kind]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findingCategoryCounts(findings []codegate.Finding) map[string]int {
	out := map[string]int{}
	for _, finding := range findings {
		out[findingCategory(finding.Kind)]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findingCategory(kind string) string {
	switch {
	case strings.HasPrefix(kind, "architecture_"):
		return "architecture"
	case strings.HasPrefix(kind, "coverage_"):
		return "coverage"
	case strings.HasPrefix(kind, "performance_"):
		return "performance"
	case strings.HasPrefix(kind, "quality_"), strings.HasPrefix(kind, "maintainability_"):
		return "maintainability"
	case strings.HasPrefix(kind, "safety_"):
		return "safety"
	case strings.HasPrefix(kind, "security_"):
		return "security"
	default:
		return "other"
	}
}

func violationCounts(violations []codegate.Violation) map[string]int {
	out := map[string]int{}
	for _, violation := range violations {
		out[violation.Kind]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterCapabilitySpecs(specs []codegate.BackendSpec, language codegate.LanguageID, filterLanguage bool, metricsOnly bool) []codegate.BackendSpec {
	out := make([]codegate.BackendSpec, 0, len(specs))
	for _, spec := range specs {
		if filterLanguage && spec.Language != language {
			continue
		}
		if metricsOnly {
			spec.Capabilities = nil
			spec.Operations = codegate.OperationSupport{
				Assessment: codegate.AssessmentSupport{
					Metrics: spec.Operations.Assessment.Metrics,
				},
			}
		}
		out = append(out, spec)
	}
	return out
}

func validationKinds(lang codegate.LanguageID) []codegate.ValidationKind {
	if lang == codegate.Go {
		return []codegate.ValidationKind{codegate.ValidationParse, codegate.ValidationTypecheck}
	}
	return []codegate.ValidationKind{codegate.ValidationParse}
}

func (a *app) print(v interface{}) error {
	if a.cfg.format != "json" {
		return fmt.Errorf("unsupported format %q", a.cfg.format)
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func parseAssessmentGates(values []string) ([]codegate.AssessmentGate, error) {
	if len(values) == 0 {
		return []codegate.AssessmentGate{codegate.AssessmentGateAll}, nil
	}
	seen := map[codegate.AssessmentGate]bool{}
	var out []codegate.AssessmentGate
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			gate := codegate.AssessmentGate(part)
			switch gate {
			case codegate.AssessmentGateAll, codegate.AssessmentGateArchitecture, codegate.AssessmentGateMaintainability, codegate.AssessmentGateSafety, codegate.AssessmentGateCoverage:
				if !seen[gate] {
					seen[gate] = true
					out = append(out, gate)
				}
			default:
				return nil, fmt.Errorf("unsupported assessment gate %q", part)
			}
		}
	}
	if len(out) == 0 {
		return []codegate.AssessmentGate{codegate.AssessmentGateAll}, nil
	}
	return out, nil
}

func parseAssessmentFailureCategories(values []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if part == "all" {
				for _, category := range []string{"boundary", "test-boundary", "effects", "unknown"} {
					if !seen[category] {
						seen[category] = true
						out = append(out, category)
					}
				}
				continue
			}
			switch part {
			case "boundary", "test-boundary", "effects", "unknown":
				if !seen[part] {
					seen[part] = true
					out = append(out, part)
				}
			default:
				return nil, fmt.Errorf("unsupported failure category %q", part)
			}
		}
	}
	return out, nil
}

func assessmentHasFailures(report codegate.AssessmentReport, categories []string, rules *codegate.ArchitectureRules) bool {
	if len(categories) == 0 {
		return false
	}
	for _, violation := range report.Violations {
		if violation.Severity != "error" {
			continue
		}
		for _, category := range categories {
			if violationMatchesFailureCategory(violation, category, rules) {
				return true
			}
		}
	}
	return false
}

func violationMatchesFailureCategory(violation codegate.Violation, category string, rules *codegate.ArchitectureRules) bool {
	switch category {
	case "boundary":
		return violation.Kind == "architecture_boundary_violation" || violation.Kind == "architecture_denied_import"
	case "test-boundary":
		return violation.Kind == "architecture_test_boundary_violation" || violation.Kind == "architecture_test_boundary_import"
	case "effects":
		return isArchitectureEffectViolation(violation.Kind, rules)
	case "unknown":
		return violation.Kind == "architecture_unknown_package"
	default:
		return false
	}
}

func isArchitectureEffectViolation(kind string, rules *codegate.ArchitectureRules) bool {
	if strings.HasPrefix(kind, "architecture_effect_") {
		return true
	}
	if rules == nil {
		return false
	}
	for _, rule := range rules.Effects {
		if kind == architectureEffectKind(rule.Name, "architecture_effect_import") || kind == architectureEffectKind(rule.Name, "architecture_effect_call") {
			return true
		}
	}
	return false
}

func architectureEffectKind(name, fallback string) string {
	if name == "" {
		return fallback
	}
	if strings.HasPrefix(name, "architecture_") {
		return name
	}
	return "architecture_" + name
}

func loadArchitectureRules(path string) (*codegate.ArchitectureRules, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules codegate.ArchitectureRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse architecture rules: %w", err)
	}
	if err := validateArchitectureRules(rules.Imports, "imports"); err != nil {
		return nil, err
	}
	if err := validateArchitectureRules(rules.TestImports, "test_imports"); err != nil {
		return nil, err
	}
	if err := validateArchitectureDependencies(rules.Dependencies); err != nil {
		return nil, err
	}
	if err := validateArchitectureEffects(rules.Effects); err != nil {
		return nil, err
	}
	return &rules, nil
}

func validateArchitectureRules(rules []codegate.ArchitectureImportRule, section string) error {
	for i, rule := range rules {
		switch rule.Action {
		case "", codegate.ArchitectureRuleAllow, codegate.ArchitectureRuleDeny:
		default:
			return fmt.Errorf("%s[%d] has unsupported action %q", section, i, rule.Action)
		}
		if rule.From == "" && rule.To == "" {
			return fmt.Errorf("%s[%d] must set from, to, or both", section, i)
		}
	}
	return nil
}

func validateArchitectureDependencies(rules []codegate.ArchitectureDependencyRule) error {
	for i, rule := range rules {
		switch rule.Action {
		case "", codegate.ArchitectureRuleAllow, codegate.ArchitectureRuleDeny:
		default:
			return fmt.Errorf("dependencies[%d] has unsupported action %q", i, rule.Action)
		}
		if rule.FromLayer == "" || rule.ToLayer == "" {
			return fmt.Errorf("dependencies[%d] must set from_layer and to_layer", i)
		}
	}
	return nil
}

func validateArchitectureEffects(rules []codegate.ArchitectureEffectRule) error {
	for i, rule := range rules {
		switch rule.Action {
		case "", codegate.ArchitectureRuleAllow, codegate.ArchitectureRuleDeny:
		default:
			return fmt.Errorf("effects[%d] has unsupported action %q", i, rule.Action)
		}
		if len(rule.Imports) == 0 && len(rule.Calls) == 0 {
			return fmt.Errorf("effects[%d] must set imports, calls, or both", i)
		}
	}
	return nil
}

type dirSource struct {
	fsys fs.FS
}

func (s dirSource) ListFiles(ctx context.Context, scope codegate.Scope) ([]string, error) {
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

func summarizeSuggestions(proposals []codegate.Proposal, executableOnly bool, limit int) []suggestionSummary {
	out := make([]suggestionSummary, 0, len(proposals))
	for _, proposal := range proposals {
		if executableOnly && !codegate.HasOperations(proposal) {
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
