package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunFailsForInvalidRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := run(context.Background(), missing, false, 0)
	if err == nil {
		t.Fatal("expected invalid root to fail")
	}
}
