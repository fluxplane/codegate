package golang

import (
	"testing"

	"github.com/codewandler/codegate"
)

func TestNewReturnsGoBackend(t *testing.T) {
	backend := New(Config{})
	spec := backend.Spec()
	if spec.Language != codegate.Go || len(spec.Operations.EditOperations) == 0 {
		t.Fatalf("unexpected Go backend spec %#v", spec)
	}
}
