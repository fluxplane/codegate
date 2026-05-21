package codegate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codewandler/codegate/internal/lang/goast"
	internalmarkdown "github.com/codewandler/codegate/internal/lang/markdown"
)

func TestEngineLookupAssessValidate(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

func Target() string {
	return helper()
}

func helper() string {
	return "ok"
}
`)

	ctx := context.Background()
	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}

	lookup, err := eng.Lookup(ctx, LookupQuery{Name: "Target", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Symbols) != 1 || lookup.Symbols[0].Name != "Target" || lookup.Confidence != HighConfidence {
		t.Fatalf("unexpected lookup result: %#v", lookup)
	}

	report, err := eng.Assess(ctx, AssessmentOptions{SuggestionLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Packages != 1 || report.Summary.Symbols == 0 || report.Validation.Passed != true {
		t.Fatalf("unexpected assessment: %#v", report)
	}

	validation, err := eng.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationParse, ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Passed {
		t.Fatalf("expected validation to pass: %#v", validation.Diagnostics)
	}
}

func TestEngineCapabilities(t *testing.T) {
	eng, err := New().
		WithFS(fstest.MapFS{}).
		WithLanguage(goast.New()).
		WithLanguage(internalmarkdown.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	specs := eng.Capabilities()
	if len(specs) != 2 || specs[0].Language != Go || specs[1].Language != Markdown {
		t.Fatalf("unexpected capabilities: %#v", specs)
	}
	if !hasCapability(specs[0], CapabilityLookup, CapabilityAdvanced) {
		t.Fatalf("go backend did not declare advanced lookup: %#v", specs[0].Capabilities)
	}
	if !hasCapability(specs[1], CapabilityLookup, CapabilityBasic) {
		t.Fatalf("markdown backend did not declare basic lookup: %#v", specs[1].Capabilities)
	}
	if !hasOperation(specs[0].Operations.ValidationKinds, ValidationTypecheck) {
		t.Fatalf("go backend did not declare typecheck validation: %#v", specs[0].Operations)
	}
	if !hasOperation(specs[1].Operations.EditOperations, OpMarkdownEnsureH1) || hasOperation(specs[1].Operations.ValidationKinds, ValidationTypecheck) {
		t.Fatalf("markdown backend did not declare expected operation detail: %#v", specs[1].Operations)
	}
	if !hasMetricSupport(specs[0], "max_cyclomatic_complexity") || !hasMetricSupport(specs[0], "ignored_error_count") || !hasMetricSupport(specs[0], "dynamic_exec_count") || !hasFindingSupport(specs[0], "quality_high_complexity_function") || !hasFindingSupport(specs[0], "security_dynamic_exec") {
		t.Fatalf("go backend did not declare expected assessment support: %#v", specs[0].Operations.Assessment)
	}
	if !hasMetricSupport(specs[1], "debt_marker_count") || !hasFindingSupport(specs[1], "markdown_broken_local_heading_link") {
		t.Fatalf("markdown backend did not declare expected assessment support: %#v", specs[1].Operations.Assessment)
	}
}

func TestEngineCapabilityMetricsCoverAssessmentOutput(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

// TODO: replace placeholder.
func Demo(v interface{}) {
	_ = v.(string)
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	specs := eng.Capabilities()
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected one backend spec, got %#v", specs)
	}
	for key := range report.Metrics {
		supported := key
		if key == "provider_score_model" {
			supported = "score_model"
		}
		if !hasMetricSupport(specs[0], supported) {
			t.Fatalf("assessment metric %q was not declared in capabilities: %#v", key, specs[0].Operations.Assessment.Metrics)
		}
	}
}

func TestEngineMarkdownLookupAssessValidate(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "README.md", `# Demo

See [Missing](#missing).

### Jumped

`)

	ctx := context.Background()
	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		WithLanguage(internalmarkdown.New()).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}

	lookup, err := eng.Lookup(ctx, LookupQuery{
		Language: Markdown,
		Name:     "Jumped",
		Kind:     SymbolNamespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Symbols) != 1 || lookup.Symbols[0].Language != Markdown || lookup.Symbols[0].QualifiedName != "README.md#jumped" {
		t.Fatalf("unexpected markdown lookup result: %#v", lookup)
	}

	positionLookup, err := eng.Lookup(ctx, LookupQuery{
		Language: Markdown,
		Path:     "README.md",
		Line:     3,
		Column:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionLookup.Symbols) != 1 || positionLookup.Symbols[0].Name != "Demo" || positionLookup.Target.NodeKind != "enclosing_symbol" {
		t.Fatalf("unexpected markdown position lookup result: %#v", positionLookup)
	}

	report, err := eng.Assess(ctx, AssessmentOptions{
		Scope: Scope{Language: Markdown},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["provider_score_model"] != "markdown-structure-v0" || !hasFinding(report, "markdown_heading_level_jump") || !hasFinding(report, "markdown_broken_local_heading_link") {
		t.Fatalf("unexpected markdown assessment: %#v", report)
	}

	validation, err := eng.Validate(ctx, ValidationOptions{Scope: Scope{Language: Markdown}, Kinds: []ValidationKind{ValidationParse}})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Passed || validation.ResolutionMode != "structural" || len(validation.AffectedPaths) != 1 {
		t.Fatalf("unexpected markdown validation: %#v", validation)
	}
}

func TestEngineMarkdownSuggestApplyValidateReassess(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "README.md", `Intro text.

## Parent

#### Setup

## Duplicate
content

## Duplicate

See [Missing](#missing).
`)

	ctx := context.Background()
	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(internalmarkdown.New()).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}

	report, err := eng.Assess(ctx, AssessmentOptions{
		Scope: Scope{Language: Markdown},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Suggestions == 0 || report.Summary.ExecutableFixes == 0 {
		t.Fatalf("expected executable markdown suggestions, got %#v", report)
	}

	proposals, err := eng.Suggest(ctx, SuggestOptions{Scope: Scope{Language: Markdown}})
	if err != nil {
		t.Fatal(err)
	}
	missingH1 := proposalWithEvidence(proposals, "markdown_missing_h1")
	if missingH1 == nil || len(missingH1.Operations) == 0 {
		t.Fatalf("expected executable missing-H1 proposal, got %#v", proposals)
	}
	brokenLink := proposalWithEvidence(proposals, "markdown_broken_local_heading_link")
	if brokenLink == nil || len(brokenLink.Operations) != 0 {
		t.Fatalf("expected broken-link proposal to remain advisory, got %#v", brokenLink)
	}
	headingJump := proposalWithEvidence(proposals, "markdown_heading_level_jump")
	if headingJump == nil || len(headingJump.Operations) == 0 {
		t.Fatalf("expected executable heading-jump proposal, got %#v", proposals)
	}
	emptySection := proposalWithEvidence(proposals, "markdown_empty_section")
	if emptySection == nil || len(emptySection.Operations) == 0 {
		t.Fatalf("expected executable empty-section proposal, got %#v", proposals)
	}
	duplicateHeading := proposalWithEvidence(proposals, "markdown_duplicate_heading_anchor")
	if duplicateHeading == nil || len(duplicateHeading.Operations) == 0 {
		t.Fatalf("expected executable duplicate-heading proposal, got %#v", proposals)
	}

	changes := eng.NewChangeSet()
	if err := changes.Apply(ctx, missingH1.Operations...); err != nil {
		t.Fatal(err)
	}
	validation, err := changes.Validate(ctx, ValidationOptions{Scope: Scope{Language: Markdown}, Kinds: []ValidationKind{ValidationParse}})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Passed {
		t.Fatalf("expected markdown validation to pass after fix, got %#v", validation)
	}
	diff, err := changes.Diff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+# README") {
		t.Fatalf("expected H1 in diff, got:\n%s", diff)
	}
	if err := changes.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	reassessed, err := eng.Assess(ctx, AssessmentOptions{Scope: Scope{Language: Markdown}, Gates: []AssessmentGate{AssessmentGateMaintainability}})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(reassessed, "markdown_missing_h1") {
		t.Fatalf("expected missing H1 finding to be fixed, got %#v", reassessed.Findings)
	}
}

func TestEngineMarkdownDebtMarkersSkipFencedCode(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "README.md", "# Demo\n\nTODO: write the overview.\n\nIgnore `FIXME` inline code.\n\n```go\n// FIXME: sample only\n```\n")

	ctx := context.Background()
	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(internalmarkdown.New()).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(ctx, AssessmentOptions{
		Scope:           Scope{Language: Markdown},
		Gates:           []AssessmentGate{AssessmentGateMaintainability},
		SuggestionLimit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := report.Metrics["debt_marker_count"]; count != 1 {
		t.Fatalf("expected one prose debt marker, got %#v in %#v", count, report.Metrics)
	}
	if !hasFinding(report, "maintainability_debt_marker") || !hasSuggestion(report, RefactorReviewDebtMarkers) {
		t.Fatalf("expected markdown debt marker finding and suggestion, got %#v %#v", report.Findings, report.Suggestions)
	}
}

func TestEngineAssessReportsMaintainabilityFindings(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	src := "package demo\n\n"
	for i := 0; i < 120; i++ {
		src += "func Exported" + string(rune('A'+i%26)) + string(rune('A'+(i/26)%26)) + "() {}\n"
	}
	writeEngineFile(t, root, "demo.go", src)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{Gates: []AssessmentGate{AssessmentGateMaintainability}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report, "maintainability_high_pressure_unit") {
		t.Fatalf("expected maintainability finding, got %#v", report.Findings)
	}
	if report.Summary.Findings != len(report.Findings) || report.Metrics["provider_score_model"] != "go-architecture-v0" {
		t.Fatalf("unexpected assessment summary/metrics: %#v", report)
	}
}

func TestEngineAssessReportsGoDebtMarkers(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

// TODO: replace temporary implementation.
func Target() string {
	// fixme: remove this branch.
	return "ok"
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope:           Scope{Language: Go},
		Gates:           []AssessmentGate{AssessmentGateMaintainability},
		SuggestionLimit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := report.Metrics["debt_marker_count"]; count != 2 {
		t.Fatalf("expected two debt markers, got %#v in %#v", count, report.Metrics)
	}
	if !hasFinding(report, "maintainability_debt_marker") || !hasSuggestion(report, RefactorReviewDebtMarkers) {
		t.Fatalf("expected debt marker finding and suggestion, got %#v %#v", report.Findings, report.Suggestions)
	}
}

func TestEngineAssessReportsGoQualityMetrics(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	src := `package demo

type LargeStruct struct {
`
	for i := 0; i < 21; i++ {
		src += "	F" + string(rune('A'+i)) + " int\n"
	}
	src += `}

type BroadInterface interface {
`
	for i := 0; i < 9; i++ {
		src += "	M" + string(rune('A'+i)) + "()\n"
	}
	src += `}

func Big() {
`
	for i := 0; i < 81; i++ {
		src += "	_ = " + string(rune('0'+i%10)) + "\n"
	}
	src += `}

func Complex(a, b, c, d, e, f int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				if d > 0 {
					if e > 0 {
						return 1
					}
				}
			}
		}
	}
	switch a {
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 4
	case 5:
		return 5
	}
	for i := 0; i < 3; i++ {
		if i%2 == 0 && a > 0 || b > 0 {
			return i
		}
	}
	return 0
}
`
	for i := 0; i < 430; i++ {
		src += "const C" + string(rune('A'+i%26)) + string(rune('A'+(i/26)%26)) + " = 1\n"
	}
	writeEngineFile(t, root, "quality.go", src)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{
		"quality_high_complexity_function",
		"quality_deeply_nested_function",
		"quality_many_parameters",
		"quality_many_returns",
		"quality_large_function",
		"quality_large_file",
		"quality_large_struct",
		"quality_broad_interface",
	} {
		if !hasFinding(report, kind) {
			t.Fatalf("expected %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["max_cyclomatic_complexity"] == 0 || report.Metrics["high_complexity_function_count"] == 0 || report.Scores.Maintainability >= 100 {
		t.Fatalf("expected quality metrics to affect maintainability, got metrics=%#v scores=%#v", report.Metrics, report.Scores)
	}
}

func TestEngineAssessReportsGoSafetyAndPerformanceSmells(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "smells.go", `package demo

import "os"

func Smells(v interface{}) {
	_ = os.Chdir(".")
	_ = v.(string)
	for i := 0; i < 2; i++ {
		defer os.Chdir(".")
	}
	panic("stop")
	var s string
	for i := 0; i < 2; i++ {
		s += "x"
	}
	_ = s
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{
		"safety_ignored_error",
		"safety_unchecked_type_assertion",
		"safety_defer_in_loop",
		"safety_process_exit",
		"performance_string_concat_in_loop",
	} {
		if !hasFinding(report, kind) {
			t.Fatalf("expected %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["ignored_error_count"] != 1 || report.Metrics["unchecked_type_assertion_count"] != 1 || report.Metrics["defer_in_loop_count"] != 1 || report.Metrics["process_exit_count"] != 1 || report.Metrics["string_concat_in_loop_count"] != 1 {
		t.Fatalf("unexpected safety/performance metrics: %#v", report.Metrics)
	}
	if report.Scores.SideEffect >= 100 {
		t.Fatalf("expected safety findings to affect side-effect score, got %#v", report.Scores)
	}
}

func TestEngineAssessReportsGoSecurityMetrics(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "security.go", `package demo

import (
	"crypto/md5"
	"database/sql"
	"os"
	"os/exec"
	"unsafe"
)

func Risk(db *sql.DB, cmd, name string) {
	_ = md5.Sum(nil)
	_ = exec.Command(cmd, "status")
	_, _ = db.Query("select * from " + name)
	_, _ = os.Open(name)
	var p unsafe.Pointer
	_ = p
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{
		"security_unsafe_usage",
		"security_weak_crypto",
		"security_dynamic_exec",
		"security_sql_concat",
		"security_dynamic_file_path",
	} {
		if !hasFinding(report, kind) {
			t.Fatalf("expected %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["unsafe_usage_count"] != 1 || report.Metrics["weak_crypto_count"] != 1 || report.Metrics["dynamic_exec_count"] != 1 || report.Metrics["sql_concat_count"] != 1 || report.Metrics["path_risk_count"] != 1 {
		t.Fatalf("unexpected security metrics: %#v", report.Metrics)
	}
	if report.Scores.SideEffect >= 100 {
		t.Fatalf("expected security findings to affect side-effect score, got %#v", report.Scores)
	}
}

func TestEngineAssessIgnoresCommonSafeSecurityPatterns(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "security.go", `package demo

import (
	"database/sql"
	"os"
	"os/exec"
)

func Safe(db *sql.DB, id int) {
	_ = exec.Command("git", "status")
	_, _ = db.Query("select * from users where id = ?", id)
	_, _ = os.Open("safe.txt")
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"security_dynamic_exec", "security_sql_concat", "security_dynamic_file_path"} {
		if hasFinding(report, kind) {
			t.Fatalf("did not expect %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["dynamic_exec_count"] != 0 || report.Metrics["sql_concat_count"] != 0 || report.Metrics["path_risk_count"] != 0 {
		t.Fatalf("unexpected safe security metrics: %#v", report.Metrics)
	}
}

func TestEngineAssessReportsGoPerformanceMetrics(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "performance.go", `package demo

import "reflect"

type Big struct {
	F01 int
	F02 int
	F03 int
	F04 int
	F05 int
	F06 int
	F07 int
	F08 int
	F09 int
	F10 int
	F11 int
	F12 int
	F13 int
	F14 int
	F15 int
	F16 int
	F17 int
	F18 int
	F19 int
	F20 int
	F21 int
}

func Perf(xs []int) []int {
	_ = reflect.TypeOf(xs)
	var out []int
	for _, x := range xs {
		out = append(out, x)
	}
	for _, item := range []Big{{}} {
		_ = item
	}
	return out
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"performance_reflect_usage", "performance_missing_capacity", "performance_large_range_copy"} {
		if !hasFinding(report, kind) {
			t.Fatalf("expected %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["reflect_usage_count"] != 1 || report.Metrics["missing_capacity_count"] != 1 || report.Metrics["large_range_copy_count"] != 1 {
		t.Fatalf("unexpected performance metrics: %#v", report.Metrics)
	}
}

func TestEngineAssessReportsMissingCapacityForLenBoundLoop(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "performance.go", `package demo

func Perf(xs []int) []int {
	var out []int
	for i := 0; i < len(xs); i++ {
		out = append(out, xs[i])
	}
	return out
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["missing_capacity_count"] != 1 {
		t.Fatalf("expected len-bound loop append to be flagged, got %#v", report.Metrics)
	}
	if !hasFinding(report, "performance_missing_capacity") {
		t.Fatalf("expected missing-capacity finding, got %#v", report.Findings)
	}
}

func TestEngineAssessIgnoresPreallocatedAppend(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "performance.go", `package demo

func Perf(xs []int) []int {
	out := make([]int, 0, len(xs))
	for _, x := range xs {
		out = append(out, x)
	}
	return out
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["missing_capacity_count"] != 0 {
		t.Fatalf("expected preallocated append to be ignored, got %#v", report.Metrics)
	}
	if hasFinding(report, "performance_missing_capacity") {
		t.Fatalf("did not expect missing-capacity finding, got %#v", report.Findings)
	}
}

func TestEngineAssessIgnoresAppendWithoutObviousCapacitySource(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "performance.go", `package demo

func Perf(next func() (int, bool)) []int {
	var out []int
	for {
		x, ok := next()
		if !ok {
			break
		}
		out = append(out, x)
	}
	return out
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["missing_capacity_count"] != 0 {
		t.Fatalf("expected append without obvious capacity source to be ignored, got %#v", report.Metrics)
	}
	if hasFinding(report, "performance_missing_capacity") {
		t.Fatalf("did not expect missing-capacity finding, got %#v", report.Findings)
	}
}

func TestEngineAssessSkipsGeneratedAndVendorQualityFindings(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	noisy := `package demo

func Complex(a, b, c, d, e, f int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				if d > 0 {
					if e > 0 {
						return 1
					}
				}
			}
		}
	}
	switch a {
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 4
	case 5:
		return 5
	}
	return 0
}
`
	writeEngineFile(t, root, "generated.go", "// Code generated by test. DO NOT EDIT.\n"+noisy)
	writeEngineFile(t, root, "vendor/example.com/lib/noisy.go", noisy)
	writeEngineFile(t, root, "demo.go", "package demo\n\nfunc Fine() {}\n")

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range report.Findings {
		if finding.Location.URI == "generated.go" || strings.HasPrefix(finding.Location.URI, "vendor/") {
			t.Fatalf("expected generated/vendor files to be skipped for file-specific quality findings, got %#v", report.Findings)
		}
	}
	if report.Metrics["max_cyclomatic_complexity"] != 1 {
		t.Fatalf("expected only normal file metrics, got %#v", report.Metrics)
	}
	if report.Metrics["generated_loc_percent"] == 0 || !hasFinding(report, "quality_high_generated_ratio") {
		t.Fatalf("expected generated LOC to be counted as an aggregate signal, got %#v %#v", report.Metrics, report.Findings)
	}
}

func TestEngineAssessReportsGoDocsAndNamingMetrics(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "util/util.go", `package util

// Good documents an exported function.
func Good() {}

func Helper() {}

type Manager struct {
	Field string
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"quality_undocumented_export", "quality_low_doc_coverage", "quality_weak_package_name", "quality_weak_identifier_name"} {
		if !hasFinding(report, kind) {
			t.Fatalf("expected %s finding, got %#v", kind, report.Findings)
		}
	}
	if report.Metrics["doc_coverage_percent"] == 100 || report.Metrics["undocumented_export_count"] == 0 || report.Metrics["weak_package_name_count"] == 0 || report.Metrics["weak_identifier_count"] == 0 {
		t.Fatalf("unexpected docs/naming metrics: %#v", report.Metrics)
	}
}

func TestEngineAssessCountsWeakPackageNameOncePerPackage(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "util/a.go", "package util\n\n// A documents A.\nfunc A() {}\n")
	writeEngineFile(t, root, "util/b.go", "package util\n\n// B documents B.\nfunc B() {}\n")

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["weak_package_name_count"] != 1 {
		t.Fatalf("expected one weak package metric for multi-file package, got %#v", report.Metrics)
	}
	if n := countFindings(report, "quality_weak_package_name"); n != 1 {
		t.Fatalf("expected one weak package finding, got %d in %#v", n, report.Findings)
	}
}

func TestEngineAssessTreatsTrailingCommentsAsDocs(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

const Foo = 1 // Foo is documented.

var Bar = 2 // Bar is documented.

type Thing struct {
	Field string // Field is documented.
} // Thing is documented.

type Service interface {
	Run() // Run is documented.
} // Service is documented.
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(report, "quality_undocumented_export") {
		t.Fatalf("expected trailing comments to count as docs, got %#v", report.Findings)
	}
	if report.Metrics["doc_coverage_percent"] != 100 || report.Metrics["undocumented_export_count"] != 0 {
		t.Fatalf("unexpected doc metrics for trailing comments: %#v", report.Metrics)
	}
}

func TestEngineAssessReportsGoTestabilityMetrics(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

// Add adds two numbers.
func Add(a, b int) int {
	if a > 0 {
		return a + b
	}
	return b
}
`)
	writeEngineFile(t, root, "demo_test.go", `package demo

import (
	"testing"
	"time"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		a int
		b int
		want int
	}{
		{"basic", 1, 2, 3},
	}
	for _, tt := range tests {
		time.Sleep(1)
		if got := Add(tt.a, tt.b); got != tt.want {
			t.Fatal(got)
		}
	}
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go, IncludeTests: true},
		Gates: []AssessmentGate{AssessmentGateAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["test_file_count"] != 1 || report.Metrics["test_function_count"] != 1 || report.Metrics["table_test_count"] != 1 || report.Metrics["flaky_test_smell_count"] != 1 {
		t.Fatalf("unexpected testability metrics: %#v", report.Metrics)
	}
	if !hasFinding(report, "coverage_flaky_test_smell") {
		t.Fatalf("expected flaky test smell finding, got %#v", report.Findings)
	}
}

func TestEngineAssessCountsOnlyRunnableGoTests(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", "package demo\n\n// Add adds numbers.\nfunc Add(a, b int) int { return a + b }\n")
	writeEngineFile(t, root, "demo_test.go", `package demo

func TestData() {}

func BenchmarkConfig() {}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go, IncludeTests: true},
		Gates: []AssessmentGate{AssessmentGateCoverage},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["test_function_count"] != 0 {
		t.Fatalf("expected helper-like prefixed functions not to count as runnable tests, got %#v", report.Metrics)
	}
	if !hasFinding(report, "coverage_no_go_tests") {
		t.Fatalf("expected no runnable tests finding, got %#v", report.Findings)
	}
}

func TestEngineAssessCountsExactRunnableGoTestNames(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", "package demo\n\n// Add adds numbers.\nfunc Add(a, b int) int { return a + b }\n")
	writeEngineFile(t, root, "demo_test.go", `package demo

import "testing"

func Test(t *testing.T) {}

func Benchmark(b *testing.B) {}

func Fuzz(f *testing.F) {}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go, IncludeTests: true},
		Gates: []AssessmentGate{AssessmentGateCoverage},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["test_function_count"] != 3 {
		t.Fatalf("expected exact Test/Benchmark/Fuzz names to count as runnable tests, got %#v", report.Metrics)
	}
	if hasFinding(report, "coverage_no_go_tests") {
		t.Fatalf("did not expect no-tests finding, got %#v", report.Findings)
	}
}

func TestEngineAssessIgnoresWeakExportedNamesInTests(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", "package demo\n\n// Add adds numbers.\nfunc Add(a, b int) int { return a + b }\n")
	writeEngineFile(t, root, "demo_test.go", `package demo

import "testing"

func TestAdd(t *testing.T) {}

func Helper() {}

type Manager struct{}

var Data = "fixture"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go, IncludeTests: true},
		Gates: []AssessmentGate{AssessmentGateMaintainability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Metrics["weak_identifier_count"] != 0 {
		t.Fatalf("expected weak exported names in test files to be ignored, got %#v", report.Metrics)
	}
	if hasFinding(report, "quality_weak_identifier_name") {
		t.Fatalf("did not expect weak identifier finding from test helper names, got %#v", report.Findings)
	}
}

func TestEngineAssessReportsValidationViolations(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "broken.go", "package demo\n\nfunc Broken( {\n")

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{Gates: []AssessmentGate{AssessmentGateSafety}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Validation.Passed || !hasViolation(report, "safety_validation_diagnostic") {
		t.Fatalf("expected validation violation, got %#v", report)
	}
}

func TestEngineAssessArchitectureGateUsesParseValidationOnly(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "demo.go", `package demo

func WrongType() int {
	return "not an int"
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	architectureReport, err := eng.Assess(context.Background(), AssessmentOptions{Gates: []AssessmentGate{AssessmentGateArchitecture}})
	if err != nil {
		t.Fatal(err)
	}
	if !architectureReport.Validation.Passed || architectureReport.Validation.ResolutionMode != "ast" || hasViolation(architectureReport, "safety_validation_diagnostic") {
		t.Fatalf("expected architecture gate to use parse validation only, got %#v", architectureReport)
	}

	safetyReport, err := eng.Assess(context.Background(), AssessmentOptions{Gates: []AssessmentGate{AssessmentGateSafety}})
	if err != nil {
		t.Fatal(err)
	}
	if safetyReport.Validation.Passed || safetyReport.Validation.ResolutionMode != "typecheck" || !hasViolation(safetyReport, "safety_validation_diagnostic") {
		t.Fatalf("expected safety gate to include typecheck diagnostics, got %#v", safetyReport)
	}
}

func TestEngineGoBuildConstraintsSelectActiveFiles(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "active.go", `package demo

func Active() string {
	return activePlatform()
}
`)
	writeEngineFile(t, root, "active_"+runtime.GOOS+".go", `package demo

func activePlatform() string {
	return "active"
}
`)
	writeEngineFile(t, root, "inactive_"+inactiveGOOS()+".go", `package demo

func brokenPlatform( {
`)
	writeEngineFile(t, root, "tagged_out.go", `//go:build codegate_missing_tag

package demo

func brokenTagged( {
`)
	writeEngineFile(t, root, "legacy_tagged_out.go", `// +build codegate_missing_tag

package demo

func brokenLegacyTagged( {
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := eng.Validate(context.Background(), ValidationOptions{Kinds: []ValidationKind{ValidationParse, ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Passed {
		t.Fatalf("expected inactive build files to be skipped, got %#v", validation)
	}

	lookup, err := eng.Lookup(context.Background(), LookupQuery{Name: "Active", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Symbols) != 1 {
		t.Fatalf("expected active symbol lookup, got %#v", lookup)
	}
	inactiveLookup, err := eng.Lookup(context.Background(), LookupQuery{Name: "brokenTagged", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(inactiveLookup.Symbols) != 0 {
		t.Fatalf("expected build-tagged file to be absent from index, got %#v", inactiveLookup.Symbols)
	}
}

func TestEngineAssessAppliesArchitectureImportRules(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "domain/domain.go", `package domain

import "example.com/demo/infra"

func UseInfra() string {
	return infra.Name
}
`)
	writeEngineFile(t, root, "infra/infra.go", `package infra

const Name = "infra"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			Imports: []ArchitectureImportRule{{
				From:   "domain",
				To:     "example.com/demo/infra",
				Action: ArchitectureRuleDeny,
				Reason: "domain must not depend on infra",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasViolation(report, "architecture_denied_import") || report.Scores.Boundary >= 100 {
		t.Fatalf("expected denied import violation and boundary score impact, got %#v", report)
	}
}

func TestEngineAssessArchitectureAllowOverridesBroaderDeny(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "domain/domain.go", `package domain

import "example.com/demo/infra/safe"

func UseSafeInfra() string {
	return safe.Name
}
`)
	writeEngineFile(t, root, "infra/safe/safe.go", `package safe

const Name = "safe"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			Imports: []ArchitectureImportRule{
				{From: "domain", To: "example.com/demo/infra", Action: ArchitectureRuleDeny},
				{From: "domain", To: "example.com/demo/infra/safe", Action: ArchitectureRuleAllow},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasViolation(report, "architecture_denied_import") {
		t.Fatalf("expected allow override to suppress broader deny, got %#v", report.Violations)
	}
}

func TestEngineAssessAppliesArchitectureTestImportRules(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "app/app.go", `package app

import "example.com/demo/testutil"

func Fixture() string {
	return testutil.Name
}
`)
	writeEngineFile(t, root, "testutil/testutil.go", `package testutil

const Name = "fixture"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			TestImports: []ArchitectureImportRule{{
				From:   "app",
				To:     "example.com/demo/testutil",
				Action: ArchitectureRuleDeny,
				Reason: "production code should not import test helpers",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasViolation(report, "architecture_test_boundary_import") || report.Scores.TestBoundary >= 100 {
		t.Fatalf("expected test-boundary violation and score impact, got %#v", report)
	}
}

func TestEngineAssessAppliesGenericArchitectureLayerRules(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "core/core.go", `package core

import "example.com/demo/runtime"

func Bad() string {
	return runtime.Name
}
`)
	writeEngineFile(t, root, "runtime/runtime.go", `package runtime

const Name = "runtime"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			Layers: []ArchitectureLayer{
				{Name: "core", Prefixes: []string{"core"}},
				{Name: "runtime", Prefixes: []string{"runtime"}},
			},
			Dependencies: []ArchitectureDependencyRule{
				{FromLayer: "runtime", ToLayer: "core"},
				{FromLayer: "runtime", ToLayer: "runtime"},
				{FromLayer: "core", ToLayer: "core"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasViolation(report, "architecture_boundary_violation") || report.Scores.Boundary != 75 {
		t.Fatalf("expected generic layer boundary violation, got %#v", report)
	}
}

func TestEngineAssessAppliesGenericArchitectureEffectRules(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "core/core.go", `package core

import "os"

func Token() string {
	return os.Getenv("TOKEN")
}
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			Layers: []ArchitectureLayer{{Name: "domain", Prefixes: []string{"core"}}},
			Effects: []ArchitectureEffectRule{{
				Name:    "host_io",
				Scope:   ArchitectureScope{Layers: []string{"domain"}},
				Imports: []string{"os"},
				Calls:   []ArchitectureCallRule{{Import: "os", Symbol: "Getenv"}},
				Reason:  "domain layer must not access host IO directly",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasViolation(report, "architecture_host_io") || report.Scores.SideEffect >= 100 {
		t.Fatalf("expected generic effect violation and side-effect score impact, got %#v", report)
	}
}

func TestEngineAssessAllowsReviewedFanOut(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "core/hub/hub.go", `package hub

import (
	"example.com/demo/core/a"
	"example.com/demo/core/b"
	"example.com/demo/core/c"
)

func Names() []string {
	return []string{a.Name, b.Name, c.Name}
}
`)
	for _, name := range []string{"a", "b", "c"} {
		writeEngineFile(t, root, "core/"+name+"/"+name+".go", "package "+name+"\n\nconst Name = \""+name+"\"\n")
	}

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope: Scope{Language: Go},
		Gates: []AssessmentGate{AssessmentGateArchitecture},
		Architecture: &ArchitectureRules{
			Layers: []ArchitectureLayer{{Name: "core", Prefixes: []string{"core"}}},
			Dependencies: []ArchitectureDependencyRule{
				{FromLayer: "core", ToLayer: "core"},
			},
			Coupling: ArchitectureCouplingRules{
				FanOutThreshold: 2,
				Layers:          []string{"core"},
				ReviewedFanOut:  []ArchitecturePackageNote{{Package: "core/hub", Reason: "hub intentionally aggregates core packages"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasAllowedFinding(report, "architecture_fan_out") || report.Scores.Coupling != 100 {
		t.Fatalf("expected reviewed fan-out note without coupling penalty, got %#v", report)
	}
}

func TestEngineAssessAgentruntimeStyleArchitecturePolicyExample(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module github.com/fluxplane/agentruntime\n\ngo 1.24\n")
	writeEngineFile(t, root, "core/operation/operation.go", `package operation

const Name = "operation"
`)
	writeEngineFile(t, root, "core/resource/resource.go", `package resource

import (
	"github.com/fluxplane/agentruntime/core/operation"
	"github.com/fluxplane/agentruntime/core/policy"
	"github.com/fluxplane/agentruntime/core/session"
)

func Names() []string {
	return []string{operation.Name, policy.Name, session.Name}
}
`)
	writeEngineFile(t, root, "core/policy/policy.go", `package policy

const Name = "policy"
`)
	writeEngineFile(t, root, "core/session/session.go", `package session

const Name = "session"
`)
	writeEngineFile(t, root, "core/bad/bad.go", `package bad

import "github.com/fluxplane/agentruntime/runtime/system"

func Bad() string {
	return system.Name
}
`)
	writeEngineFile(t, root, "runtime/system/system.go", `package system

import "net/http"

const Name = "system"

var Client = http.DefaultClient
`)
	writeEngineFile(t, root, "orchestration/session/session.go", `package session

import (
	"github.com/fluxplane/agentruntime/core/operation"
	"github.com/fluxplane/agentruntime/runtime/system"
)

func Run() string {
	return operation.Name + system.Name
}
`)
	writeEngineFile(t, root, "plugins/integrations/slack/plugin.go", `package slack

import "os"

func Token() string {
	return os.Getenv("SLACK_TOKEN")
}
`)
	writeEngineFile(t, root, "experimental/foo/foo.go", `package foo

const Name = "foo"
`)

	eng, err := New().
		Roots(".").
		WithFS(os.DirFS(root)).
		WithLanguage(goast.New()).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := eng.Assess(context.Background(), AssessmentOptions{
		Scope:        Scope{Language: Go},
		Gates:        []AssessmentGate{AssessmentGateArchitecture},
		Architecture: agentruntimeStyleArchitectureRules(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"architecture_boundary_violation", "architecture_plugin_host_effect", "architecture_unknown_package"} {
		if !hasViolation(report, kind) {
			t.Fatalf("expected %s in agentruntime-style policy report, got %#v", kind, report)
		}
	}
	if !hasAllowedFinding(report, "architecture_fan_out") {
		t.Fatalf("expected reviewed fan-out finding, got %#v", report.Findings)
	}
	if report.Scores.Boundary != 75 || report.Scores.SideEffect != 90 || report.Scores.Coverage != 80 || report.Scores.Coupling != 100 {
		t.Fatalf("unexpected agentruntime-style component scores: %#v", report.Scores)
	}
}

func TestEngineRejectsMultipleRootsForNow(t *testing.T) {
	_, err := New().Roots("one", "two").WithLanguage(goast.New()).Build(context.Background())
	if err == nil {
		t.Fatal("expected multiple roots to fail until multi-root source support lands")
	}
}

func hasFinding(report AssessmentReport, kind string) bool {
	for _, finding := range report.Findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func countFindings(report AssessmentReport, kind string) int {
	n := 0
	for _, finding := range report.Findings {
		if finding.Kind == kind {
			n++
		}
	}
	return n
}

func hasAllowedFinding(report AssessmentReport, kind string) bool {
	for _, finding := range report.Findings {
		if finding.Kind == kind && finding.Allowed {
			return true
		}
	}
	return false
}

func hasViolation(report AssessmentReport, kind string) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind {
			return true
		}
	}
	return false
}

func hasSuggestion(report AssessmentReport, kind RefactorKind) bool {
	for _, suggestion := range report.Suggestions {
		if suggestion.Kind == kind {
			return true
		}
	}
	return false
}

func hasCapability(spec BackendSpec, capability Capability, level CapabilityLevel) bool {
	for _, support := range spec.Capabilities {
		if support.Capability == capability && support.Level == level {
			return true
		}
	}
	return false
}

func hasOperation[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasMetricSupport(spec BackendSpec, id string) bool {
	for _, metric := range spec.Operations.Assessment.Metrics {
		if metric.ID == id {
			return true
		}
	}
	return false
}

func hasFindingSupport(spec BackendSpec, id string) bool {
	for _, finding := range spec.Operations.Assessment.Findings {
		if finding.ID == id {
			return true
		}
	}
	return false
}

func proposalWithEvidence(proposals []Proposal, kind string) *Proposal {
	for i := range proposals {
		for _, evidence := range proposals[i].Evidence {
			if evidence.Kind == kind {
				return &proposals[i]
			}
		}
	}
	return nil
}

func inactiveGOOS() string {
	if runtime.GOOS == "windows" {
		return "linux"
	}
	return "windows"
}

func agentruntimeStyleArchitectureRules() *ArchitectureRules {
	return &ArchitectureRules{
		ModulePath: "github.com/fluxplane/agentruntime",
		Layers: []ArchitectureLayer{
			{Name: "facade", Prefixes: []string{"."}},
			{Name: "core", Prefixes: []string{"core"}},
			{Name: "sdk", Prefixes: []string{"sdk"}},
			{Name: "runtime", Prefixes: []string{"runtime"}},
			{Name: "orchestration", Prefixes: []string{"orchestration"}},
			{Name: "adapters", Prefixes: []string{"adapters"}},
			{Name: "plugins", Prefixes: []string{"plugins"}},
			{Name: "apps", Prefixes: []string{"apps"}},
			{Name: "cmd", Prefixes: []string{"cmd"}},
		},
		Dependencies: []ArchitectureDependencyRule{
			{FromLayer: "core", ToLayer: "core"},
			{FromLayer: "sdk", ToLayer: "core"},
			{FromLayer: "sdk", ToLayer: "sdk"},
			{FromLayer: "runtime", ToLayer: "core"},
			{FromLayer: "runtime", ToLayer: "runtime"},
			{FromLayer: "orchestration", ToLayer: "core"},
			{FromLayer: "orchestration", ToLayer: "runtime"},
			{FromLayer: "orchestration", ToLayer: "orchestration"},
			{FromLayer: "adapters", ToLayer: "core"},
			{FromLayer: "adapters", ToLayer: "runtime"},
			{FromLayer: "adapters", ToLayer: "orchestration"},
			{FromLayer: "adapters", ToLayer: "adapters"},
			{FromLayer: "plugins", ToLayer: "core"},
			{FromLayer: "plugins", ToLayer: "sdk"},
			{FromLayer: "plugins", ToLayer: "runtime"},
			{FromLayer: "plugins", ToLayer: "orchestration"},
			{FromLayer: "plugins", ToLayer: "adapters"},
			{FromLayer: "plugins", ToLayer: "plugins"},
			{FromLayer: "apps", ToLayer: "core"},
			{FromLayer: "apps", ToLayer: "sdk"},
			{FromLayer: "apps", ToLayer: "runtime"},
			{FromLayer: "apps", ToLayer: "orchestration"},
			{FromLayer: "apps", ToLayer: "adapters"},
			{FromLayer: "apps", ToLayer: "plugins"},
			{FromLayer: "apps", ToLayer: "apps"},
			{FromLayer: "apps", ToLayer: "facade"},
			{FromLayer: "cmd", ToLayer: "apps"},
			{FromLayer: "cmd", ToLayer: "adapters"},
			{FromLayer: "cmd", ToLayer: "cmd"},
			{FromLayer: "facade", ToLayer: "core"},
			{FromLayer: "facade", ToLayer: "sdk"},
			{FromLayer: "facade", ToLayer: "runtime"},
			{FromLayer: "facade", ToLayer: "orchestration"},
			{FromLayer: "facade", ToLayer: "adapters"},
		},
		Effects: []ArchitectureEffectRule{
			{
				Name:    "inner_host_io",
				Scope:   ArchitectureScope{Layers: []string{"core", "sdk", "orchestration"}},
				Imports: []string{"os", "os/exec", "syscall", "net", "net/http", "net/url", "database/sql"},
				Reason:  "inner production layers must not import host IO directly",
			},
			{
				Name:    "runtime_host_io",
				Scope:   ArchitectureScope{Layers: []string{"runtime"}},
				Imports: []string{"os", "os/exec", "os/user", "syscall", "net", "net/http", "net/url", "database/sql", "path/filepath"},
				Reason:  "runtime host IO imports require an explicit package allowlist reason",
			},
			{
				Name:   "plugin_host_effect",
				Scope:  ArchitectureScope{Layers: []string{"plugins"}},
				Calls:  []ArchitectureCallRule{{Import: "os", Symbol: "Getenv"}, {Import: "os/exec", Symbol: "Command"}, {Import: "net/http", Symbol: "Get"}},
				Reason: "plugin host side effects must go through the project-defined system boundary",
			},
		},
		Coupling: ArchitectureCouplingRules{
			FanOutThreshold: 2,
			Layers:          []string{"core", "runtime", "orchestration"},
			ReviewedFanOut: []ArchitecturePackageNote{
				{Package: "core/resource", Reason: "resource owns the inert contribution bundle, index, and resolver hub"},
			},
		},
		Exceptions: []ArchitectureException{
			{Kind: "architecture_runtime_host_io", Package: "runtime/system", Reason: "system runtime is the central host side-effect boundary"},
		},
	}
}

func writeEngineFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
