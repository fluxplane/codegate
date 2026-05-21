package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"

	"github.com/fluxplane/codegate"
)

//go:embed report.html
var assessmentReportTemplate string

var assessmentReportHTML = template.Must(template.New("assessment-report").Parse(assessmentReportTemplate))

type reportContext struct {
	Root        string
	Generator   string
	GeneratedAt time.Time
}

type assessmentReportView struct {
	Generator         string
	Generated         string
	Module            string
	Ref               string
	EmbeddedJSON      template.JS
	Cards             []reportCard
	RunDetails        []reportDetail
	ScoreRows         []reportTableRow
	FindingCounts     []reportCountRow
	FindingSeverity   []reportCountRow
	ViolationCounts   []reportCountRow
	ViolationSeverity []reportCountRow
	MetricRows        []reportTableRow
	Units             []reportUnitRow
	Violations        reportIssueTable
	Findings          reportIssueTable
	Diagnostics       reportDiagnosticTable
	Suggestions       reportSuggestionTable
}

type reportDetail struct {
	Label      string
	Value      string
	BadgeClass string
}

type reportTableRow struct {
	Key   string
	Value string
}

type reportCountRow struct {
	Key     string
	Value   int
	Percent int
}

type reportUnitRow struct {
	UnitID            string
	LOC               int
	FileCount         int
	DirectFanIn       int
	DirectFanOut      int
	CallFanIn         int
	CallFanOut        int
	PublicSymbolCount int
	PressureScore     string
}

type reportIssueTable struct {
	Note string
	Rows []reportIssueRow
}

type reportIssueRow struct {
	Kind     string
	Severity string
	Location string
	Package  string
	Symbol   string
	Reason   string
	Evidence []reportEvidenceRow
}

type reportDiagnosticTable struct {
	Note string
	Rows []reportDiagnosticRow
}

type reportDiagnosticRow struct {
	Severity string
	Location string
	Message  string
}

type reportSuggestionTable struct {
	Note string
	Rows []reportSuggestionRow
}

type reportSuggestionRow struct {
	ID         string
	Kind       string
	Title      string
	Summary    string
	Confidence string
	Risk       string
	Operations int
	Evidence   []reportEvidenceRow
}

type reportEvidenceRow struct {
	Kind     string
	Location string
	Message  string
	Metrics  string
	Language string
}

func timeNow() time.Time {
	return time.Now()
}

func assessmentRating(score int) string {
	switch {
	case score >= 98:
		return "A++"
	case score >= 95:
		return "A+"
	case score >= 90:
		return "A"
	case score >= 85:
		return "A-"
	case score >= 80:
		return "B++"
	case score >= 75:
		return "B+"
	case score >= 70:
		return "B"
	case score >= 65:
		return "B-"
	case score >= 60:
		return "C++"
	case score >= 55:
		return "C+"
	case score >= 50:
		return "C"
	case score >= 45:
		return "C-"
	case score >= 40:
		return "D++"
	case score >= 35:
		return "D+"
	case score >= 30:
		return "D"
	case score >= 25:
		return "D-"
	case score >= 20:
		return "F++"
	case score >= 15:
		return "F+"
	case score >= 10:
		return "F"
	case score >= 5:
		return "F-"
	default:
		return "F--"
	}
}

func renderAssessmentHTML(report codegate.AssessmentReport, ctx reportContext) (string, error) {
	if ctx.Generator == "" {
		ctx.Generator = "github.com/fluxplane/codegate"
	}
	if ctx.GeneratedAt.IsZero() {
		ctx.GeneratedAt = time.Now()
	}
	fullJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	embeddedJSON := strings.ReplaceAll(string(fullJSON), "</", "<\\/")
	meta := discoverReportMetadata(ctx.Root)
	rating := assessmentRating(report.Scores.Overall)
	var b bytes.Buffer
	view := assessmentReportView{
		Generator:         ctx.Generator,
		Generated:         ctx.GeneratedAt.Format(time.RFC3339),
		Module:            meta.Module,
		Ref:               meta.Ref,
		EmbeddedJSON:      template.JS(embeddedJSON),
		Cards:             reportCards(report, rating),
		RunDetails:        reportRunDetails(report),
		ScoreRows:         reportScoreRows(report.Scores),
		FindingCounts:     countRows(countIssues(report.Findings)),
		FindingSeverity:   countRows(countFindingSeverity(report.Findings)),
		ViolationCounts:   countRows(countViolations(report.Violations)),
		ViolationSeverity: countRows(countViolationSeverity(report.Violations)),
		MetricRows:        reportMetricRows(report.Metrics),
		Units:             reportUnitRows(report.TopUnits, 30),
		Violations:        reportViolations(report.Violations, 100),
		Findings:          reportFindings(report.Findings, 120),
		Diagnostics:       reportDiagnostics(report.Diagnostics, 100),
		Suggestions:       reportSuggestions(report.Suggestions, 80),
	}
	if err := assessmentReportHTML.Execute(&b, view); err != nil {
		return "", err
	}
	return b.String(), nil
}

type reportMetadata struct {
	Module string
	Ref    string
}

func discoverReportMetadata(root string) reportMetadata {
	if root == "" {
		root = "."
	}
	meta := reportMetadata{Module: "unknown", Ref: "unknown"}
	if module := readGoModuleName(root); module != "" {
		meta.Module = module
	}
	if ref := readGitRefName(root); ref != "" {
		meta.Ref = ref
	}
	return meta
}

func readGoModuleName(root string) string {
	src, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	f, err := modfile.Parse("go.mod", src, nil)
	if err != nil || f.Module == nil {
		return ""
	}
	return f.Module.Mod.Path
}

func readGitRefName(root string) string {
	gitDir := filepath.Join(root, ".git")
	if src, err := os.ReadFile(gitDir); err == nil && strings.HasPrefix(string(src), "gitdir:") {
		gitDir = strings.TrimSpace(strings.TrimPrefix(string(src), "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(root, gitDir)
		}
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(head))
	if strings.HasPrefix(value, "ref: ") {
		ref := strings.TrimPrefix(value, "ref: ")
		return strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
	}
	if len(value) >= 12 {
		return value[:12]
	}
	return value
}

type reportCard struct {
	Label string
	Value string
	Class string
}

func reportCards(report codegate.AssessmentReport, rating string) []reportCard {
	return []reportCard{
		{"Overall", fmt.Sprint(report.Scores.Overall), scoreClass(report.Scores.Overall)},
		{"Rating", rating, scoreClass(report.Scores.Overall)},
		{"Maintainability", fmt.Sprint(report.Scores.Maintainability), scoreClass(report.Scores.Maintainability)},
		{"Coverage", fmt.Sprint(report.Scores.Coverage), scoreClass(report.Scores.Coverage)},
		{"Coupling", fmt.Sprint(report.Scores.Coupling), scoreClass(report.Scores.Coupling)},
		{"Findings", fmt.Sprint(report.Summary.Findings), countClass(report.Summary.Findings)},
		{"Violations", fmt.Sprint(report.Summary.Violations), countClass(report.Summary.Violations)},
		{"Diagnostics", fmt.Sprint(report.Summary.Diagnostics), countClass(report.Summary.Diagnostics)},
		{"Suggestions", fmt.Sprint(report.Summary.Suggestions), "warn"},
	}
}

func reportRunDetails(report codegate.AssessmentReport) []reportDetail {
	validationClass := "bad"
	validationValue := "failed"
	if report.Validation.Passed {
		validationClass = "good"
		validationValue = "passed"
	}
	return []reportDetail{
		{Label: "Validation", Value: validationValue, BadgeClass: validationClass},
		{Label: "Resolution mode", Value: report.Validation.ResolutionMode},
		{Label: "Files indexed", Value: fmt.Sprint(report.Validation.Files)},
		{Label: "Packages", Value: fmt.Sprint(report.Summary.Packages)},
		{Label: "Symbols", Value: fmt.Sprint(report.Summary.Symbols)},
		{Label: "Imports", Value: fmt.Sprint(report.Summary.Imports)},
		{Label: "Executable fixes", Value: fmt.Sprint(report.Summary.ExecutableFixes)},
		{Label: "Pressure", Value: fmt.Sprintf("%.0f", report.Scores.Pressure)},
	}
}

func reportScoreRows(scores codegate.ScoreSet) []reportTableRow {
	return []reportTableRow{
		{Key: "overall", Value: fmt.Sprint(scores.Overall)},
		{Key: "boundary", Value: fmt.Sprint(scores.Boundary)},
		{Key: "test_boundary", Value: fmt.Sprint(scores.TestBoundary)},
		{Key: "coupling", Value: fmt.Sprint(scores.Coupling)},
		{Key: "side_effect", Value: fmt.Sprint(scores.SideEffect)},
		{Key: "coverage", Value: fmt.Sprint(scores.Coverage)},
		{Key: "maintainability", Value: fmt.Sprint(scores.Maintainability)},
		{Key: "pressure", Value: fmt.Sprintf("%.0f", scores.Pressure)},
	}
}

func reportMetricRows(metrics map[string]interface{}) []reportTableRow {
	keys := make([]string, 0, len(metrics))
	for key, value := range metrics {
		if scalarMetric(value) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	rows := make([]reportTableRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, reportTableRow{Key: key, Value: metricValue(metrics[key])})
	}
	return rows
}

func scalarMetric(value interface{}) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct:
		return false
	default:
		return true
	}
}

func metricValue(value interface{}) string {
	switch v := value.(type) {
	case float32:
		return fmt.Sprintf("%.2f", v)
	case float64:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprint(value)
	}
}

func countRows(counts map[string]int) []reportCountRow {
	type row struct {
		Key   string
		Value int
	}
	rows := make([]row, 0, len(counts))
	total := 0
	for key, value := range counts {
		rows = append(rows, row{Key: key, Value: value})
		total += value
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Value != rows[j].Value {
			return rows[i].Value > rows[j].Value
		}
		return rows[i].Key < rows[j].Key
	})
	out := make([]reportCountRow, 0, len(rows))
	for _, row := range rows {
		percent := 0
		if total > 0 {
			percent = row.Value * 100 / total
		}
		out = append(out, reportCountRow{Key: row.Key, Value: row.Value, Percent: percent})
	}
	return out
}

func reportUnitRows(units []codegate.UnitMetrics, limit int) []reportUnitRow {
	if len(units) > limit {
		units = units[:limit]
	}
	out := make([]reportUnitRow, 0, len(units))
	for _, unit := range units {
		out = append(out, reportUnitRow{
			UnitID:            unit.UnitID,
			LOC:               unit.LOC,
			FileCount:         unit.FileCount,
			DirectFanIn:       unit.DirectFanIn,
			DirectFanOut:      unit.DirectFanOut,
			CallFanIn:         unit.CallFanIn,
			CallFanOut:        unit.CallFanOut,
			PublicSymbolCount: unit.PublicSymbolCount,
			PressureScore:     fmt.Sprintf("%.0f", unit.PressureScore),
		})
	}
	return out
}

func reportFindings(items []codegate.Finding, limit int) reportIssueTable {
	rows := make([]reportIssueRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, reportIssueRow{
			Kind:     item.Kind,
			Severity: item.Severity,
			Location: locationString(item.Location),
			Package:  item.Package,
			Symbol:   item.Symbol,
			Reason:   item.Reason,
			Evidence: reportEvidenceRows(item.Evidence),
		})
	}
	return limitedIssueTable(rows, limit, "findings")
}

func reportViolations(items []codegate.Violation, limit int) reportIssueTable {
	rows := make([]reportIssueRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, reportIssueRow{
			Kind:     item.Kind,
			Severity: item.Severity,
			Location: locationString(item.Location),
			Package:  item.Package,
			Symbol:   item.Symbol,
			Reason:   item.Reason,
			Evidence: reportEvidenceRows(item.Evidence),
		})
	}
	return limitedIssueTable(rows, limit, "violations")
}

func limitedIssueTable(rows []reportIssueRow, limit int, label string) reportIssueTable {
	table := reportIssueTable{Rows: rows}
	if len(rows) > limit {
		table.Note = fmt.Sprintf("Showing first %d of %d %s.", limit, len(rows), label)
		table.Rows = rows[:limit]
	}
	return table
}

func reportDiagnostics(items []codegate.Diagnostic, limit int) reportDiagnosticTable {
	rows := make([]reportDiagnosticRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, reportDiagnosticRow{
			Severity: item.Severity,
			Location: locationString(item.Location),
			Message:  item.Message,
		})
	}
	table := reportDiagnosticTable{Rows: rows}
	if len(rows) > limit {
		table.Note = fmt.Sprintf("Showing first %d of %d diagnostics.", limit, len(rows))
		table.Rows = rows[:limit]
	}
	return table
}

func reportSuggestions(items []codegate.AssessmentSuggestion, limit int) reportSuggestionTable {
	rows := make([]reportSuggestionRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, reportSuggestionRow{
			ID:         item.ID,
			Kind:       string(item.Kind),
			Title:      item.Title,
			Summary:    item.Summary,
			Confidence: string(item.Confidence),
			Risk:       string(item.Risk),
			Operations: item.Operations,
			Evidence:   reportEvidenceRows(item.Evidence),
		})
	}
	table := reportSuggestionTable{Rows: rows}
	if len(rows) > limit {
		table.Note = fmt.Sprintf("Showing first %d of %d suggestions.", limit, len(rows))
		table.Rows = rows[:limit]
	}
	return table
}

func reportEvidenceRows(items []codegate.Evidence) []reportEvidenceRow {
	rows := make([]reportEvidenceRow, 0, len(items))
	for _, item := range items {
		metrics := evidenceMetrics(item.Metrics)
		message := strings.TrimSpace(item.Message)
		if item.Kind == "" && item.Location.URI == "" && message == "" && metrics == "" {
			continue
		}
		rows = append(rows, reportEvidenceRow{
			Kind:     item.Kind,
			Location: locationString(item.Location),
			Message:  message,
			Metrics:  metrics,
			Language: evidenceLanguage(item.Location, message),
		})
	}
	return rows
}

func evidenceMetrics(metrics map[string]float64) string {
	if len(metrics) == 0 {
		return ""
	}
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Sprint(metrics)
	}
	return string(data)
}

func evidenceLanguage(loc codegate.Location, message string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(loc.URI)), ".")
	switch ext {
	case "go":
		if looksLikeGoSnippet(message) {
			return "go"
		}
		return "plaintext"
	case "md":
		return "markdown"
	case "yml":
		return "yaml"
	case "java", "groovy", "js", "ts", "json", "markdown", "html", "css", "xml", "yaml", "sh", "bash":
		return ext
	case "":
		return "plaintext"
	default:
		return ext
	}
}

func looksLikeGoSnippet(message string) bool {
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	for _, token := range []string{
		"package ",
		"import ",
		"func ",
		"type ",
		"const ",
		"var ",
		"return ",
		"defer ",
		"if ",
		"for ",
		"switch ",
		":=",
		"_ =",
		"//",
		"/*",
		"{",
		"}",
		";",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	if strings.Contains(text, "(") && strings.Contains(text, ")") {
		return true
	}
	return false
}

func scoreClass(score int) string {
	if score >= 80 {
		return "good"
	}
	if score >= 50 {
		return "warn"
	}
	return "bad"
}

func countClass(n int) string {
	if n == 0 {
		return "good"
	}
	return "bad"
}

func countIssues(findings []codegate.Finding) map[string]int {
	out := map[string]int{}
	for _, item := range findings {
		out[item.Kind]++
	}
	return out
}

func countViolations(violations []codegate.Violation) map[string]int {
	out := map[string]int{}
	for _, item := range violations {
		out[item.Kind]++
	}
	return out
}

func countFindingSeverity(findings []codegate.Finding) map[string]int {
	out := map[string]int{}
	for _, item := range findings {
		out[item.Severity]++
	}
	return out
}

func countViolationSeverity(violations []codegate.Violation) map[string]int {
	out := map[string]int{}
	for _, item := range violations {
		out[item.Severity]++
	}
	return out
}

func locationString(loc codegate.Location) string {
	if loc.URI == "" {
		return ""
	}
	line := loc.Range.Start.Line
	if line > 0 {
		return fmt.Sprintf("%s:%d", loc.URI, line)
	}
	return loc.URI
}
