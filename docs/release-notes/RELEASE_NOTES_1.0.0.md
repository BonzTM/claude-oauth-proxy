# [1.0.0] Release Notes - 2026-03-23

## Release Summary

Initial public release of **claude-oauth-proxy**, an OpenAI-compatible API proxy for Claude. The proxy translates OpenAI-format requests into Anthropic API calls, manages OAuth token lifecycle, and applies automatic prompt caching — letting any OpenAI-compatible tool use Claude without code changes.

## Highlights

- **Drop-in OpenAI compatibility** — `GET /v1/models` and `POST /v1/chat/completions` (streaming and non-streaming) with standard `OPENAI_BASE_URL` / `OPENAI_API_KEY` configuration.
- **Tool use / function calling** — Full translation between OpenAI tool call format and Anthropic tool_use blocks, including streaming tool call deltas with proper `index` fields.
- **Automatic prompt caching** — Cache breakpoints on system messages, the last two user messages, and the last tool definition. Cache metrics (`prompt_tokens_details.cached_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`) passed through in usage responses.
- **OAuth token management** — One-time browser or headless login, persistent token storage, background refresh with configurable interval and skew, and automatic 401 retry with forced refresh.
- **Claude CLI credential reuse** — Mount `~/.claude/.credentials.json` as a read-only seed file; the proxy converts the format automatically and writes refreshed tokens separately.
- **Multiple deployment paths** — Local binary, Docker Compose, and Helm chart for Kubernetes.

## Added

### API Surface

- `GET /v1/models` — Lists available Claude models in OpenAI format.
- `POST /v1/chat/completions` — Chat completions with streaming (`text/event-stream`) and non-streaming JSON responses.
- Tool use / function calling with OpenAI-to-Anthropic translation and streaming tool call chunks.
- Health and readiness endpoints: `/health`, `/healthz`, `/livez`, `/ready`, `/readyz`.

### Prompt Caching

- Automatic `CacheControl: ephemeral` breakpoints on the last system block, last two user messages, and last tool definition.
- `prompt_tokens_details.cached_tokens` in usage responses for OpenAI-compatible client display.
- Anthropic-native `cache_creation_input_tokens` and `cache_read_input_tokens` also included.

### Authentication

- OAuth login flow with browser launch and headless (`--no-browser`, `--code`) modes.
- Token persistence to disk (`~/.config/claude-oauth-proxy/tokens.json`) with `0600` permissions.
- Background token refresh goroutine (default: check every 1 minute, refresh 5 minutes before expiry).
- Automatic 401 retry with forced token refresh on upstream auth failures.
- Seed file support for Claude CLI credential reuse without modifying the original file.

### CLI

- `serve` — Start the proxy with optional `--listen-addr`, `--api-key`, `--relogin`, `--no-browser`, `--code` flags.
- `login` — Standalone OAuth login with `--no-browser` and `--code` flags.
- `logout` — Delete the token file.
- `status` — Show token file existence and expiry state.
- `config validate` — Check required configuration values.
- `version` — Print the build version.

### Deployment

- Docker Compose example with `.env` configuration and runtime volume mount.
- Helm chart with PVC-backed token persistence, API key Secret management, optional Ingress, and `extraEnv` / `extraEnvFrom` overrides.
- Published container images with `latest`, `nightly`, and `develop` tag channels.

### Configuration

- Full environment variable configuration (17 variables) with sensible defaults for local development.
- Billing header injected as a system block for upstream compatibility.
- Configurable Anthropic beta header, base URL, OAuth endpoints, and scopes.

### Documentation

- README with quick start, client integration examples (opencode, aider, Continue, OpenAI SDK, curl), and CLI reference.
- Configuration reference (`docs/configuration.md`).
- Prompt caching guide (`docs/caching.md`).
- Deployment guides for Docker Compose and Kubernetes/Helm.
- Testing guide with coverage baseline and smoke test examples.
- CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md.
- GitHub issue templates (bug report, feature request) and PR template.

## Compatibility

- Requires Go 1.24+ for building from source.
- Tested with opencode, aider, Continue, and the OpenAI Python SDK.
- Any client that supports `OPENAI_BASE_URL` and `OPENAI_API_KEY` should work without modification.

## Known Limitations

- Single-writer token model — do not scale above one replica without external token coordination.
- Prompt caching requires a minimum token threshold per model (2,048 for Sonnet 4.6, 4,096 for Opus 4.6 / Haiku 4.5). Short prompts will not trigger caching.
- This project uses a Claude OAuth flow that works in practice but is not publicly positioned by Anthropic as a supported third-party integration surface. Treat it as an experimental compatibility path.

## Full Changelog

- https://github.com/BonzTM/claude-oauth-proxy/commits/v1.0.0
