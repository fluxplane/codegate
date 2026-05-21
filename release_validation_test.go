package codegate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalConsumerModuleUsesOnlyPublicPackages(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeReleaseTestFile(t, dir, "go.mod", `module example.com/codegate-consumer

go 1.24

require github.com/fluxplane/codegate v0.0.0

replace github.com/fluxplane/codegate => `+filepath.ToSlash(repoRoot)+`
`)
	writeReleaseTestFile(t, dir, "main.go", `package main

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/fluxplane/codegate"
	"github.com/fluxplane/codegate/adapter/agentruntime"
	"github.com/fluxplane/codegate/language/golang"
	"github.com/fluxplane/codegate/language/markdown"
)

func main() {
	ctx := context.Background()
	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc Target() {}\n")},
			"README.md": &fstest.MapFile{Data: []byte("# Demo\n\n## Notes\n")},
		}).
		WithLanguage(golang.New(golang.Config{})).
		WithLanguage(markdown.New(markdown.Config{})).
		WithValidationAdapter(codegate.NewValidationAdapter("unit", func(context.Context, codegate.Source, codegate.ValidationOptions) (codegate.ValidationResult, error) {
			return codegate.ValidationResult{
				Passed:         true,
				Complete:       true,
				Kinds:          []codegate.ValidationKind{codegate.ValidationExternal},
				ResolutionMode: "external",
				AffectedPaths:  []string{"demo.go"},
			}, nil
		})).
		Build(ctx)
	if err != nil {
		panic(err)
	}
	if len(engine.Capabilities()) != 2 {
		panic("expected two capabilities")
	}
	lookup, err := engine.Lookup(ctx, codegate.LookupQuery{Name: "Target", Kind: codegate.SymbolFunction})
	if err != nil {
		panic(err)
	}
	report, err := engine.Assess(ctx, codegate.AssessmentOptions{Scope: codegate.Scope{Language: codegate.Markdown}})
	if err != nil {
		panic(err)
	}
	validation, err := engine.Validate(ctx, codegate.ValidationOptions{
		Scope:    codegate.Scope{Language: codegate.Go},
		Kinds:    []codegate.ValidationKind{codegate.ValidationParse},
		External: []string{"unit"},
	})
	if err != nil {
		panic(err)
	}
	source, err := agentruntime.NewSource(
		func(context.Context, string, int64) ([]byte, bool, error) {
			return []byte("package demo\n"), false, nil
		},
		func(context.Context, codegate.Scope) ([]string, error) {
			return []string{"demo.go"}, nil
		},
	)
	if err != nil {
		panic(err)
	}
	if _, err := source.ReadFile(ctx, "demo.go"); err != nil {
		panic(err)
	}
	fmt.Println("ok", lookup.Symbols[0].Name, report.Language, validation.Passed, len(validation.AffectedPaths))
}
`)
	env := append(os.Environ(), "GOCACHE="+filepath.Join(dir, ".gocache"), "GOWORK=off")
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = env
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("external consumer tidy failed: %v\n%s", err, out)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external consumer failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "ok Target markdown true 1" {
		t.Fatalf("unexpected external consumer output: %q", got)
	}
}

func TestPublicDocsDoNotImportInternalPackages(t *testing.T) {
	for _, path := range []string{"README.md", "doc.go", "example_test.go", "language/golang/doc.go", "language/markdown/doc.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "github.com/fluxplane/codegate/internal") || strings.Contains(string(data), "internal/") {
			t.Fatalf("%s references internal packages in public documentation", path)
		}
	}
}

func TestPublicLanguageWrappersHideInternalBackendTypes(t *testing.T) {
	for _, path := range []string{"language/golang/golang.go", "language/markdown/markdown.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if !strings.Contains(src, "func New(Config) codegate.Backend") {
			t.Fatalf("%s must expose New(Config) codegate.Backend", path)
		}
		if strings.Contains(src, "func New(Config) goast.") || strings.Contains(src, "func New(Config) internalmarkdown.") {
			t.Fatalf("%s exposes an internal concrete backend type", path)
		}
	}
}

func writeReleaseTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
