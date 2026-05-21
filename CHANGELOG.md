# Changelog

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
