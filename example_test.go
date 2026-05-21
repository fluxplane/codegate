package codegate_test

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/codewandler/codegate"
	agentruntimeadapter "github.com/codewandler/codegate/adapter/agentruntime"
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

func ExampleNew_agentLoop() {
	ctx := context.Background()
	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"go.mod":  &fstest.MapFile{Data: []byte("module example.com/demo\n\ngo 1.24\n")},
			"demo.go": &fstest.MapFile{Data: []byte("package demo\n\nfunc used() string { return \"ok\" }\n\nfunc unused() {}\n")},
		}).
		WithLanguage(golang.New(golang.Config{})).
		Build(ctx)
	if err != nil {
		panic(err)
	}

	_, err = engine.Lookup(ctx, codegate.LookupQuery{Name: "used", Kind: codegate.SymbolFunction})
	if err != nil {
		panic(err)
	}
	report, err := engine.Assess(ctx, codegate.AssessmentOptions{
		Scope: codegate.Scope{Language: codegate.Go},
		Gates: []codegate.AssessmentGate{codegate.AssessmentGateAll},
	})
	if err != nil {
		panic(err)
	}
	proposals, err := engine.Suggest(ctx, codegate.SuggestOptions{Scope: codegate.Scope{Language: codegate.Go}})
	if err != nil {
		panic(err)
	}
	executable := codegate.ExecutableProposals(proposals)
	if len(executable) > 0 {
		changes := engine.NewChangeSet()
		if err := changes.Apply(ctx, executable[0].Operations...); err != nil {
			panic(err)
		}
		if _, err := changes.Validate(ctx, codegate.ValidationOptions{Scope: codegate.Scope{Language: codegate.Go}}); err != nil {
			panic(err)
		}
		if _, err := changes.Diff(ctx); err != nil {
			panic(err)
		}
	}

	fmt.Println(report.Language, len(executable) > 0)

	// Output:
	// go true
}

func ExampleNew_markdownCleanupLoop() {
	ctx := context.Background()
	engine, err := codegate.New().
		WithFS(fstest.MapFS{
			"README.md": &fstest.MapFile{Data: []byte("Intro text.\n\n### Setup\n")},
		}).
		WithLanguage(markdown.New(markdown.Config{})).
		Build(ctx)
	if err != nil {
		panic(err)
	}

	report, err := engine.Assess(ctx, codegate.AssessmentOptions{
		Scope: codegate.Scope{Language: codegate.Markdown},
		Gates: []codegate.AssessmentGate{codegate.AssessmentGateMaintainability},
	})
	if err != nil {
		panic(err)
	}
	proposals, err := engine.Suggest(ctx, codegate.SuggestOptions{Scope: codegate.Scope{Language: codegate.Markdown}})
	if err != nil {
		panic(err)
	}
	executable := codegate.ExecutableProposals(proposals)
	if len(executable) > 0 {
		changes := engine.NewChangeSet()
		if err := changes.Apply(ctx, executable[0].Operations...); err != nil {
			panic(err)
		}
		if _, err := changes.Validate(ctx, codegate.ValidationOptions{Scope: codegate.Scope{Language: codegate.Markdown}}); err != nil {
			panic(err)
		}
		if _, err := changes.Diff(ctx); err != nil {
			panic(err)
		}
	}

	fmt.Println(report.Language, len(executable) > 0)

	// Output:
	// markdown true
}

func ExampleNew_agentruntimeSource() {
	ctx := context.Background()
	files := map[string][]byte{
		"demo.go": []byte("package demo\n\nfunc Target() {}\n"),
	}
	readFile := func(_ context.Context, filePath string, maxBytes int64) ([]byte, bool, error) {
		data := files[filePath]
		if maxBytes > 0 && int64(len(data)) > maxBytes {
			return data[:maxBytes], true, nil
		}
		return data, false, nil
	}
	walkFiles := func(context.Context, string, agentruntimeadapter.WalkOptions) ([]agentruntimeadapter.WalkEntry, bool, error) {
		return []agentruntimeadapter.WalkEntry{{Path: "demo.go", Kind: "file"}}, false, nil
	}
	source, err := agentruntimeadapter.NewWalkSource(readFile, walkFiles)
	if err != nil {
		panic(err)
	}
	engine, err := codegate.New().
		WithSource(source).
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
