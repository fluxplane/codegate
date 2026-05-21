# Changelog

## v0.3.0 - 2026-05-21

### Highlights

- Added standalone HTML assessment reports for `codegate assess --format html`.
- HTML reports include module/ref metadata, score/rating cards, tables, embedded JSON download, expandable evidence blocks, and CDN-backed syntax highlighting.
- Added compact JSON rating metadata with `rating` and `score_max`.
- Added a Go-specific README quality badge convention and bot update recipe.

### Added

- `Scope.IncludeGenerated` plus `--generated` and `--include-generated` CLI flags so callers can opt generated Go source into analysis.
- Package-loader Go typecheck validation for disk-backed workspaces.
- Workspace-root plumbing for sources that can safely support real package loading.
- Release checklist coverage for HTML report rendering.

### Changed

- Generated Go files are excluded from Go assessment by default while still remaining available to package-loader typechecking context.
- Go validation falls back to AST-only typechecking for in-memory sources and pending overlays.
- README scoring docs now explain the 0-100 score bounds, rating scale, pressure score, HTML reports, generated-code behavior, and Go validation mode.

### Fixed

- Reduced false typecheck diagnostics from unresolved external dependency aliases by using real Go package loading when possible.
- Preserved generated-file context for disk-backed typecheck validation even when generated files are excluded from selected report scope.

### Breaking Changes

- None.

### Validation

- `go test ./...`
- `git diff --check`
- `go run ./cmd/codegate --root . --language go capabilities`
- `go run ./cmd/codegate --root . --language markdown assess --gate all`
- `go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3`
- `go run ./cmd/codegate --root . --language go assess --gate all --view full --suggestions 3`
- `go run ./cmd/codegate --root . --language go assess --gate all --summary-only`
- `go run ./cmd/codegate --root . --language go --format html assess --gate all > /tmp/codegate-release-report.html`
- `go run ./cmd/codegate --root . --language go assess --gate architecture --rules examples/agentruntime-architecture.rules.json --summary-only`

## v0.2.0 - 2026-05-21

### Changed

- Renamed the Go module from `github.com/codewandler/codegate` to `github.com/fluxplane/codegate`.

### Added

- Generic `codegate op run` CLI operation runner for agent-driven structured edit operations.
- Go module path rename operation with dry-run, patch, validation, and explicit write support.
- Agent release instructions in `AGENTS.md`.

### Fixed

- Hardened Go module path literal rewrites so unrelated substrings such as longer path tokens are not modified.
- Added package-local test coverage across internal helpers, Go backend module rename behavior, Markdown indexing, and public language wrappers.

## v0.1.0 - 2026-05-21

First public release of `github.com/fluxplane/codegate`.

### Added

- Agent-facing engine API for lookup, assessment, suggestions, in-memory changes, validation, and diffs.
- Go backend with AST-backed lookup, assessment, structured edit/refactor operations, parse validation, and best-effort typecheck validation.
- Markdown backend with structural lookup, quality assessment, validation, and conservative cleanup operations.
- Consumer-defined Go architecture rules for layers, dependency direction, side-effect rules, coupling thresholds, reviewed exceptions, and fail-on categories.
- Go quality, maintainability, coverage, safety, security, and performance metrics exposed through language capability metadata.
- Explicit caller-owned validation adapters for opt-in build, test, or policy workflows.
- JSON-first `cmd/codegate` CLI with compact, summary, and full assessment views for agent workflows.
- `adapter/agentruntime` source bridge for integrating codegate with agentruntime-style workspace readers.

### Notes

- Core codegate does not run hidden shell commands, git commands, builds, tests, or persistent disk writes.
- Go analysis is AST-first and local-module focused; full package loading and type-aware external dependency analysis are future work.
- Markdown support is structural and intentionally conservative.
- Architecture policies are currently Go-only and AST-backed.
