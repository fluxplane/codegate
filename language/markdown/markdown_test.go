package markdown

import (
	"testing"

	"github.com/codewandler/codegate"
)

func TestNewReturnsMarkdownBackend(t *testing.T) {
	backend := New(Config{})
	spec := backend.Spec()
	if spec.Language != codegate.Markdown || len(spec.Operations.EditOperations) == 0 {
		t.Fatalf("unexpected Markdown backend spec %#v", spec)
	}
}
