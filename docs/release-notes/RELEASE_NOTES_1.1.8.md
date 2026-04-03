# [1.1.8] Release Notes - 2026-04-03

## Release Summary

Bug fix release. Resolves a crash-loop on first Kubernetes deploy where the pod would exit before the user could exec in to complete the initial OAuth login.

## Fixed

- **Container bootstrap crash-loop** — The `serve` command previously attempted an interactive login flow when no tokens existed. In a container (no interactive stdin), this caused an immediate exit before the HTTP server started, making `kubectl exec ... login --no-browser` impossible. The container image now sets `CLAUDE_OAUTH_PROXY_NO_AUTO_LOGIN=true` by default, so the server starts in an unauthenticated state and waits for a separate `login` command.

## Added

- **`CLAUDE_OAUTH_PROXY_NO_AUTO_LOGIN`** environment variable and `NoAutoLogin` config field. When set to `true`, `1`, or `yes`, the `serve` command skips the interactive login prompt on startup and starts the HTTP server immediately. The readiness probe (`/readyz`) returns `503 not_ready` until a valid OAuth session is established via a separate `login` invocation. The liveness probe (`/livez`) is unaffected and returns `200 OK` as usual.

## Changed

- None.

## Admin/Operations

- The Dockerfile now sets `CLAUDE_OAUTH_PROXY_NO_AUTO_LOGIN=true` in the default environment. Local binary users are unaffected — the default remains interactive login on first run.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.8
```

The Kubernetes bootstrap flow documented in the README and `docs/deploy/kubernetes.md` now works as written:

```bash
# 1. Install chart (pod starts, readiness probe reports not_ready)
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy --create-namespace \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy

# 2. Bootstrap login inside the running pod
kubectl exec -it -n claude-oauth-proxy deployment/claude-oauth-proxy -- \
  /usr/local/bin/claude-oauth-proxy login --no-browser

# 3. Readiness probe passes, proxy begins serving traffic
```

## Breaking Changes

None. Local binary behavior is unchanged. Container-only change is additive — the pod now survives startup without tokens instead of crash-looping.

## Known Issues

- `softprops/action-gh-release@v2` and `peaceiris/actions-gh-pages@v4` remain on Node 20 as no upstream Node 24 release is available yet. These will be updated when new major versions are published.

## Compatibility and Migration

- No configuration changes or migration steps required.
- Existing 1.1.7 deployments can upgrade directly to 1.1.8.
- Users who override `CLAUDE_OAUTH_PROXY_NO_AUTO_LOGIN` can set it to `false` or unset it to restore the previous interactive-login-on-startup behavior in containers.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.7...1.1.8
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/1.1.8
