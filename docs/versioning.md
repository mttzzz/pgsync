# Versioning

pgsync uses semantic version tags (`vMAJOR.MINOR.PATCH`) generated on pushes to `main` after CI passes.

The automatic bump uses Conventional Commit-style messages since the latest semver tag:

- `feat:` bumps minor.
- `fix:`, `chore:`, `docs:`, `test:`, and other non-feature commits bump patch.
- `BREAKING CHANGE` in the commit body or `type!:` bumps major.

The workflow updates `VERSION`, commits `chore(release): bump version to vX.Y.Z [skip ci]`, creates tag `vX.Y.Z`, and pushes both. The tag triggers the release workflow.
