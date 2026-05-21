package editor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
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
	eng, err := NewEngine().Roots(".").WithFS(os.DirFS(root)).Build(ctx)
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
	eng, err := NewEngine().WithFS(fstest.MapFS{}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	specs := eng.Capabilities()
	if len(specs) != 1 || specs[0].Language != Go {
		t.Fatalf("unexpected capabilities: %#v", specs)
	}
	if !hasCapability(specs[0], CapabilityLookup, CapabilityAdvanced) {
		t.Fatalf("go backend did not declare advanced lookup: %#v", specs[0].Capabilities)
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

	eng, err := NewEngine().Roots(".").WithFS(os.DirFS(root)).Build(context.Background())
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

func TestEngineAssessReportsValidationViolations(t *testing.T) {
	root := t.TempDir()
	writeEngineFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeEngineFile(t, root, "broken.go", "package demo\n\nfunc Broken( {\n")

	eng, err := NewEngine().Roots(".").WithFS(os.DirFS(root)).Build(context.Background())
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

func TestEngineRejectsMultipleRootsForNow(t *testing.T) {
	_, err := NewEngine().Roots("one", "two").Build(context.Background())
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

func hasViolation(report AssessmentReport, kind string) bool {
	for _, violation := range report.Violations {
		if violation.Kind == kind {
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
