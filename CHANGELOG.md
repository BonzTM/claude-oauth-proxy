# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.3] - 2026-03-25

### Fixed

- Release-blocking `errcheck` failures in CLI output paths are now handled explicitly across command, usage, status, and login flows.
- Existing `Close()` cleanup paths in the HTTP adapter, OAuth token exchange flow, and OpenRouter pricing fetch now handle ignored return values explicitly, resolving CI lint failures without changing runtime behavior.
- Anthropic provider tests now make body/stream cleanup explicit so static analysis passes consistently.

### Changed

- `internal/runtime/service_factory.go` now relies on inferred local typing for `providerService`, resolving a `staticcheck` warning without changing runtime behavior.

See [docs/release-notes/RELEASE_NOTES_1.1.3.md](docs/release-notes/RELEASE_NOTES_1.1.3.md) for the full release notes.

## [1.1.2] - 2026-03-25

### Added

- Theoretical cost tracking via OpenRouter pricing data. Enable with `--cost-tracking` flag or `CLAUDE_OAUTH_PROXY_COST_TRACKING=true` env var.
- Per-request cost details in response body (`usage.cost` field) with input, output, cache write, cache read, and total cost breakdowns in USD.
- `X-Request-Cost` response header on non-streaming requests (e.g. `0.010500 USD`).
- Cost logging via structured log event `cost.tracked` with per-component cost fields.
- Cache-aware cost calculation: cache writes priced at 1.25x input, cache reads at 0.1x input.
- OpenRouter pricing source with `anthropic/` prefix fallback for model name resolution.
- `CLAUDE_OAUTH_PROXY_OPENROUTER_URL` env var for custom OpenRouter endpoint (defaults to `https://openrouter.ai/api/v1/models`).

### Changed

- `X-Request-Cost` added to CORS `Access-Control-Expose-Headers` for browser client visibility.
- OpenRouter pricing fetch is now lazy (on first cost lookup) instead of app startup, preventing `login`/`status`/`logout` from depending on OpenRouter network reachability when cost tracking is enabled.

### Fixed

- Resolved a cost-tracking regression where non-serving CLI commands (`login`, `status`, `logout`) could block on OpenRouter pricing fetch during app initialization.

See [docs/release-notes/RELEASE_NOTES_1.1.2.md](docs/release-notes/RELEASE_NOTES_1.1.2.md) for the full release notes.

## [1.1.1] - 2026-03-24

### Added

- Graceful shutdown via `SIGTERM`/`SIGINT` signal handling using `signal.NotifyContext`.
- Request ID middleware — generates or propagates `X-Request-ID` headers and includes `request_id` in structured log entries.
- Configurable CORS support via `CLAUDE_OAUTH_PROXY_CORS_ORIGINS` (comma-separated origins or `*`).
- Configurable request body size limit via `CLAUDE_OAUTH_PROXY_MAX_REQUEST_BODY` (default `10MB`).
- `http.MaxBytesReader` on `/v1/chat/completions` to reject oversized request bodies.
- `go vet` and `golangci-lint` steps in CI pipeline.
- Context-aware stdin reading during interactive login — Ctrl+C now exits the prompt reliably.
- `estimateThinkingTokens` shared helper, replacing duplicated inline heuristic.

### Fixed

- `CLAUDE_OAUTH_PROXY_REQUEST_TIMEOUT` and `CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL` are now validated at startup and wired into the runtime. Previously these config fields were parsed but silently ignored.
- Request timeout is applied per non-streaming API call via context deadline instead of `http.Client.Timeout`, preventing long SSE streams from being killed mid-flight.
- Auto-refresh goroutine lifecycle is now tied to a cancellable context. Previously the goroutine would leak if the HTTP server failed to bind.
- Streaming errors mid-flight are now logged (`stream.error`) instead of silently dropped.
- Request finish logging now uses `r.Context()` instead of `context.Background()`, preserving request-scoped values.
- CORS preflight from disallowed origins falls through to the route handler instead of returning a bare `204`.
- CORS responses include `Vary: Origin` (and preflight adds `Vary: Access-Control-Request-Method, Access-Control-Request-Headers`) to prevent cross-origin cache mismatches.
- `RefreshSkew` parse errors now include field context in the error message (`"refresh skew: ..."` instead of raw parse error).

### Changed

- `NewHandler` accepts a `HandlerConfig` struct for CORS and body-limit settings.
- `App` struct exposes `RefreshInterval` as a parsed `time.Duration` so callers no longer need to re-parse the string.
- `autoRefresh` wrapper removed; `autoRefreshWithInterval` used directly with the config-derived interval.
- `/v1/models` endpoint documented as intentionally unauthenticated (code comment and README).

## [1.1.0] - 2026-03-23

### Added

- Extended thinking / reasoning support via `reasoning_effort` request field (`low`/`medium`/`high`).
- `completion_tokens_details.reasoning_tokens` in usage responses for reasoning token visibility.
- `prompt_tokens_details.cached_tokens` in usage responses for OpenAI-format cache metric display.
- `index` field on streaming tool call chunks for OpenAI spec compliance.
- Client fingerprint matching with Claude Code (User-Agent, Stainless headers, billing header).
- 6 environment variables for fingerprint overrides without rebuilding.
- `scripts/extract-cc-fingerprint.sh` for deriving fingerprint values from installed Claude Code.
- Fingerprint maintenance guide (`docs/maintainers/FINGERPRINT.md`).
- Prompt caching documentation (`docs/caching.md`).
- Client integration examples for opencode, aider, Continue, and OpenAI Python SDK.
- Community docs: CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md, CHANGELOG.md.
- GitHub issue/PR templates and release notes template.
- Cross-navigation links across all documentation pages.

### Fixed

- `prompt_tokens` now includes cached + non-cached input tokens, fixing negative token displays.
- Streaming tool call chunks include required `index` field, fixing opencode validation errors.
- Helm chart release workflow stamps version at release time instead of requiring pre-matched Chart.yaml.
- Helm chart release waits for container image in GHCR before packaging.

### Changed

- Billing header uses dynamic `cc_version={version}.{turn}` format matching Claude Code.
- Model references updated from `claude-sonnet-4-5` to `claude-sonnet-4-6`.
- Fixed `CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA` default in docs.
- Refresh interval/skew docs updated from "Reserved" to actual behavior.

See [docs/release-notes/RELEASE_NOTES_1.1.0.md](docs/release-notes/RELEASE_NOTES_1.1.0.md) for the full release notes.

## [1.0.0] - 2026-03-23

Initial public release of claude-oauth-proxy.

- OpenAI-compatible API proxy for Claude via OAuth (`GET /v1/models`, `POST /v1/chat/completions`)
- Streaming and non-streaming chat completions
- Tool use / function calling with OpenAI-to-Anthropic translation
- Automatic prompt caching with cache breakpoints on user messages and tool definitions
- OAuth login flow with browser and headless modes
- Token persistence, background refresh, and automatic 401 retry
- Claude CLI credential reuse via seed file
- Docker Compose and Helm chart deployment paths
- CLI: `serve`, `login`, `logout`, `status`, `config validate`, `version`
- Health and readiness endpoints
- Full documentation: configuration reference, caching guide, deployment guides, testing guide
- Community docs: CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md, GitHub issue/PR templates

See [docs/release-notes/RELEASE_NOTES_1.0.0.md](docs/release-notes/RELEASE_NOTES_1.0.0.md) for the full release notes.

[Unreleased]: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.3...HEAD
[1.1.3]: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.2...1.1.3
[1.1.2]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/BonzTM/claude-oauth-proxy/releases/tag/v1.0.0
