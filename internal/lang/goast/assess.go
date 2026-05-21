package goast

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/codegate/internal/core"
)

func (b GoBackend) Assess(ctx context.Context, snapshot Snapshot, scope Scope, opts AssessmentOptions) (AssessmentReport, error) {
	if scope.Language != "" && scope.Language != Go {
		return AssessmentReport{Language: string(Go)}, nil
	}
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	validation, err := b.Validate(ctx, snapshot, ValidationOptions{
		Scope: scope,
		Kinds: assessmentValidationKinds(opts),
	})
	if err != nil {
		return AssessmentReport{}, err
	}
	proposals, err := b.Suggest(ctx, snapshot, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	units := topGoAssessmentUnits(computeMetrics(idx), optionLimit(opts.TopUnitLimit, 8))
	pressure := 0.0
	if len(units) > 0 {
		pressure = units[0].PressureScore
	}
	diagnostics := append([]Diagnostic(nil), idx.diagnostics...)
	diagnostics = append(diagnostics, validation.Diagnostics...)
	architecturePolicy, err := goArchitecturePolicyAssessment(ctx, snapshot, idx, opts)
	if err != nil {
		return AssessmentReport{}, err
	}
	findings := goAssessmentFindings(idx, units, validation, opts)
	findings = append(findings, architecturePolicy.findings...)
	sortFindings(findings)
	violations := goAssessmentViolations(validation, opts)
	violations = append(violations, goArchitectureViolations(idx, opts)...)
	violations = append(violations, architecturePolicy.violations...)
	sortViolations(violations)
	executable := 0
	for _, proposal := range proposals {
		if len(proposal.Operations) > 0 {
			executable++
		}
	}
	scores := goAssessmentScores(validation.Passed, findings, violations, len(proposals), pressure, opts)
	return AssessmentReport{
		Language: string(Go),
		Summary: AssessmentSummary{
			Score:           scores.Overall,
			Packages:        len(idx.packages),
			Symbols:         len(idx.symbols),
			Imports:         len(idx.imports),
			Suggestions:     len(proposals),
			ExecutableFixes: executable,
			Findings:        len(findings),
			Violations:      len(violations),
			Diagnostics:     len(diagnostics),
		},
		Scores: scores,
		Validation: ValidationSummary{
			Passed:         validation.Passed,
			ResolutionMode: validation.ResolutionMode,
			Diagnostics:    len(validation.Diagnostics),
			Files:          len(validation.AffectedPaths),
			Complete:       validation.Complete,
		},
		Findings:    findings,
		Violations:  violations,
		TopUnits:    units,
		Suggestions: summarizeGoAssessmentSuggestions(proposals, opts.SuggestionLimit),
		Diagnostics: diagnostics,
		Metrics:     goAssessmentMetrics(idx, opts),
	}, nil
}

func goAssessmentMetrics(idx *index, opts AssessmentOptions) map[string]interface{} {
	metrics := map[string]interface{}{
		"score_model":        "go-architecture-v0",
		"gates":              normalizedAssessmentGates(opts.Gates),
		"debt_marker_count":  len(idx.debtMarkers),
		"debt_marker_counts": core.CountDebtMarkers(idx.debtMarkers),
	}
	for key, value := range idx.quality.metrics.assessmentMetrics() {
		metrics[key] = value
	}
	return metrics
}

func goAssessmentFindings(idx *index, units []UnitMetrics, validation ValidationResult, opts AssessmentOptions) []Finding {
	var out []Finding
	if assessmentGateEnabled(opts, AssessmentGateCoverage) && len(idx.documents) == 0 {
		out = append(out, Finding{
			Kind:     "coverage_no_go_files",
			Severity: "warning",
			Reason:   "No Go files were indexed for the selected scope.",
		})
	}
	if assessmentGateEnabled(opts, AssessmentGateArchitecture) {
		for _, metric := range units {
			if metric.DirectFanOut < 20 {
				continue
			}
			out = append(out, Finding{
				Kind:     "architecture_high_fan_out",
				Severity: "warning",
				Package:  metric.UnitID,
				Evidence: metric.Evidence,
				Reason:   fmt.Sprintf("Unit has %d direct imports; review dependency direction and boundary pressure.", metric.DirectFanOut),
			})
		}
		for _, imp := range idx.imports {
			if !looksLikeInternalBoundaryImport(imp) {
				continue
			}
			out = append(out, Finding{
				Kind:     "architecture_internal_import",
				Severity: "info",
				Package:  imp.FromUnit,
				Location: imp.Location,
				Allowed:  true,
				Reason:   "Import crosses an internal package boundary; this may be intentional but should be explicit in architecture rules.",
			})
		}
	}
	if assessmentGateEnabled(opts, AssessmentGateMaintainability) {
		for _, metric := range units {
			if metric.PressureScore < 100 {
				continue
			}
			out = append(out, Finding{
				Kind:     "maintainability_high_pressure_unit",
				Severity: pressureSeverity(metric.PressureScore),
				Package:  metric.UnitID,
				Evidence: metric.Evidence,
				Reason:   "High pressure is based on fan-in, call fan-in, public symbols, files, and implementation edges.",
			})
		}
		out = append(out, goDebtMarkerFindings(idx)...)
	}
	out = append(out, goQualityFindings(idx, opts)...)
	if assessmentGateEnabled(opts, AssessmentGateSafety) && !validation.Complete {
		out = append(out, Finding{
			Kind:     "safety_incomplete_validation",
			Severity: "warning",
			Reason:   "Validation completed with incomplete language facts.",
		})
	}
	sortFindings(out)
	return out
}

func goQualityFindings(idx *index, opts AssessmentOptions) []Finding {
	if len(idx.quality.findings) == 0 {
		return nil
	}
	all := assessmentGateEnabled(opts, AssessmentGateAll)
	maintainability := assessmentGateEnabled(opts, AssessmentGateMaintainability)
	safety := assessmentGateEnabled(opts, AssessmentGateSafety)
	coverage := assessmentGateEnabled(opts, AssessmentGateCoverage)
	out := make([]Finding, 0, len(idx.quality.findings))
	for _, finding := range idx.quality.findings {
		switch {
		case all:
			out = append(out, finding)
		case strings.HasPrefix(finding.Kind, "quality_") || strings.HasPrefix(finding.Kind, "performance_"):
			if maintainability {
				out = append(out, finding)
			}
		case strings.HasPrefix(finding.Kind, "safety_") || strings.HasPrefix(finding.Kind, "security_"):
			if safety {
				out = append(out, finding)
			}
		case strings.HasPrefix(finding.Kind, "coverage_"):
			if coverage {
				out = append(out, finding)
			}
		}
	}
	return out
}

func goAssessmentViolations(validation ValidationResult, opts AssessmentOptions) []Violation {
	var out []Violation
	if assessmentGateEnabled(opts, AssessmentGateSafety) {
		for _, diagnostic := range validation.Diagnostics {
			out = append(out, Violation{
				Kind:     "safety_validation_diagnostic",
				Severity: diagnostic.Severity,
				Location: diagnostic.Location,
				Reason:   diagnostic.Message,
			})
		}
	}
	return out
}

func goDebtMarkerFindings(idx *index) []Finding {
	out := make([]Finding, 0, len(idx.debtMarkers))
	for _, marker := range idx.debtMarkers {
		out = append(out, Finding{
			Kind:     "maintainability_debt_marker",
			Severity: debtMarkerSeverity(marker.Marker),
			Location: marker.Location,
			Package:  idx.fileUnits[marker.Location.URI],
			Symbol:   marker.Marker,
			Evidence: []Evidence{{
				Kind:     "debt_marker",
				Message:  marker.Text,
				Location: marker.Location,
				Metrics:  map[string]float64{"count": 1},
			}},
			Reason: fmt.Sprintf("%s marker should be reviewed before publishing or automated cleanup.", marker.Marker),
		})
	}
	return out
}

func debtMarkerSeverity(marker string) string {
	switch marker {
	case "FIXME", "HACK", "XXX", "DEPRECATED":
		return "warning"
	default:
		return "info"
	}
}

func goArchitectureViolations(idx *index, opts AssessmentOptions) []Violation {
	if !assessmentGateEnabled(opts, AssessmentGateArchitecture) || opts.Architecture == nil {
		return nil
	}
	out := make([]Violation, 0)
	for _, imp := range idx.imports {
		if rule, denied := deniedArchitectureImport(imp, opts.Architecture.Imports); denied {
			out = append(out, architectureImportViolation("architecture_denied_import", imp, rule))
		}
		if rule, denied := deniedArchitectureImport(imp, opts.Architecture.TestImports); denied {
			out = append(out, architectureImportViolation("architecture_test_boundary_import", imp, rule))
		}
	}
	return out
}

func deniedArchitectureImport(imp ImportEdge, rules []ArchitectureImportRule) (ArchitectureImportRule, bool) {
	rule, ok := selectedArchitectureRule(imp, rules)
	if !ok {
		return ArchitectureImportRule{}, false
	}
	action := rule.Action
	if action == "" {
		action = ArchitectureRuleDeny
	}
	return rule, action == ArchitectureRuleDeny
}

func selectedArchitectureRule(imp ImportEdge, rules []ArchitectureImportRule) (ArchitectureImportRule, bool) {
	var best ArchitectureImportRule
	bestSpecificity := -1
	matched := false
	for _, rule := range rules {
		if !architectureRuleMatches(imp, rule) {
			continue
		}
		action := rule.Action
		if action == "" {
			action = ArchitectureRuleDeny
		}
		if action != ArchitectureRuleAllow && action != ArchitectureRuleDeny {
			continue
		}
		specificity := len(rule.From) + len(rule.To)
		if !matched || specificity > bestSpecificity || specificity == bestSpecificity && action == ArchitectureRuleDeny && best.Action != ArchitectureRuleDeny {
			best = rule
			bestSpecificity = specificity
			matched = true
		}
	}
	return best, matched
}

func architectureRuleMatches(imp ImportEdge, rule ArchitectureImportRule) bool {
	if rule.From != "" && !architectureRulePrefix(rule.From, imp.FromUnit) && !architectureRulePrefix(rule.From, packageDir(imp.FromUnit)) && !architectureRulePrefix(rule.From, imp.FromPath) {
		return false
	}
	if rule.To != "" && !architectureRulePrefix(rule.To, imp.Import) {
		return false
	}
	return true
}

func architectureRulePrefix(prefix, value string) bool {
	return value == prefix || strings.HasPrefix(value, prefix+"/") || strings.HasPrefix(value, prefix+"#")
}

func architectureImportViolation(kind string, imp ImportEdge, rule ArchitectureImportRule) Violation {
	reason := rule.Reason
	if reason == "" {
		reason = fmt.Sprintf("Import %q is denied from %q by architecture rules.", imp.Import, imp.FromUnit)
	}
	return Violation{
		Kind:     kind,
		Severity: "error",
		Package:  imp.FromUnit,
		Location: imp.Location,
		Reason:   reason,
	}
}

func goAssessmentScores(validationPassed bool, findings []Finding, violations []Violation, suggestions int, pressure float64, opts AssessmentOptions) ScoreSet {
	boundary := 100 - minAssessmentInt(40, countFindings(findings, "architecture_")*10)
	testBoundary := 100
	if opts.Architecture != nil {
		boundaryViolations := countViolations(violations, "architecture_denied_import") + countViolations(violations, "architecture_boundary_violation")
		testViolations := countViolations(violations, "architecture_test_boundary_import") + countViolations(violations, "architecture_test_boundary_violation")
		boundary = 100 - minAssessmentInt(100, boundaryViolations*25)
		testBoundary = 100 - minAssessmentInt(100, testViolations*10)
	}
	coupling := 100 - minAssessmentInt(35, countFindings(findings, "architecture_high_fan_out")*5)
	if opts.Architecture != nil {
		if opts.Architecture.Coupling.FanOutThreshold > 0 {
			coupling = 100 - minAssessmentInt(40, countUnallowedFindings(findings, "architecture_fan_out")*2)
		}
	}
	sideEffect := 100
	coverage := 100 - minAssessmentInt(50, countFindings(findings, "coverage_")*25)
	if opts.Architecture != nil {
		sideEffect = 100 - minAssessmentInt(60, countArchitectureEffectViolations(violations, opts.Architecture)*10)
		coverage = minAssessmentInt(coverage, 100-minAssessmentInt(100, countViolations(violations, "architecture_unknown_package")*20))
	}
	debtMarkers := countFindings(findings, "maintainability_debt_marker")
	qualityFindings := countFindings(findings, "quality_") + countFindings(findings, "performance_")
	safetyFindings := countFindings(findings, "safety_") + countFindings(findings, "security_")
	maintainability := 100 - minAssessmentInt(40, suggestions/5) - minAssessmentInt(20, int(pressure/100)) - minAssessmentInt(20, debtMarkers*2) - minAssessmentInt(25, qualityFindings*3)
	if maintainability < 50 {
		maintainability = 50
	}
	if assessmentGateEnabled(opts, AssessmentGateSafety) && !validationPassed {
		sideEffect = minAssessmentInt(sideEffect, 50)
	}
	if safetyFindings > 0 {
		sideEffect = minAssessmentInt(sideEffect, 100-minAssessmentInt(40, safetyFindings*5))
	}
	overall := minAssessmentInt(boundary, maintainability)
	overall = minAssessmentInt(overall, coverage)
	overall = minAssessmentInt(overall, coupling)
	overall = minAssessmentInt(overall, testBoundary)
	if opts.Architecture != nil && boundary == 100 && assessmentOnlyArchitecture(opts) {
		softImpact := ceilAssessmentDiv(100-coupling, 10) + ceilAssessmentDiv(100-sideEffect, 20) + ceilAssessmentDiv(100-coverage, 20) + ceilAssessmentDiv(100-testBoundary, 20)
		overall = 100 - minAssessmentInt(10, softImpact)
	} else if opts.Architecture != nil && boundary < 100 && assessmentOnlyArchitecture(opts) {
		overall = boundary
	} else {
		overall -= minAssessmentInt(30, len(violations)*10)
	}
	if overall < 0 {
		overall = 0
	}
	return ScoreSet{
		Overall:         overall,
		Boundary:        boundary,
		TestBoundary:    testBoundary,
		Coupling:        coupling,
		SideEffect:      sideEffect,
		Coverage:        coverage,
		Maintainability: maintainability,
		Pressure:        pressure,
	}
}

func summarizeGoAssessmentSuggestions(proposals []Proposal, limit int) []AssessmentSuggestion {
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

func topGoAssessmentUnits(units []UnitMetrics, limit int) []UnitMetrics {
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

func assessmentGateEnabled(opts AssessmentOptions, gate AssessmentGate) bool {
	gates := normalizedAssessmentGates(opts.Gates)
	for _, candidate := range gates {
		if candidate == AssessmentGateAll || candidate == gate {
			return true
		}
	}
	return false
}

func assessmentValidationKinds(opts AssessmentOptions) []ValidationKind {
	if assessmentGateEnabled(opts, AssessmentGateSafety) {
		return []ValidationKind{ValidationParse, ValidationTypecheck}
	}
	return []ValidationKind{ValidationParse}
}

func normalizedAssessmentGates(gates []AssessmentGate) []AssessmentGate {
	if len(gates) == 0 {
		return []AssessmentGate{AssessmentGateAll}
	}
	seen := map[AssessmentGate]bool{}
	out := make([]AssessmentGate, 0, len(gates))
	for _, gate := range gates {
		if gate == "" || seen[gate] {
			continue
		}
		seen[gate] = true
		out = append(out, gate)
	}
	if len(out) == 0 {
		return []AssessmentGate{AssessmentGateAll}
	}
	return out
}

func looksLikeInternalBoundaryImport(imp ImportEdge) bool {
	return strings.Contains(imp.Import, "/internal/") && !isInternalPackageDir(packageDir(imp.FromUnit))
}

func isInternalPackageDir(dir string) bool {
	return dir == "internal" || strings.HasPrefix(dir, "internal/") || strings.Contains(dir, "/internal/")
}

func pressureSeverity(score float64) string {
	if score >= 500 {
		return "warning"
	}
	return "info"
}

func countFindings(findings []Finding, prefix string) int {
	n := 0
	for _, finding := range findings {
		if strings.HasPrefix(finding.Kind, prefix) {
			n++
		}
	}
	return n
}

func countUnallowedFindings(findings []Finding, prefix string) int {
	n := 0
	for _, finding := range findings {
		if strings.HasPrefix(finding.Kind, prefix) && !finding.Allowed {
			n++
		}
	}
	return n
}

func countViolations(violations []Violation, kind string) int {
	n := 0
	for _, violation := range violations {
		if violation.Kind == kind {
			n++
		}
	}
	return n
}

func countViolationsPrefix(violations []Violation, prefix string) int {
	n := 0
	for _, violation := range violations {
		if strings.HasPrefix(violation.Kind, prefix) {
			n++
		}
	}
	return n
}

func countArchitectureEffectViolations(violations []Violation, rules *ArchitectureRules) int {
	effectKinds := map[string]bool{
		"architecture_effect_import": true,
		"architecture_effect_call":   true,
	}
	if rules != nil {
		for _, rule := range rules.Effects {
			effectKinds[architectureEffectKind(rule, "architecture_effect_import")] = true
			effectKinds[architectureEffectKind(rule, "architecture_effect_call")] = true
		}
	}
	n := 0
	for _, violation := range violations {
		if effectKinds[violation.Kind] && violation.Severity == "error" {
			n++
		}
	}
	return n
}

func assessmentOnlyArchitecture(opts AssessmentOptions) bool {
	gates := normalizedAssessmentGates(opts.Gates)
	return len(gates) == 1 && gates[0] == AssessmentGateArchitecture
}

func ceilAssessmentDiv(n, d int) int {
	if n <= 0 {
		return 0
	}
	return (n + d - 1) / d
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].Package != findings[j].Package {
			return findings[i].Package < findings[j].Package
		}
		return findings[i].Location.URI < findings[j].Location.URI
	})
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Severity != violations[j].Severity {
			return violations[i].Severity > violations[j].Severity
		}
		if violations[i].Kind != violations[j].Kind {
			return violations[i].Kind < violations[j].Kind
		}
		return violations[i].Location.URI < violations[j].Location.URI
	})
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
