# Release Checklist

Run these checks before tagging a release:

```sh
go test ./...
git diff --check
go run ./cmd/codegate --root . --language go capabilities
go run ./cmd/codegate --root . --language markdown assess --gate all
go run ./cmd/codegate --root . --language go assess --gate all --suggestions 3
```

The normal test suite includes an external consumer test. It creates a temporary
module, imports only public codegate packages, and runs the engine against Go and
Markdown fixtures.

Tag placeholder:

```sh
git tag v0.1.0
```
