package agentruntime_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/codewandler/editor"
	"github.com/codewandler/editor/adapter/agentruntime"
)

func TestWalkSourceIntegratesWithEditor(t *testing.T) {
	ctx := context.Background()
	files := map[string][]byte{
		"main.go": []byte(`package main

func run() {}
`),
	}
	source, err := agentruntime.NewWalkSource(
		func(ctx context.Context, filePath string, maxBytes int64) ([]byte, bool, error) {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			data, ok := files[filePath]
			if !ok {
				return nil, false, os.ErrNotExist
			}
			if maxBytes > 0 && int64(len(data)) > maxBytes {
				return data[:maxBytes], true, nil
			}
			return append([]byte(nil), data...), false, nil
		},
		func(ctx context.Context, root string, opts agentruntime.WalkOptions) ([]agentruntime.WalkEntry, bool, error) {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			if !opts.FilesOnly {
				t.Fatal("expected files-only walk")
			}
			var entries []agentruntime.WalkEntry
			for file := range files {
				if root == "." || file == root || strings.HasPrefix(file, root+"/") {
					entries = append(entries, agentruntime.WalkEntry{Path: file, Kind: "file"})
				}
			}
			return entries, false, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ed, err := editor.New(".", editor.WithSource(source), editor.WithLanguage(editor.Go))
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := ed.FindSymbols(ctx, editor.SymbolSelector{Name: "run", Kind: editor.SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 {
		t.Fatalf("expected one runtime-backed symbol, got %#v", symbols)
	}
}

func TestWalkSourceReportsTruncatedReads(t *testing.T) {
	ctx := context.Background()
	source, err := agentruntime.NewSource(
		func(context.Context, string, int64) ([]byte, bool, error) {
			return []byte("package main\n"), true, nil
		},
		func(context.Context, editor.Scope) ([]string, error) {
			return []string{"main.go"}, nil
		},
		agentruntime.WithMaxBytes(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.ReadFile(ctx, "main.go")
	if err == nil || !strings.Contains(err.Error(), "exceeds read limit") {
		t.Fatalf("expected read limit error, got %v", err)
	}
}
