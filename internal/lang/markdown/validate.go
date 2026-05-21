package markdown

import (
	"context"
	"fmt"
	"sort"
)

func (b MarkdownBackend) Validate(ctx context.Context, snapshot Snapshot, opts ValidationOptions) (ValidationResult, error) {
	kinds := markdownValidationKinds(opts.Kinds)
	result := ValidationResult{
		Passed:         true,
		Kinds:          append([]ValidationKind(nil), kinds...),
		ResolutionMode: "structural",
		Complete:       true,
	}
	idx, err := buildIndex(ctx, snapshot, opts.Scope)
	if err != nil {
		return ValidationResult{}, err
	}
	for _, file := range idx.files {
		result.AffectedPaths = append(result.AffectedPaths, file.path)
	}
	for _, kind := range kinds {
		switch kind {
		case ValidationParse:
		case ValidationTypecheck:
			result.Complete = false
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: "warning",
				Message:  "markdown backend does not provide typecheck validation",
			})
		default:
			result.Complete = false
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: "warning",
				Message:  fmt.Sprintf("unsupported Markdown validation kind %q", kind),
			})
		}
	}
	result.Diagnostics = append(result.Diagnostics, idx.diagnostics...)
	if len(result.Diagnostics) > 0 {
		result.Passed = false
	}
	sort.Strings(result.AffectedPaths)
	return result, nil
}

func markdownValidationKinds(kinds []ValidationKind) []ValidationKind {
	if len(kinds) == 0 {
		return []ValidationKind{ValidationParse}
	}
	seen := map[ValidationKind]bool{}
	out := make([]ValidationKind, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	if len(out) == 0 {
		return []ValidationKind{ValidationParse}
	}
	return out
}
