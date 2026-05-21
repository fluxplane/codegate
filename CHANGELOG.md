# Changelog

## v1.1.0 - 2026-05-22

### Highlights

- Added a Claude Code plugin bundle under `plugins/codegate` with one skill,
  one command, and one reviewer agent for codegate-driven assessment workflows.
- Added README getting-started instructions for installing the CLI with
  `go install` and running installed `codegate` commands directly.
- Documented Claude plugin loading, validation, and maintenance rules for
  direct `--plugin-dir` use from a checkout.

### Added

- Claude Code skill `codegate:codegate-workflow`.
- Claude Code command `/codegate:assess`.
- Claude Code agent `codegate:reviewer`.
- Agent maintenance instructions for keeping the plugin self-contained and
  avoiding an accidental root marketplace manifest.

### Changed

- README CLI examples now use the installed `codegate` binary instead of
  `go run ./cmd/codegate`.

### Fixed

- None.

### Breaking Changes

- None.

### Upgrade Notes

- Existing Go API and CLI callers do not need code changes.
- Claude Code users can load the plugin from a checkout with
  `claude --plugin-dir ./plugins/codegate`.
- Persistent Claude Code plugin installs still require a marketplace in the
  current Claude CLI; this release intentionally ships the direct plugin bundle
  without a root marketplace manifest.

### Validation

- `go test ./...`
- `git diff --check`
- `claude plugin validate ./plugins/codegate`
- `claude --plugin-dir ./plugins/codegate agents`
- `go run ./cmd/codegate --root . --language go capabilities`
- `go run ./cmd/codegate --root . --language markdown assess --gate all`
- `go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3`
- `go run ./cmd/codegate --root . --language go assess --gate all --view full --suggestions 3`
- `go run ./cmd/codegate --root . --language go assess --gate all --summary-only`
- `go run ./cmd/codegate --root . --language go --format html assess --gate all > /tmp/codegate-release-report.html`
- `go run ./cmd/codegate --root . --language go assess --gate architecture --rules examples/agentruntime-architecture.rules.json --summary-only`

## v1.0.0 - 2026-05-22

### Highlights

- Normalized Go assessment scoring across soft quality categories so scores
  move with finding density instead of saturated raw counts.
- Normalized Markdown assessment scoring for structural findings, debt markers,
  and validation-diagnostic drag using document, heading, and line denominators.
- Improved this repository's Go quality signal from `55 C+` before cleanup to
  `95 A+` after quality cleanup and normalized scoring.
- Added score invariant tests covering density, severity weighting, pressure,
  coupling, architecture policy soft penalties, and normalized component
  aggregation.

### Changed

- Go scoring model is now `go-architecture-v1`.
- Markdown scoring model is now `markdown-structure-v1`.
- Go soft scoring now normalizes maintainability findings, suggestions,
  pressure, coupling fan-out, architecture policy soft findings, safety/security
  side-effect findings, and final violation drag where a denominator exists.
- Markdown metrics now include `document_count`, `heading_count`, and
  `line_count` for normalized structural scoring.
- README quality badge now reflects the current Go assessment: `95 A+`.

### Fixed

- Reduced noisy Go performance findings by adding missing slice capacities and
  avoiding numeric `+=` false positives in string-concatenation detection.
- Added public facade documentation comments to reduce undocumented-export
  findings while preserving the public API surface.

### Breaking Changes

- Assessment score values and rating outputs may change for the same input
  repository because Go and Markdown score models now use normalized density and
  severity weighting instead of several raw-count penalties.
- Consumers that compare exact scores, score deltas, or `provider_score_model`
  values should update expectations from `go-architecture-v0` and
  `markdown-structure-v0` to the v1 model identifiers.

### Upgrade Notes

- Prefer score and rating thresholds over exact score snapshots in automation.
- Use the reported score model and denominator metrics when comparing assessment
  output across releases.
- Hard architecture boundary/test-boundary failures and no-file coverage
  failures remain hard gates rather than normalized soft penalties.

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
