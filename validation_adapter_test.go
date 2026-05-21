package codegate_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
)

func TestEngineValidationAdapterRunsOnlyWhenNamed(t *testing.T) {
	ctx := context.Background()
	called := false
	adapter := codegate.NewValidationAdapter("unit", func(context.Context, codegate.Source, codegate.ValidationOptions) (codegate.ValidationResult, error) {
		called = true
		return codegate.ValidationResult{Passed: true, Complete: true, Kinds: []codegate.ValidationKind{codegate.ValidationExternal}, ResolutionMode: "external"}, nil
	})
	eng, err := codegate.New().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		WithValidationAdapter(adapter).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Validate(ctx, codegate.ValidationOptions{Scope: codegate.Scope{Language: codegate.Go}, Kinds: []codegate.ValidationKind{codegate.ValidationParse}})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("validation adapter should not run unless named")
	}
	if !result.Passed || hasValidationKind(result.Kinds, codegate.ValidationExternal) {
		t.Fatalf("unexpected default validation result: %#v", result)
	}
}

func TestEngineValidationAdapterMergesResults(t *testing.T) {
	ctx := context.Background()
	adapter := codegate.NewValidationAdapter("unit", func(context.Context, codegate.Source, codegate.ValidationOptions) (codegate.ValidationResult, error) {
		return codegate.ValidationResult{
			Passed:         false,
			Complete:       true,
			Kinds:          []codegate.ValidationKind{codegate.ValidationExternal},
			ResolutionMode: "external",
			AffectedPaths:  []string{"demo.go"},
			Diagnostics:    []codegate.Diagnostic{{Severity: "error", Message: "external check failed"}},
		}, nil
	})
	eng, err := codegate.New().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		WithValidationAdapter(adapter).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Validate(ctx, codegate.ValidationOptions{
		Scope:    codegate.Scope{Language: codegate.Go},
		Kinds:    []codegate.ValidationKind{codegate.ValidationParse},
		External: []string{"unit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || !hasValidationKind(result.Kinds, codegate.ValidationExternal) || len(result.Diagnostics) != 1 || len(result.AffectedPaths) != 1 {
		t.Fatalf("unexpected adapter validation result: %#v", result)
	}
}

func TestChangeSetValidationAdapterSeesPendingOverlay(t *testing.T) {
	ctx := context.Background()
	adapter := codegate.NewValidationAdapter("overlay", func(ctx context.Context, snapshot codegate.Source, _ codegate.ValidationOptions) (codegate.ValidationResult, error) {
		src, err := snapshot.ReadFile(ctx, "demo.go")
		if err != nil {
			return codegate.ValidationResult{}, err
		}
		if string(src) != "package demo\n\nfunc Target() string { return \"after\" }\n" {
			return codegate.ValidationResult{Passed: false, Complete: true, Kinds: []codegate.ValidationKind{codegate.ValidationExternal}, Diagnostics: []codegate.Diagnostic{{Severity: "error", Message: "overlay not visible"}}}, nil
		}
		return codegate.ValidationResult{Passed: true, Complete: true, Kinds: []codegate.ValidationKind{codegate.ValidationExternal}}, nil
	})
	eng, err := codegate.New().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc Target() string { return \"before\" }\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		WithValidationAdapter(adapter).
		Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changes := eng.NewChangeSet()
	err = changes.Apply(ctx, codegate.ReplaceFunction{
		Target: codegate.SymbolSelector{Name: "Target", Kind: codegate.SymbolFunction},
		Source: "func Target() string { return \"after\" }",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := changes.Validate(ctx, codegate.ValidationOptions{
		Scope:    codegate.Scope{Language: codegate.Go},
		Kinds:    []codegate.ValidationKind{codegate.ValidationParse},
		External: []string{"overlay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("expected adapter to validate pending overlay, got %#v", result)
	}
}

func TestEngineValidationAdapterUnknownNameFails(t *testing.T) {
	eng, err := codegate.New().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Validate(context.Background(), codegate.ValidationOptions{
		Scope:    codegate.Scope{Language: codegate.Go},
		External: []string{"missing"},
	})
	if err == nil {
		t.Fatal("expected unknown validation adapter to fail")
	}
}

func TestEngineValidationAdapterPropagatesErrors(t *testing.T) {
	want := errors.New("adapter failed")
	adapter := codegate.NewValidationAdapter("failing", func(context.Context, codegate.Source, codegate.ValidationOptions) (codegate.ValidationResult, error) {
		return codegate.ValidationResult{}, want
	})
	eng, err := codegate.New().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		WithValidationAdapter(adapter).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Validate(context.Background(), codegate.ValidationOptions{
		Scope:    codegate.Scope{Language: codegate.Go},
		External: []string{"failing"},
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected adapter error, got %v", err)
	}
}

func hasValidationKind(kinds []codegate.ValidationKind, target codegate.ValidationKind) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}
