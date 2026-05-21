# Release Checklist

Run these checks before tagging a release:

```sh
go test ./...
git diff --check
go run ./cmd/codegate --root . --language go capabilities
go run ./cmd/codegate --root . --language markdown assess --gate all
go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3
go run ./cmd/codegate --root . --language go assess --gate all --view full --suggestions 3
go run ./cmd/codegate --root . --language go assess --gate all --summary-only
go run ./cmd/codegate --root . --language go assess --gate architecture --rules examples/agentruntime-architecture.rules.json --summary-only
```

The normal test suite includes an external consumer test. It creates a temporary
module, imports only public codegate packages, and runs the engine against Go and
Markdown fixtures.

Use the default compact `assess` view in agent loops that need scores, compact
metrics, finding counts, and small top lists without full evidence payloads. Use
`--view full` only when complete evidence is needed.

External validation adapters are covered by unit tests with fake in-process
runners. The release checklist intentionally does not run shell/build/test
adapters because codegate core must not execute external commands implicitly.

Before tagging, skim `README.md`, `examples/README.md`, and the public examples
in `example_test.go`. They are the public API smoke test for the engine builder,
language packages, Markdown cleanup, and agentruntime source adapter.

Tag placeholder:

```sh
git tag v0.1.0
```
