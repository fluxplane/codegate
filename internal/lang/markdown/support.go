package markdown

func markdownAssessmentSupport() AssessmentSupport {
	return AssessmentSupport{
		Gates: []AssessmentGate{AssessmentGateMaintainability, AssessmentGateSafety, AssessmentGateCoverage},
		Metrics: []MetricSupport{
			{ID: "score_model", Category: "reporting", Level: CapabilityBasic, Description: "Backend assessment scoring model identifier."},
			{ID: "gates", Category: "reporting", Level: CapabilityBasic, Description: "Assessment gates applied to the report."},
			{ID: "debt_marker_count", Category: "maintainability", Level: CapabilityBasic, Description: "Total TODO/FIXME/HACK/XXX/DEPRECATED markers in Markdown prose."},
			{ID: "debt_marker_counts", Category: "maintainability", Level: CapabilityBasic, Description: "Debt marker counts grouped by marker kind."},
		},
		Findings: []FindingSupport{
			{ID: "coverage_no_markdown_files", Category: "coverage", Level: CapabilityBasic, Description: "No Markdown files were indexed for the selected scope."},
			{ID: "maintainability_debt_marker", Category: "maintainability", Level: CapabilityBasic, Description: "Markdown prose contains TODO/FIXME/HACK/XXX/DEPRECATED debt marker."},
			{ID: "markdown_heading_level_jump", Category: "maintainability", Level: CapabilityBasic, Description: "Heading level skips over its parent level."},
			{ID: "markdown_large_section", Category: "maintainability", Level: CapabilityBasic, Description: "Section exceeds the configured line threshold."},
			{ID: "markdown_empty_section", Category: "maintainability", Level: CapabilityBasic, Description: "Heading has no body content before the next section."},
			{ID: "markdown_missing_h1", Category: "maintainability", Level: CapabilityBasic, Description: "Document has no H1 title."},
			{ID: "markdown_multiple_h1", Category: "maintainability", Level: CapabilityBasic, Description: "Document has multiple H1 titles."},
			{ID: "markdown_duplicate_heading_anchor", Category: "maintainability", Level: CapabilityBasic, Description: "Multiple headings generate the same local anchor."},
			{ID: "markdown_broken_local_heading_link", Category: "maintainability", Level: CapabilityBasic, Description: "Local heading link does not resolve in the document."},
		},
		Violations: []ViolationSupport{
			{ID: "safety_validation_diagnostic", Category: "safety", Level: CapabilityBasic, Description: "Markdown validation diagnostic."},
		},
	}
}
