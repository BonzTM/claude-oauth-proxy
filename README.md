# claude-oauth-proxy

`claude-oauth-proxy` is a Go CLI and HTTP proxy that exposes a small OpenAI-compatible API backed by Claude OAuth tokens.

It is built for product-style use where you want:

- a terminal-facing CLI
- a proxy on `:9999` by default
- manual Claude OAuth login in the terminal
- token persistence at `~/.config/claude-oauth-proxy/tokens.json`
- automatic token refresh while the proxy is running
- a small OpenAI-compatible surface for clients such as OpenCode
- a published container image
- Docker Compose examples for self-hosted container users
- a Helm chart for Kubernetes users

Important: the OAuth flow used here is based on the same manual browser flow as the upstream project you referenced, but Anthropic does not publicly document third-party Claude Pro/Max OAuth as a supported integration surface. Treat this as an experimental compatibility path that you operate at your own risk.

## Features

- Go binary with `serve`, `login`, `status`, `logout`, `config validate`, and `version`
- manual browser login flow against `claude.ai/oauth/authorize`
- pasted `?code=...` terminal flow
- saved tokens in `~/.config/claude-oauth-proxy/tokens.json`
- token reuse across runs unless `--relogin` is used
- automatic refresh during proxy runtime
- OpenAI-compatible endpoints:
  - `GET /v1/models`
  - `POST /v1/chat/completions`
- health endpoints:
  - `GET /health`
  - `GET /healthz`
  - `GET /livez`
  - `GET /ready`
  - `GET /readyz`
- structured JSON logging
- Go tests currently meeting the repo's `90%` baseline threshold

## Quick Start

Build the binary:

```bash
go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy
```

Run the manual login flow:

```bash
./dist/claude-oauth-proxy login
```

What happens:

1. the command opens your browser to `https://claude.ai/oauth/authorize`
2. you log in with your Claude account
3. Claude redirects to a URL containing `?code=...`
4. you copy the `code` value and paste it into the terminal
5. the app exchanges the code for tokens and writes `~/.config/claude-oauth-proxy/tokens.json`

Start the proxy:

```bash
./dist/claude-oauth-proxy serve
```

By default it listens on `http://127.0.0.1:9999` and expects the local API key `sk-proxy-local-key`.

## Use With OpenAI-Compatible Clients

Set your client to point at the local proxy:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
```

Example `curl` request:

```bash
curl http://127.0.0.1:9999/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-local-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [
      {"role": "user", "content": "Say hello in one short sentence."}
    ]
  }'
```

Example model listing:

```bash
curl http://127.0.0.1:9999/v1/models \
  -H "Authorization: Bearer sk-proxy-local-key"
```

## Subsequent Runs

Once tokens exist on disk:

- `serve` loads them automatically
- the browser login flow is skipped
- refresh is attempted automatically when the token is near expiry or an upstream auth retry is needed

Force a fresh login with:

```bash
./dist/claude-oauth-proxy serve --relogin
```

or:

```bash
./dist/claude-oauth-proxy login
```

## CLI

```bash
claude-oauth-proxy serve [--listen-addr :9999] [--api-key sk-proxy-local-key] [--relogin] [--no-browser] [--code <code>]
claude-oauth-proxy login [--no-browser] [--code <code>]
claude-oauth-proxy status
claude-oauth-proxy logout
claude-oauth-proxy config validate
claude-oauth-proxy version
```

Examples:

```bash
./dist/claude-oauth-proxy login --no-browser
./dist/claude-oauth-proxy login --code "abc123"
./dist/claude-oauth-proxy serve --api-key "my-local-key"
./dist/claude-oauth-proxy serve --listen-addr ":18080" --relogin
./dist/claude-oauth-proxy status
./dist/claude-oauth-proxy logout
./dist/claude-oauth-proxy config validate
```

## Configuration

The most important settings are:

- `CLAUDE_OAUTH_PROXY_LISTEN_ADDR`
- `CLAUDE_OAUTH_PROXY_API_KEY`
- `CLAUDE_OAUTH_PROXY_TOKEN_FILE`
- `CLAUDE_OAUTH_PROXY_ANTHROPIC_BASE_URL`
- `CLAUDE_OAUTH_PROXY_OAUTH_AUTH_URL`
- `CLAUDE_OAUTH_PROXY_OAUTH_TOKEN_URL`
- `CLAUDE_OAUTH_PROXY_OAUTH_CLIENT_ID`
- `CLAUDE_OAUTH_PROXY_OAUTH_SCOPES`
- `CLAUDE_OAUTH_PROXY_OAUTH_REDIRECT_URI`
- `CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA`
- `CLAUDE_OAUTH_PROXY_LOG_LEVEL`
- `CLAUDE_OAUTH_PROXY_LOG_SINK`

See `docs/configuration.md` for the full matrix.

## Choose Your Deployment Path

- Local binary: build the Go binary and run `login` / `serve`
- Docker Compose: `docs/deploy/docker-compose.md`
- Kubernetes with Helm: `docs/deploy/kubernetes.md`

This repository ships a container image, a Helm chart, and Compose examples. It does not treat raw Kubernetes manifests as a supported deployment interface.

## Image Channels

Published images use three channels:

- `latest` and `<release-number>` for published releases
- `nightly` and `nightly-<shortsha>` for non-release builds from `main`
- `develop` and `develop-<shortsha>` for non-`main` branch builds

If you want stable behavior, use a published release number or `latest`. If you want branch-channel builds, use `nightly` or `develop` explicitly.

## Docker Compose

Use the example in `examples/docker-compose/docker-compose.yml`.

The two common patterns are:

1. authenticate once on the host and mount your existing `~/.config/claude-oauth-proxy` directory into the container
2. run a one-time headless login with `docker compose run --rm claude-oauth-proxy login --no-browser`, then keep the generated `tokens.json` in the mounted runtime directory

See `docs/deploy/docker-compose.md` for the full walkthrough.

## Kubernetes With Helm

Kubernetes users should install the Helm chart from the published Pages-backed chart repository.

After the pod is running, perform the one-time headless login bootstrap with:

```bash
kubectl exec -it -n claude-oauth-proxy deployment/claude-oauth-proxy -- \
  /usr/local/bin/claude-oauth-proxy login --no-browser
```

See `docs/deploy/kubernetes.md` for the full install flow.

## Testing

Run the test suite:

```bash
go test ./...
```

Check coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The current baseline threshold for this repo is `90%` total statement coverage.

See `docs/testing.md` for more details.
