package markdown

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fluxplane/codegate/internal/core"
)

func (b MarkdownBackend) Assess(ctx context.Context, snapshot Snapshot, scope Scope, opts AssessmentOptions) (AssessmentReport, error) {
	if scope.Language != "" && scope.Language != Markdown {
		return AssessmentReport{Language: string(Markdown)}, nil
	}
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	validation, err := b.Validate(ctx, snapshot, ValidationOptions{Scope: scope, Kinds: []ValidationKind{ValidationParse}})
	if err != nil {
		return AssessmentReport{}, err
	}
	findings := markdownFindings(idx, opts)
	violations := markdownViolations(validation, opts)
	proposals, err := b.Suggest(ctx, snapshot, scope)
	if err != nil {
		return AssessmentReport{}, err
	}
	scores := markdownScores(validation.Passed, findings, violations)
	diagnostics := append([]Diagnostic(nil), idx.diagnostics...)
	diagnostics = append(diagnostics, validation.Diagnostics...)
	executable := 0
	for _, proposal := range proposals {
		if len(proposal.Operations) > 0 {
			executable++
		}
	}
	return AssessmentReport{
		Language: string(Markdown),
		Summary: AssessmentSummary{
			Score:           scores.Overall,
			Packages:        len(idx.packages),
			Symbols:         len(idx.symbols),
			Findings:        len(findings),
			Violations:      len(violations),
			Diagnostics:     len(diagnostics),
			Suggestions:     len(proposals),
			ExecutableFixes: executable,
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
		Suggestions: summarizeMarkdownAssessmentSuggestions(proposals, opts.SuggestionLimit),
		Diagnostics: diagnostics,
		Metrics:     markdownAssessmentMetrics(idx, opts),
	}, nil
}

func markdownAssessmentMetrics(idx *index, opts AssessmentOptions) map[string]interface{} {
	return map[string]interface{}{
		"score_model":        "markdown-structure-v0",
		"gates":              normalizedMarkdownAssessmentGates(opts.Gates),
		"debt_marker_count":  len(idx.debtMarkers),
		"debt_marker_counts": core.CountDebtMarkers(idx.debtMarkers),
	}
}

func summarizeMarkdownAssessmentSuggestions(proposals []Proposal, limit int) []AssessmentSuggestion {
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

func markdownFindings(idx *index, opts AssessmentOptions) []Finding {
	var out []Finding
	if markdownGateEnabled(opts, AssessmentGateCoverage) && len(idx.documents) == 0 {
		out = append(out, Finding{
			Kind:     "coverage_no_markdown_files",
			Severity: "warning",
			Reason:   "No Markdown files were indexed for the selected scope.",
		})
	}
	if markdownGateEnabled(opts, AssessmentGateMaintainability) {
		for _, file := range idx.files {
			out = append(out, markdownFileFindings(file)...)
		}
		out = append(out, markdownDebtMarkerFindings(idx)...)
	}
	sortFindings(out)
	return out
}

func markdownDebtMarkerFindings(idx *index) []Finding {
	out := make([]Finding, 0, len(idx.debtMarkers))
	for _, marker := range idx.debtMarkers {
		out = append(out, Finding{
			Kind:     "maintainability_debt_marker",
			Severity: markdownDebtMarkerSeverity(marker.Marker),
			Location: marker.Location,
			Package:  marker.Location.URI,
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

func markdownDebtMarkerSeverity(marker string) string {
	switch marker {
	case "FIXME", "HACK", "XXX", "DEPRECATED":
		return "warning"
	default:
		return "info"
	}
}

func markdownFileFindings(file markdownFile) []Finding {
	var out []Finding
	h1Count := 0
	anchorCounts := map[string]int{}
	for i, heading := range file.headings {
		if heading.level == 1 {
			h1Count++
		}
		if i > 0 && heading.level > file.headings[i-1].level+1 {
			out = append(out, Finding{
				Kind:     "markdown_heading_level_jump",
				Severity: "warning",
				Location: heading.location,
				Symbol:   heading.name,
				Reason:   fmt.Sprintf("Heading level jumps from H%d to H%d.", file.headings[i-1].level, heading.level),
			})
		}
		anchorCounts[heading.anchor]++
		lines := heading.sectionRange.End.Line - heading.sectionRange.Start.Line
		if lines > 80 {
			out = append(out, Finding{
				Kind:     "markdown_large_section",
				Severity: "info",
				Location: heading.location,
				Symbol:   heading.name,
				Reason:   fmt.Sprintf("Section spans %d lines; agents may need a smaller lookup target.", lines),
			})
		}
		if sectionText(file.src, heading) == "" {
			out = append(out, Finding{
				Kind:     "markdown_empty_section",
				Severity: "info",
				Location: heading.location,
				Symbol:   heading.name,
				Reason:   "Heading has no body content before the next section.",
			})
		}
	}
	if h1Count == 0 {
		out = append(out, Finding{
			Kind:     "markdown_missing_h1",
			Severity: "warning",
			Location: Location{URI: file.path},
			Reason:   "Document has no H1 title.",
		})
	}
	if h1Count > 1 {
		out = append(out, Finding{
			Kind:     "markdown_multiple_h1",
			Severity: "warning",
			Location: Location{URI: file.path},
			Reason:   fmt.Sprintf("Document has %d H1 titles.", h1Count),
		})
	}
	for anchor, count := range anchorCounts {
		if count <= 1 {
			continue
		}
		out = append(out, Finding{
			Kind:     "markdown_duplicate_heading_anchor",
			Severity: "warning",
			Location: Location{URI: file.path},
			Reason:   fmt.Sprintf("Anchor %q is generated by %d headings.", anchor, count),
		})
	}
	anchors := map[string]bool{}
	for _, heading := range file.headings {
		anchors[strings.TrimPrefix(heading.qualified, file.path+"#")] = true
		anchors[heading.anchor] = true
	}
	for _, link := range file.links {
		dest := strings.TrimSpace(link.destination)
		if dest == "" || anchors[dest] {
			continue
		}
		out = append(out, Finding{
			Kind:     "markdown_broken_local_heading_link",
			Severity: "warning",
			Location: link.location,
			Reason:   fmt.Sprintf("Local heading link #%s does not resolve in this document.", dest),
		})
	}
	return out
}

func markdownViolations(validation ValidationResult, opts AssessmentOptions) []Violation {
	if !markdownGateEnabled(opts, AssessmentGateSafety) {
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

func markdownScores(validationPassed bool, findings []Finding, violations []Violation) ScoreSet {
	boundary := 100
	testBoundary := 100
	coupling := 100
	sideEffect := 100
	coverage := 100 - minAssessmentInt(50, countMarkdownFindings(findings, "coverage_")*25)
	maintainability := 100 - minAssessmentInt(50, countMarkdownFindings(findings, "markdown_")*5) - minAssessmentInt(20, countMarkdownFindings(findings, "maintainability_debt_marker")*2)
	if maintainability < 50 {
		maintainability = 50
	}
	if !validationPassed {
		sideEffect = 50
	}
	overall := minAssessmentInt(boundary, maintainability)
	overall = minAssessmentInt(overall, coverage)
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
	}
}

func markdownGateEnabled(opts AssessmentOptions, gate AssessmentGate) bool {
	for _, candidate := range normalizedMarkdownAssessmentGates(opts.Gates) {
		if candidate == AssessmentGateAll || candidate == gate {
			return true
		}
	}
	return false
}

func normalizedMarkdownAssessmentGates(gates []AssessmentGate) []AssessmentGate {
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

func countMarkdownFindings(findings []Finding, prefix string) int {
	n := 0
	for _, finding := range findings {
		if strings.HasPrefix(finding.Kind, prefix) {
			n++
		}
	}
	return n
}

func sectionText(src []byte, heading headingInfo) string {
	start := heading.location.Range.End.Offset
	end := heading.sectionRange.End.Offset
	if start < 0 || end > len(src) || start > end {
		return ""
	}
	return strings.TrimSpace(string(src[start:end]))
}

func minAssessmentInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Location.URI != findings[j].Location.URI {
			return findings[i].Location.URI < findings[j].Location.URI
		}
		if findings[i].Location.Range.Start.Offset != findings[j].Location.Range.Start.Offset {
			return findings[i].Location.Range.Start.Offset < findings[j].Location.Range.Start.Offset
		}
		return findings[i].Kind < findings[j].Kind
	})
}

func sortViolations(violations []Violation) {
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].Location.URI != violations[j].Location.URI {
			return violations[i].Location.URI < violations[j].Location.URI
		}
		if violations[i].Location.Range.Start.Offset != violations[j].Location.Range.Start.Offset {
			return violations[i].Location.Range.Start.Offset < violations[j].Location.Range.Start.Offset
		}
		return violations[i].Kind < violations[j].Kind
	})
}
