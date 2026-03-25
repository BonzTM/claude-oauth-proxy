# [1.1.3] Release Notes - 2026-03-25

## Release Summary

Maintenance release focused on static-analysis cleanup and release readiness. This patch makes existing best-effort write and cleanup handling explicit across CLI, HTTP, auth, cost-fetch, and provider test paths, and removes one internal `staticcheck` warning in the runtime factory. No new features or configuration changes are included in this release.

## Fixed

- **CLI lint failures** — Previously ignored `fmt.Fprintf`, `fmt.Fprintln`, and `fmt.Fprint` return values in command, usage, status, and login output paths are now handled explicitly so `errcheck` no longer blocks release validation.
- **HTTP/auth/cost cleanup lint failures** — Existing body and stream cleanup paths in the HTTP adapter, OAuth token exchange flow, and OpenRouter pricing fetch now explicitly handle ignored `Close()` return values, preserving behavior while satisfying `errcheck`.
- **Test cleanup lint failures** — Anthropic stream tests now make body/stream cleanup explicit so `golangci-lint` passes consistently.

## Added

- None. This patch is maintenance-only.

## Changed

- **Runtime factory maintenance** — `providerService` in `internal/runtime/service_factory.go` now uses inferred local typing instead of a redundant explicit interface type to satisfy `staticcheck`.

## Admin/Operations

- None beyond the maintenance fixes listed above.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.3
```

## Breaking Changes

None. All changes are internal maintenance and fully backwards-compatible.

## Known Issues

- Known issues from 1.1.2 remain unchanged; this patch does not add new user-facing behavior.

## Compatibility and Migration

- No configuration changes or migration steps are required for this release.
- Existing 1.1.2 deployments can upgrade directly to 1.1.3.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.2...1.1.3
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/1.1.3
