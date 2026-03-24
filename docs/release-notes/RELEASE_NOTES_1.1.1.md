# [1.1.1] Release Notes - 2026-03-24

## Release Summary

Patch release focused on runtime hardening, observability, and correctness. Fixes config fields that were silently ignored, adds graceful shutdown and request correlation, introduces CORS and body-size controls, and raises test coverage from 83% to 93%.

## Fixed

- **`RequestTimeout` and `RefreshInterval` config fields were dead code.** Both were parsed from environment variables but never wired into the runtime. Setting `CLAUDE_OAUTH_PROXY_REQUEST_TIMEOUT=30m` or `CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL=2m` had no effect. Both are now validated at startup and applied at runtime.
- **Request timeout could kill long streaming responses.** Timeout was previously applied via `http.Client.Timeout`, which covers the full request lifecycle including body reads. Long SSE streams exceeding the timeout were silently truncated. Timeout is now applied as a per-call context deadline on non-streaming operations only; streaming calls are exempt.
- **Auto-refresh goroutine leaked on server bind failure.** If the HTTP server failed to start (e.g. port in use), the background token refresh goroutine continued running indefinitely. It is now tied to a cancellable context that is cleaned up when `runServe` exits.
- **Streaming errors mid-flight were silently dropped.** Non-EOF errors during SSE streaming were swallowed without logging. They are now logged as `stream.error` events.
- **Request finish logging used `context.Background()`.** The end-of-request log entry dropped request-scoped context values (trace IDs, deadline info). It now uses `r.Context()`.
- **Interactive login prompt blocked on Ctrl+C.** `executeLoginFlow` blocked on `ReadString('\n')` and did not respond to context cancellation. Pressing Ctrl+C during the "code:" prompt could leave the process stuck. Stdin reading now races against `ctx.Done()`.
- **CORS preflight from disallowed origins returned 204.** `OPTIONS` requests from non-allowed origins got a bare `204 No Content` instead of falling through to the route handler. Preflight short-circuit now only fires for allowed origins.
- **Missing `Vary` headers on CORS responses.** Dynamic `Access-Control-Allow-Origin` was set without `Vary: Origin`, causing potential cache mismatches behind CDN/reverse proxy. Preflight responses now also include `Vary: Access-Control-Request-Method, Access-Control-Request-Headers`.
- **`RefreshSkew` parse error lacked field context.** Error was the raw `time.ParseDuration` message without indicating which field failed. Now prefixed with `"refresh skew: "`.

## Added

- **Graceful shutdown** — `signal.NotifyContext` in `main` catches `SIGTERM`/`SIGINT` and initiates clean HTTP server shutdown. Previously the process only stopped on `SIGKILL`.
- **Request ID middleware** — Generates a random `X-Request-ID` (or propagates the client-provided value). Included in all structured log entries as `request_id`.
- **CORS support** — New `CLAUDE_OAUTH_PROXY_CORS_ORIGINS` env var. Set to comma-separated origins (e.g. `http://localhost:3000,http://example.com`) or `*` for all origins. Disabled by default.
- **Request body size limit** — New `CLAUDE_OAUTH_PROXY_MAX_REQUEST_BODY` env var (default `10MB`, supports `KB`/`MB`/`GB` suffixes). Enforced via `http.MaxBytesReader` on `/v1/chat/completions`.
- **`estimateThinkingTokens` helper** — Extracted the `(chars+3)/4` heuristic from two inline call sites into a named, tested function.
- **CI static analysis** — `go vet ./...` and `golangci-lint` steps added to CI before test execution.

## Changed

- `NewHandler` now accepts a `HandlerConfig` struct carrying CORS and body-limit settings.
- `App` struct exposes `RefreshInterval` as a parsed `time.Duration` instead of callers re-parsing the config string.
- `autoRefresh` thin wrapper removed; `autoRefreshWithInterval` called directly with the config-derived duration.
- `/v1/models` endpoint documented as intentionally unauthenticated in handler code comment and README.

## Admin/Operations

- Coverage raised from 83.2% to 93.1% (90% CI threshold now passes).
- `go vet ./...` added to CI pipeline (per coding handbook baseline).
- `golangci-lint-action@v8` added to CI pipeline.
- Coverage build artifacts (`coverage.out`, `coverage_all.out`) confirmed in `.gitignore` and untracked.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.1
```

## Breaking Changes

None. All changes are backwards-compatible.

- `NewHandler` signature changed (added `HandlerConfig` parameter), but this is an internal API — no external consumers.
- `App` struct gained `RefreshInterval` field — additive only.
- New env vars (`CORS_ORIGINS`, `MAX_REQUEST_BODY`) default to disabled/10MB respectively, preserving existing behavior.

## Known Issues

- Reasoning token counts remain estimated (~4 chars/token heuristic). Anthropic does not expose a separate thinking token count.
- `readLineWithContext` leaves a blocked goroutine on stdin if the context is cancelled before input arrives. The goroutine will exit when the process terminates or stdin is closed, but it is not force-interrupted.
- Single-writer token model unchanged — do not scale above one replica without external coordination.

## Compatibility and Migration

- No configuration changes required for existing deployments.
- New env vars are optional with safe defaults.
- `CLAUDE_OAUTH_PROXY_REQUEST_TIMEOUT` and `CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL` now take effect. If you were relying on them being silently ignored (unlikely), verify your values parse as valid Go durations.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.0...1.1.1
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/1.1.1
