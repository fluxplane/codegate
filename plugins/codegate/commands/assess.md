---
description: Run a codegate assessment for the current workspace and summarize the score, validation status, and top findings.
argument-hint: "[language] [gate]"
disable-model-invocation: true
---

# Assess With Codegate

Run a codegate assessment for this workspace.

Use `$ARGUMENTS` as optional `language` and `gate` values. If omitted, default to:

- language: `go`
- gate: `all`

Recommended command:

```sh
codegate --root . --language go --format json assess --gate all
```

For user-provided arguments, substitute them into:

```sh
codegate --root . --language <language> --format json assess --gate <gate>
```

Summarize:

- overall score and rating
- validation `passed`, `resolution_mode`, `diagnostics`, `files`, and `complete`
- component scores
- violation counts
- top finding categories
- the most actionable next steps

If the command fails because `codegate` is missing, tell the user to install it:

```sh
go install github.com/fluxplane/codegate/cmd/codegate@latest
```
