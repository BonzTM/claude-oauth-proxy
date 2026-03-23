# Configuration

`claude-oauth-proxy` uses environment variables plus command flags.

Defaults are designed for local development, with the proxy listening on `:9999` and storing tokens at `~/.config/claude-oauth-proxy/tokens.json`.

Most users only need to change three things:

- the local API key clients will use
- the token file path when mounting runtime state into Docker Compose or Helm-managed storage
- optional port overrides when they do not want `:9999`

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDE_OAUTH_PROXY_LISTEN_ADDR` | `:9999` | HTTP listen address |
| `CLAUDE_OAUTH_PROXY_API_KEY` | `sk-proxy-local-key` | Local API key accepted by the proxy |
| `CLAUDE_OAUTH_PROXY_TOKEN_FILE` | `~/.config/claude-oauth-proxy/tokens.json` | Token persistence path |
| `CLAUDE_OAUTH_PROXY_ANTHROPIC_BASE_URL` | `https://api.anthropic.com` | Upstream Anthropic API base URL |
| `CLAUDE_OAUTH_PROXY_OAUTH_AUTH_URL` | `https://claude.ai/oauth/authorize` | Browser auth endpoint |
| `CLAUDE_OAUTH_PROXY_OAUTH_TOKEN_URL` | `https://platform.claude.com/v1/oauth/token` | Token exchange endpoint |
| `CLAUDE_OAUTH_PROXY_OAUTH_CLIENT_ID` | upstream-compatible default | OAuth client id |
| `CLAUDE_OAUTH_PROXY_OAUTH_SCOPES` | upstream-compatible default | OAuth scopes string |
| `CLAUDE_OAUTH_PROXY_OAUTH_REDIRECT_URI` | `https://platform.claude.com/oauth/code/callback` | Redirect URI used during login |
| `CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA` | `oauth-2025-04-20,prompt-caching-2024-07-31` | Anthropic beta header; prompt caching is now GA but header is kept for compatibility |
| `CLAUDE_OAUTH_PROXY_LOG_LEVEL` | unset -> `info` | Structured log level |
| `CLAUDE_OAUTH_PROXY_LOG_SINK` | unset -> `stderr` | Log output sink: `stderr`, `stdout`, or `discard` |
| `CLAUDE_OAUTH_PROXY_REQUEST_TIMEOUT` | `10m` | Per-request timeout for upstream Anthropic calls |
| `CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL` | `1m` | How often the background goroutine checks token freshness |
| `CLAUDE_OAUTH_PROXY_REFRESH_SKEW` | `5m` | Refresh token this long before it actually expires |
| `CLAUDE_OAUTH_PROXY_SEED_FILE` | unset | Read-only seed token file (e.g. Claude CLI credentials) |
| `CLAUDE_OAUTH_PROXY_CC_VERSION` | `2.1.81` | Claude Code version for billing header and User-Agent |
| `CLAUDE_OAUTH_PROXY_CC_USER_AGENT` | `claude-cli/2.1.81 (external, cli)` | User-Agent header sent upstream |
| `CLAUDE_OAUTH_PROXY_CC_SDK_VERSION` | `0.74.0` | JS SDK version for X-Stainless-Package-Version header |
| `CLAUDE_OAUTH_PROXY_CC_RUNTIME_VERSION` | `v25.8.1` | Node.js version for X-Stainless-Runtime-Version header |
| `CLAUDE_OAUTH_PROXY_CC_OS` | `Linux` | OS identifier for X-Stainless-OS header |
| `CLAUDE_OAUTH_PROXY_CC_ARCH` | `x64` | Architecture for X-Stainless-Arch header |

Use `scripts/extract-cc-fingerprint.sh` to derive current values from your installed Claude Code. See `docs/maintainers/FINGERPRINT.md` for details on each value.

## Flags

### `serve`

```bash
claude-oauth-proxy serve \
  --listen-addr :9999 \
  --api-key sk-proxy-local-key \
  --relogin \
  --no-browser \
  --code abc123
```

- `--listen-addr`: override the HTTP bind address
- `--api-key`: override the local API key required by clients
- `--relogin`: force a new browser OAuth flow even if tokens already exist
- `--no-browser`: print the auth URL instead of launching a browser
- `--code`: skip the paste prompt and provide the OAuth code directly

### `login`

```bash
claude-oauth-proxy login --no-browser --code abc123
```

- `--no-browser`: print the OAuth URL instead of launching a browser
- `--code`: provide the redirect code directly

### `status`

Shows whether a token file exists and whether it is expired.

### `logout`

Deletes the token file.

### `config validate`

Checks that required configuration values are present.

## Token File

Default location:

```text
~/.config/claude-oauth-proxy/tokens.json
```

The file is written with `0600` permissions and contains:

- access token
- refresh token
- token type
- scope
- expiry time

## Seed File

When `CLAUDE_OAUTH_PROXY_SEED_FILE` is set, the proxy reads initial credentials from that file but writes refreshed tokens to `CLAUDE_OAUTH_PROXY_TOKEN_FILE`. This is useful for mounting a read-only credential file (such as the Claude CLI's `~/.claude/.credentials.json`) without the proxy overwriting it.

The token file format from the Claude CLI (`{"claudeAiOauth": {"accessToken": ..., "expiresAt": <unix>}}`) is detected and converted automatically. No format conversion is needed on your part.

Load order:

1. try the primary token file (`CLAUDE_OAUTH_PROXY_TOKEN_FILE`)
2. if that file does not exist, fall back to the seed file (`CLAUDE_OAUTH_PROXY_SEED_FILE`)

Once the proxy refreshes tokens, they are saved to the primary token file and the seed is not read again until the primary is removed.

## Practical Examples

Use a custom local key and port:

```bash
export CLAUDE_OAUTH_PROXY_LISTEN_ADDR=":18080"
export CLAUDE_OAUTH_PROXY_API_KEY="opencode-local-key"
./dist/claude-oauth-proxy serve
```

Use a custom token location:

```bash
export CLAUDE_OAUTH_PROXY_TOKEN_FILE="$PWD/.runtime/tokens.json"
./dist/claude-oauth-proxy login
```

Use manual copy/paste without auto-opening a browser:

```bash
./dist/claude-oauth-proxy login --no-browser
```

## Deployment Mapping

Docker Compose users usually set:

- `CLAUDE_OAUTH_PROXY_API_KEY`
- `CLAUDE_OAUTH_PROXY_TOKEN_FILE=/config/tokens.json`
- `CLAUDE_OAUTH_PROXY_SEED_FILE=/config/credentials.json` (optional, when reusing Claude CLI credentials)

Helm users usually set:

- `config.apiKey.value` or `config.apiKey.existingSecret.name`
- `persistence.existingClaim` or the chart-managed PVC settings
- `config.extraEnv` for any non-default environment variables

When using the Helm chart, `config.extraEnv` takes precedence over the chart's built-in environment variables. That means you can override values such as:

- `CLAUDE_OAUTH_PROXY_LISTEN_ADDR`
- `CLAUDE_OAUTH_PROXY_TOKEN_FILE`
- `CLAUDE_OAUTH_PROXY_API_KEY`

`config.extraEnvFrom` is additive only. Use `config.extraEnv` when you want a user-supplied value to replace a chart default.

Example Helm values:

```yaml
config:
  extraEnv:
    CLAUDE_OAUTH_PROXY_API_KEY: my-custom-local-key
    CLAUDE_OAUTH_PROXY_TOKEN_FILE: /data/custom/tokens.json
```

Equivalent Helm command:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set-string config.extraEnv.CLAUDE_OAUTH_PROXY_API_KEY=my-custom-local-key \
  --set-string config.extraEnv.CLAUDE_OAUTH_PROXY_TOKEN_FILE=/data/custom/tokens.json
```

That override path is especially useful when:

- you want the API key value to come from your own env-management process instead of the chart-managed Secret
- you want the runtime token file to live at a different mounted path than the chart default

## Billing Header

The proxy injects a billing metadata string as the first system block on every request. This is set internally and not configurable via environment variable. The default value identifies traffic as coming from a Claude CLI-compatible client.

The billing header is included in the cached system prompt prefix, so it does not add per-request cost after the first turn.

## Token Refresh

The proxy keeps tokens fresh through two mechanisms:

**Background refresh**: a goroutine ticks every `CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL` (default 1 minute) and calls `AccessToken`. If the token is within `CLAUDE_OAUTH_PROXY_REFRESH_SKEW` (default 5 minutes) of expiry, a refresh is triggered. This means tokens are typically refreshed 5 minutes before they expire.

**401 retry**: if an upstream Anthropic request returns HTTP 401, the proxy immediately retries with a forced token refresh. This handles edge cases where a token expires between the background check and the actual request. Clients see a slightly slower response but never an auth error under normal conditions.

---

**See also:** [README](../README.md) · [Prompt caching](caching.md) · [Docker Compose](deploy/docker-compose.md) · [Kubernetes / Helm](deploy/kubernetes.md) · [Testing](testing.md)
