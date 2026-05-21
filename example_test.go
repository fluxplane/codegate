package codegate_test

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
	"github.com/codewandler/codegate/language/markdown"
)

func ExampleNew() {
	ctx := context.Background()
	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc Target() {}\n")},
		}).
		WithLanguage(golang.New(golang.Config{})).
		Build(ctx)
	if err != nil {
		panic(err)
	}

	result, err := engine.Lookup(ctx, codegate.LookupQuery{Name: "Target", Kind: codegate.SymbolFunction})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Symbols[0].Name)

	// Output:
	// Target
}

func ExampleNew_markdown() {
	ctx := context.Background()
	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"README.md": &fstest.MapFile{Data: []byte("# Demo\n\n## Notes\nbody\n")},
		}).
		WithLanguage(markdown.New(markdown.Config{})).
		Build(ctx)
	if err != nil {
		panic(err)
	}

	report, err := engine.Assess(ctx, codegate.AssessmentOptions{Scope: codegate.Scope{Language: codegate.Markdown}})
	if err != nil {
		panic(err)
	}
	fmt.Println(report.Language, report.Validation.Passed)

	// Output:
	// markdown true
}

func ExampleNewEditor() {
	ctx := context.Background()
	editor, err := codegate.NewEditor(".", codegate.WithFS(fstest.MapFS{
		"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc hello() string { return \"hello\" }\n")},
	}), codegate.WithLanguage(codegate.Go))
	if err != nil {
		panic(err)
	}

	fragment, err := editor.ReadSymbol(ctx, codegate.SymbolSelector{Name: "hello", Kind: codegate.SymbolFunction})
	if err != nil {
		panic(err)
	}
	changes := editor.NewChangeSet()
	err = changes.Apply(ctx, codegate.ReplaceFunction{
		Target: codegate.SymbolSelector{ID: fragment.Symbol.ID},
		Source: "func hello() string { return \"hi\" }",
	})
	if err != nil {
		panic(err)
	}
	_, err = changes.Diff(ctx)
	if err != nil {
		panic(err)
	}
}
