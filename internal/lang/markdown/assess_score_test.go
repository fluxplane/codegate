package markdown

import "testing"

func TestMarkdownScoresNormalizeStructureFindingsByDocumentSize(t *testing.T) {
	findings := markdownTestFindings("markdown_heading_level_jump", "warning", 4)

	small := markdownScores(true, findings, nil, markdownScoreMetrics{DocumentCount: 1, HeadingCount: 4, LineCount: 40})
	large := markdownScores(true, findings, nil, markdownScoreMetrics{DocumentCount: 4, HeadingCount: 40, LineCount: 400})
	if large.Maintainability <= small.Maintainability {
		t.Fatalf("expected same structure finding count in larger docs to score better, large=%#v small=%#v", large, small)
	}
}

func TestMarkdownScoresNormalizeDebtByLineCount(t *testing.T) {
	findings := markdownTestFindings("maintainability_debt_marker", "info", 4)

	small := markdownScores(true, findings, nil, markdownScoreMetrics{DocumentCount: 1, HeadingCount: 1, LineCount: 40})
	large := markdownScores(true, findings, nil, markdownScoreMetrics{DocumentCount: 1, HeadingCount: 1, LineCount: 400})
	if large.Maintainability <= small.Maintainability {
		t.Fatalf("expected same debt marker count in larger docs to score better, large=%#v small=%#v", large, small)
	}
}

func TestMarkdownScoresWeightsInfoFindingsBelowWarnings(t *testing.T) {
	metrics := markdownScoreMetrics{DocumentCount: 2, HeadingCount: 20, LineCount: 200}

	warnings := markdownTestFindings("markdown_heading_level_jump", "warning", 4)
	info := markdownTestFindings("markdown_empty_section", "info", 4)
	warningScore := markdownScores(true, warnings, nil, metrics)
	infoScore := markdownScores(true, info, nil, metrics)
	if infoScore.Maintainability <= warningScore.Maintainability {
		t.Fatalf("expected info findings to have lower maintainability impact than warnings, info=%#v warnings=%#v", infoScore, warningScore)
	}
}

func TestMarkdownScoresNormalizeViolationPenaltyByLineCount(t *testing.T) {
	violations := []Violation{{Kind: "safety_validation_diagnostic", Severity: "error"}}

	small := markdownScores(true, nil, violations, markdownScoreMetrics{DocumentCount: 1, HeadingCount: 1, LineCount: 20})
	large := markdownScores(true, nil, violations, markdownScoreMetrics{DocumentCount: 1, HeadingCount: 1, LineCount: 200})
	if large.Overall <= small.Overall {
		t.Fatalf("expected same violation count in larger docs to score better, large=%#v small=%#v", large, small)
	}
}

func markdownTestFindings(kind, severity string, n int) []Finding {
	out := make([]Finding, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Finding{Kind: kind, Severity: severity})
	}
	return out
}
