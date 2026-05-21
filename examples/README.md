# Examples

This directory contains copyable policy examples for consumers. The examples are
not built-in codegate architectures; projects own their own layer names, allowed
dependency directions, side-effect rules, coupling thresholds, and exceptions.

## Architecture Policies

Architecture policies are passed to `Engine.Assess` through
`codegate.AssessmentOptions.Architecture` or to the CLI with `--rules`.

```sh
go run ./cmd/codegate --root . --language go assess \
  --gate architecture \
  --rules examples/agentruntime-architecture.rules.json \
  --summary-only
```

The bundled `agentruntime-architecture.rules.json` shows a larger layered
runtime shape with:

- consumer-defined layers such as `core`, `runtime`, `plugins`, and `apps`;
- allowed dependency directions between those layers;
- side-effect rules for imports and calls that should stay behind project-owned
  boundaries;
- reviewed fan-out exceptions with reasons.

Use it as a template for structure and naming, then replace the layers and rules
with the consuming project's actual architecture.
