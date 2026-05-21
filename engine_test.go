package codegate

import (
	"context"
	"os"
	"path/filepath"
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
