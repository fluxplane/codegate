package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFailsForInvalidRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := run(context.Background(), missing, false, 0)
	if err == nil {
		t.Fatal("expected invalid root to fail")
	}
}

func TestRunPrintsOccurrenceCounts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.go"), []byte(`package demo

import "fmt"

var Counter = 0

func Target() {}

func Use() string {
	Counter++
	Target()
	return fmt.Sprint(Counter)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := run(context.Background(), root, false, 2); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "validation: passed=true") || !strings.Contains(output, "limitations: ast_only=true") || !strings.Contains(output, "occurrences:") || !strings.Contains(output, "import=") || !strings.Contains(output, "call=") {
		t.Fatalf("expected validation, limitation, and occurrence counts in output:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
