# [1.1.9] Release Notes - 2026-04-14

## Release Summary

Fingerprint maintenance release. Updates the Claude Code client version defaults from `2.1.91` to `2.1.107`.

## Changed

- **Claude Code version bump** — `DefaultCCVersion` updated from `2.1.91` to `2.1.107` in `internal/runtime/config.go`.
- **User-Agent default** — `DefaultUserAgent` updated from `claude-cli/2.1.91 (external, cli)` to `claude-cli/2.1.107 (external, cli)`.
- **Documentation** — Configuration reference (`docs/configuration.md`) and fingerprint maintenance guide (`docs/maintainers/FINGERPRINT.md`) updated to reflect the new version.
- **Extraction script** — Comment example in `scripts/extract-cc-fingerprint.sh` updated to `2.1.107`.

## Fixed

- None.

## Added

- None.

## Admin/Operations

- None beyond the fingerprint update listed above.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.9
```

Alternatively, set the new version via environment variables on an existing binary without rebuilding:

```bash
export CLAUDE_OAUTH_PROXY_CC_VERSION=2.1.107
export CLAUDE_OAUTH_PROXY_CC_USER_AGENT="claude-cli/2.1.107 (external, cli)"
```

## Breaking Changes

None. All changes are default-value updates and fully backwards-compatible.

## Known Issues

- `softprops/action-gh-release@v2` and `peaceiris/actions-gh-pages@v4` remain on Node 20 as no upstream Node 24 release is available yet. These will be updated when new major versions are published.
- Other known issues from 1.1.8 remain unchanged; this patch does not add new user-facing behavior.

## Compatibility and Migration

- No configuration changes or migration steps are required for this release.
- Existing 1.1.8 deployments can upgrade directly to 1.1.9.
- Users who already override `CLAUDE_OAUTH_PROXY_CC_VERSION` and `CLAUDE_OAUTH_PROXY_CC_USER_AGENT` via environment variables are unaffected by this change.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.8...1.1.9
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/1.1.9
