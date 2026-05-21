package codegate_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/language/golang"
	"github.com/codewandler/codegate/language/markdown"
)

func TestPublicEngineBuilderRegistersLanguagePackages(t *testing.T) {
	eng, err := codegate.New().
		WithFS(fstest.MapFS{
			"demo.go":   &fstest.MapFile{Data: []byte("package demo\n\nfunc Target() {}\n")},
			"README.md": &fstest.MapFile{Data: []byte("# Demo\n\n## Notes\n")},
		}).
		WithLanguage(golang.New(golang.Config{})).
		WithLanguage(markdown.New(markdown.Config{})).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	specs := eng.Capabilities()
	if len(specs) != 2 || specs[0].Language != codegate.Go || specs[1].Language != codegate.Markdown {
		t.Fatalf("unexpected capabilities: %#v", specs)
	}
}

func TestNewEngineCompatibilityAlias(t *testing.T) {
	eng, err := codegate.NewEngine().
		WithFS(fstest.MapFS{"demo.go": &fstest.MapFile{Data: []byte("package demo\n")}}).
		WithLanguage(golang.New(golang.Config{})).
		Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(eng.Capabilities()) != 1 {
		t.Fatalf("unexpected capabilities: %#v", eng.Capabilities())
	}
}

func TestPublicEngineRequiresLanguageBackend(t *testing.T) {
	_, err := codegate.New().WithFS(fstest.MapFS{}).Build(context.Background())
	if err == nil {
		t.Fatal("expected missing language backend to fail")
	}
}
