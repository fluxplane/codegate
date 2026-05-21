package goast

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
		Kinds: []ValidationKind{ValidationParse, ValidationTypecheck},
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
	findings := goAssessmentFindings(idx, units, validation, opts)
	violations := goAssessmentViolations(validation, opts)
	executable := 0
	for _, proposal := range proposals {
		if len(proposal.Operations) > 0 {
			executable++
		}
	}
	scores := goAssessmentScores(validation.Passed, findings, violations, len(proposals), pressure)
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
		Metrics: map[string]interface{}{
			"score_model": "go-architecture-v0",
			"gates":       normalizedAssessmentGates(opts.Gates),
		},
	}, nil
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
	}
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

func goAssessmentViolations(validation ValidationResult, opts AssessmentOptions) []Violation {
	if !assessmentGateEnabled(opts, AssessmentGateSafety) {
		return nil
	}
	var out []Violation
	for _, diagnostic := range validation.Diagnostics {
		out = append(out, Violation{
			Kind:     "safety_validation_diagnostic",
			Severity: diagnostic.Severity,
			Location: diagnostic.Location,
			Reason:   diagnostic.Message,
		})
	}
	sortViolations(out)
	return out
}

func goAssessmentScores(validationPassed bool, findings []Finding, violations []Violation, suggestions int, pressure float64) ScoreSet {
	boundary := 100 - minAssessmentInt(40, countFindings(findings, "architecture_")*10)
	testBoundary := 100
	coupling := 100 - minAssessmentInt(35, countFindings(findings, "architecture_high_fan_out")*5)
	sideEffect := 100
	coverage := 100 - minAssessmentInt(50, countFindings(findings, "coverage_")*25)
	maintainability := 100 - minAssessmentInt(40, suggestions/5) - minAssessmentInt(20, int(pressure/100))
	if maintainability < 50 {
		maintainability = 50
	}
	if !validationPassed {
		sideEffect = minAssessmentInt(sideEffect, 50)
	}
	overall := minAssessmentInt(boundary, maintainability)
	overall = minAssessmentInt(overall, coverage)
	overall = minAssessmentInt(overall, coupling)
	overall -= minAssessmentInt(30, len(violations)*10)
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
