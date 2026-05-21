package codegate

import (
	"context"
	"strings"
)

// ValidationAdapterFunc is the callback used by NewValidationAdapter.
type ValidationAdapterFunc func(ctx context.Context, snapshot Source, opts ValidationOptions) (ValidationResult, error)

type namedValidationAdapter struct {
	name string
	fn   ValidationAdapterFunc
}

// NewValidationAdapter creates an explicit, caller-owned validation adapter.
//
// Adapters are only run when callers name them in ValidationOptions.External.
// Core validation never shells out or runs build/test commands implicitly.
func NewValidationAdapter(name string, fn ValidationAdapterFunc) ValidationAdapter {
	return namedValidationAdapter{name: strings.TrimSpace(name), fn: fn}
}

func (a namedValidationAdapter) Name() string {
	return a.name
}

func (a namedValidationAdapter) Validate(ctx context.Context, snapshot Source, opts ValidationOptions) (ValidationResult, error) {
	if a.fn == nil {
		return ValidationResult{Passed: false, Complete: false, Kinds: []ValidationKind{ValidationExternal}}, nil
	}
	return a.fn(ctx, snapshot, opts)
}
