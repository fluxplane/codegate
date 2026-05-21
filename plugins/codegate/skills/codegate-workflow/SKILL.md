---
name: codegate-workflow
description: Use codegate to assess code quality, architecture, maintainability, safety, and improvement opportunities before or after code changes.
when_to_use: Use when reviewing a repository, validating a refactor, comparing quality before and after edits, investigating architecture boundaries, or deciding which code improvements to make next.
---

# Codegate Workflow

Use the installed `codegate` CLI to produce structured evidence before making quality claims.

Default workflow:

1. Run a compact assessment for the relevant language:
   `codegate --root . --language go --format json assess --gate all`
2. Inspect `summary`, `rating`, `scores`, `validation`, `finding_counts`, and `finding_category_counts`.
3. Use full view only when evidence or exact locations are needed:
   `codegate --root . --language go --format json assess --gate all --view full --suggestions 3`
4. Treat hard violations, diagnostics, and validation failures as higher priority than soft quality findings.
5. When proposing fixes, target code changes that reduce findings or violations. Do not change scoring logic unless the task is specifically about scoring behavior.
6. Re-run the same assessment after edits and report the score, rating, validation status, and material finding-count changes.

Use `--language markdown` for Markdown documents and `--gate architecture --rules <file>` when the repository provides architecture rules.

If `codegate` is not installed, tell the user to install it with:

```sh
go install github.com/fluxplane/codegate/cmd/codegate@latest
```

Do not write changes with `codegate op run --write` unless the user explicitly asks for workspace writes. Prefer dry-run JSON or patch output for exploratory operations.
