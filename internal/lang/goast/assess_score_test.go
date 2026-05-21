package goast

import "testing"

func TestGoAssessmentScoresNormalizeQualityFindingsByLOC(t *testing.T) {
	metrics := goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}
	topUnit := UnitMetrics{LOC: 5_000, PressureScore: 1_000}

	fewer := goAssessmentScores(true, testFindings("quality_high_complexity_function", 50), nil, nil, topUnit.PressureScore, AssessmentOptions{}, metrics, topUnit)
	more := goAssessmentScores(true, testFindings("quality_high_complexity_function", 100), nil, nil, topUnit.PressureScore, AssessmentOptions{}, metrics, topUnit)
	if fewer.Maintainability <= more.Maintainability {
		t.Fatalf("expected removing quality findings to improve maintainability, fewer=%#v more=%#v", fewer, more)
	}

	scaledMetrics := goQualityMetrics{TotalNonCommentLOC: 20_000, TotalFunctionCount: 200}
	scaledTopUnit := UnitMetrics{LOC: 10_000, PressureScore: 2_000}
	scaled := goAssessmentScores(true, testFindings("quality_high_complexity_function", 100), nil, nil, scaledTopUnit.PressureScore, AssessmentOptions{}, scaledMetrics, scaledTopUnit)
	if scaled.Maintainability != fewer.Maintainability {
		t.Fatalf("expected same finding and pressure density to preserve maintainability, scaled=%#v baseline=%#v", scaled, fewer)
	}
}

func TestGoAssessmentScoresPenalizeFindingDensity(t *testing.T) {
	findings := testFindings("quality_high_complexity_function", 50)
	topUnit := UnitMetrics{LOC: 5_000, PressureScore: 1_000}

	small := goAssessmentScores(true, findings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 2_000, TotalFunctionCount: 20}, topUnit)
	large := goAssessmentScores(true, findings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}, topUnit)
	if large.Maintainability <= small.Maintainability {
		t.Fatalf("expected same finding count in a larger codebase to score better, large=%#v small=%#v", large, small)
	}
}

func TestGoAssessmentScoresNormalizePressureByUnitLOC(t *testing.T) {
	metrics := goQualityMetrics{TotalNonCommentLOC: 20_000, TotalFunctionCount: 200}
	findings := testFindings("quality_high_complexity_function", 10)

	baselineTop := UnitMetrics{LOC: 5_000, PressureScore: 5_000}
	baseline := goAssessmentScores(true, findings, nil, nil, baselineTop.PressureScore, AssessmentOptions{}, metrics, baselineTop)

	scaledTop := UnitMetrics{LOC: 10_000, PressureScore: 10_000}
	scaled := goAssessmentScores(true, findings, nil, nil, scaledTop.PressureScore, AssessmentOptions{}, metrics, scaledTop)
	if scaled.Maintainability != baseline.Maintainability {
		t.Fatalf("expected same pressure density to preserve maintainability, scaled=%#v baseline=%#v", scaled, baseline)
	}

	lowerPressureTop := UnitMetrics{LOC: 10_000, PressureScore: 5_000}
	lowerPressure := goAssessmentScores(true, findings, nil, nil, lowerPressureTop.PressureScore, AssessmentOptions{}, metrics, lowerPressureTop)
	if goPressurePenalty(lowerPressureTop, lowerPressureTop.PressureScore, metrics) >= goPressurePenalty(baselineTop, baselineTop.PressureScore, metrics) {
		t.Fatalf("expected lower pressure density to reduce pressure penalty, lower=%#v baseline=%#v", lowerPressure, baseline)
	}
	if lowerPressure.Maintainability < baseline.Maintainability {
		t.Fatalf("expected lower pressure density to improve maintainability, lower=%#v baseline=%#v", lowerPressure, baseline)
	}
}

func TestGoAssessmentScoresNormalizeCouplingByLOC(t *testing.T) {
	findings := testFindings("architecture_high_fan_out", 4)
	topUnit := UnitMetrics{LOC: 5_000, PressureScore: 1_000}

	baseline := goAssessmentScores(true, findings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}, topUnit)
	scaled := goAssessmentScores(true, testFindings("architecture_high_fan_out", 8), nil, nil, topUnit.PressureScore, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 20_000, TotalFunctionCount: 200}, topUnit)
	if scaled.Coupling != baseline.Coupling {
		t.Fatalf("expected same high-fan-out density to preserve coupling, scaled=%#v baseline=%#v", scaled, baseline)
	}

	lowerDensity := goAssessmentScores(true, findings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 20_000, TotalFunctionCount: 200}, topUnit)
	if lowerDensity.Coupling <= baseline.Coupling {
		t.Fatalf("expected lower high-fan-out density to improve coupling, lower=%#v baseline=%#v", lowerDensity, baseline)
	}
}

func TestGoAssessmentScoresWeightsInfoFindingsBelowWarnings(t *testing.T) {
	metrics := goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}
	topUnit := UnitMetrics{LOC: 5_000, PressureScore: 1_000}

	warnings := testFindingsWithSeverity("quality_high_complexity_function", "warning", 40)
	info := testFindingsWithSeverity("quality_undocumented_export", "info", 40)
	warningScore := goAssessmentScores(true, warnings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, metrics, topUnit)
	infoScore := goAssessmentScores(true, info, nil, nil, topUnit.PressureScore, AssessmentOptions{}, metrics, topUnit)
	if infoScore.Maintainability <= warningScore.Maintainability {
		t.Fatalf("expected info findings to have lower maintainability impact than warnings, info=%#v warnings=%#v", infoScore, warningScore)
	}
}

func TestGoAssessmentScoresAggregatesNormalizedMaintainabilityComponents(t *testing.T) {
	metrics := goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}
	topUnit := UnitMetrics{LOC: 5_000, PressureScore: 1_000}
	findings := testFindingsWithSeverity("quality_high_complexity_function", "warning", 40)

	score := goAssessmentScores(true, findings, nil, nil, topUnit.PressureScore, AssessmentOptions{}, metrics, topUnit)
	qualityScore := 100 - goQualityPenalty(findings, metrics)
	pressureScore := 100 - goPressurePenalty(topUnit, topUnit.PressureScore, metrics)
	if score.Maintainability < minAssessmentInt(qualityScore, pressureScore) {
		t.Fatalf("expected normalized components to aggregate as scores, maintainability=%d quality=%d pressure=%d", score.Maintainability, qualityScore, pressureScore)
	}
}

func TestGoAssessmentScoresNormalizeSafetyFindingsByLOC(t *testing.T) {
	findings := testFindingsWithSeverity("safety_ignored_error", "warning", 10)

	small := goAssessmentScores(true, findings, nil, nil, 0, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 1_000, TotalFunctionCount: 10}, UnitMetrics{})
	large := goAssessmentScores(true, findings, nil, nil, 0, AssessmentOptions{}, goQualityMetrics{TotalNonCommentLOC: 10_000, TotalFunctionCount: 100}, UnitMetrics{})
	if large.SideEffect <= small.SideEffect {
		t.Fatalf("expected same safety finding count in a larger codebase to score better, large=%#v small=%#v", large, small)
	}
}

func TestGoAssessmentScoresNormalizeArchitecturePolicySoftPenalties(t *testing.T) {
	rules := &ArchitectureRules{
		Coupling: ArchitectureCouplingRules{FanOutThreshold: 1},
		Effects:  []ArchitectureEffectRule{{Name: "network"}},
	}
	opts := AssessmentOptions{Architecture: rules}
	findings := []Finding{{Kind: "architecture_fan_out", Severity: "warning"}}
	violations := []Violation{
		{Kind: "architecture_effect_import", Severity: "error"},
		{Kind: "architecture_unknown_package", Severity: "error"},
	}

	small := goAssessmentScores(true, findings, violations, nil, 0, opts, goQualityMetrics{TotalNonCommentLOC: 20, TotalFunctionCount: 2}, UnitMetrics{})
	large := goAssessmentScores(true, findings, violations, nil, 0, opts, goQualityMetrics{TotalNonCommentLOC: 200, TotalFunctionCount: 20}, UnitMetrics{})
	if large.Coupling <= small.Coupling || large.SideEffect <= small.SideEffect || large.Coverage <= small.Coverage {
		t.Fatalf("expected same architecture policy signal count in a larger codebase to score better, large=%#v small=%#v", large, small)
	}
}

func testFindings(kind string, n int) []Finding {
	return testFindingsWithSeverity(kind, "", n)
}

func testFindingsWithSeverity(kind, severity string, n int) []Finding {
	out := make([]Finding, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Finding{Kind: kind, Severity: severity})
	}
	return out
}
