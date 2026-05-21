package markdown

import (
	"context"
	"testing"
)

type testSnapshot map[string]string

func (s testSnapshot) ListFiles(ctx context.Context, scope Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	files := make([]string, 0, len(s))
	for p := range s {
		files = append(files, p)
	}
	return files, nil
}

func (s testSnapshot) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []byte(s[path]), nil
}

func TestMarkdownBackendSpecAndIndex(t *testing.T) {
	ctx := context.Background()
	backend := New()
	spec := backend.Spec()
	if spec.Language != Markdown || len(spec.Operations.EditOperations) == 0 || len(spec.Operations.Assessment.Metrics) == 0 {
		t.Fatalf("unexpected spec %#v", spec)
	}
	idx, err := backend.Index(ctx, testSnapshot{
		"README.md": "# Title\n\n## Setup\n\nSee [Setup](#setup).\n",
	}, Scope{Language: Markdown})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Documents) != 1 || len(idx.Symbols) < 2 {
		t.Fatalf("unexpected markdown index: documents=%d symbols=%d", len(idx.Documents), len(idx.Symbols))
	}
}

func TestMarkdownBackendSuggestsStructuralFix(t *testing.T) {
	ctx := context.Background()
	proposals, err := New().Suggest(ctx, testSnapshot{
		"README.md": "Intro\n\n### Jumped\n",
	}, Scope{Language: Markdown})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) == 0 {
		t.Fatal("expected markdown structural proposal")
	}
}
