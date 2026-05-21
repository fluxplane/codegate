# Agent Instructions

## Commit Rules

- Do not create commits unless the user explicitly asks for a commit.
- Use Conventional Commits for commit messages, for example `feat: ...`,
  `fix: ...`, `docs: ...`, `test: ...`, `refactor: ...`, or `chore: ...`.
- Keep commits focused. Do not mix unrelated implementation, release, and
  cleanup changes in one commit unless the user explicitly asks for that.
- If `CHANGELOG.md` does not exist, create it before preparing a release.
- Use semantic versioning for release decisions:
  - patch for compatible bug fixes and documentation-only release fixes
  - minor for backward-compatible features
  - major for breaking API, CLI, behavior, or compatibility changes

## Release Process

1. Ensure the repository is clean before release preparation:
   ```sh
   git status --short
   ```
2. Identify the previous release tag:
   ```sh
   git tag --sort=-v:refname
   ```
3. Compare changes since the previous tag:
   ```sh
   git diff --stat <previous-tag>..HEAD
   git log --oneline <previous-tag>..HEAD
   ```
4. Decide the next semantic version from the actual changes since the previous
   release. Call out breaking changes explicitly.
5. Update `CHANGELOG.md` with the new version, date, highlights, breaking
   changes if any, and notable fixes.
6. Create exactly one release-prep commit for the changelog update:
   ```sh
   git add CHANGELOG.md
   git commit -m "docs: prepare vX.Y.Z release"
   ```
7. Create an annotated tag:
   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   ```
8. Push the main branch, then push the tag:
   ```sh
   git push origin main
   git push origin vX.Y.Z
   ```
9. Ensure there is a GitHub release for the tag using `gh`.
   Include detailed release notes with:
   - highlights
   - breaking changes
   - fixes
   - upgrade notes
   - validation performed

## Release Safety

- Do not tag from a dirty worktree.
- Do not push unless the user explicitly asks for publishing or release.
- Before tagging, run the repository release checks documented in `RELEASE.md`
  when present.
- Verify that the changelog version, annotated tag, and GitHub release version
  all match exactly.

## Claude Code Plugin

- The Claude Code plugin bundle lives in `plugins/codegate`.
- Keep the plugin self-contained:
  - `plugins/codegate/.claude-plugin/plugin.json`
  - `plugins/codegate/skills/`
  - `plugins/codegate/commands/`
  - `plugins/codegate/agents/`
- Do not add a root `.claude-plugin/marketplace.json` unless the user
  explicitly asks to make this repository a Claude plugin marketplace.
- Validate plugin changes before committing or releasing:
  ```sh
  claude plugin validate ./plugins/codegate
  ```
- Test local loading with:
  ```sh
  claude --plugin-dir ./plugins/codegate agents
  ```
- Claude Code persistent installs currently go through marketplaces. For direct
  repository use, document `--plugin-dir` against a local checkout or sparse
  checkout of `plugins/codegate`.
