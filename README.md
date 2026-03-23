# claude-oauth-proxy

`claude-oauth-proxy` lets you use Claude through an OpenAI-compatible API.

It is built for tools that already know how to talk to OpenAI-style endpoints, but where you want Claude behind the scenes instead.

If you are brand new here, start with Docker Compose. It is the fastest path from "clone the repo" to "my app can talk to Claude."

Important: this project uses a Claude OAuth flow that works for real users in practice, but Anthropic does not publicly position third-party Claude Pro/Max OAuth as a normal supported integration surface. Treat it as an experimental compatibility path.

## Start Here

Choose one way to run the proxy:

- Docker Compose: easiest for most users
- Local binary: good if you prefer running directly on your machine
- Helm: the supported Kubernetes path

Then the setup is always the same:

1. authenticate to Claude once
2. save the resulting token file somewhere writable
3. run the proxy
4. point your client at `http://127.0.0.1:9999/v1`

## Fastest Path: Docker Compose

Copy the example files:

```bash
mkdir -p claude-oauth-proxy-compose
cp examples/docker-compose/docker-compose.yml claude-oauth-proxy-compose/
cp examples/docker-compose/.env.example claude-oauth-proxy-compose/.env
cd claude-oauth-proxy-compose
mkdir -p runtime
```

Do a one-time Claude login inside the container:

```bash
docker compose run --rm claude-oauth-proxy login --no-browser
```

That command will:

1. print a Claude login URL
2. ask you to sign in with your Claude account
3. ask you to paste the `?code=...` value from the redirect URL
4. save `tokens.json` into `./runtime`

Start the proxy:

```bash
docker compose up -d
```

Point your client at the proxy:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
```

Quick smoke test:

```bash
curl http://127.0.0.1:9999/v1/models \
  -H "Authorization: Bearer sk-proxy-local-key"
```

More detail: `docs/deploy/docker-compose.md`

## If You Already Logged In On Your Host

If you already have a token file at:

```text
~/.config/claude-oauth-proxy/tokens.json
```

you do not need to log in again.

Mount that directory into Docker Compose or into your container runtime, and the proxy will reuse the existing Claude session.

## Reusing Claude CLI Credentials

If you already authenticated with the Claude CLI (`claude`), the proxy can use those credentials directly. Mount `~/.claude/.credentials.json` as a read-only seed file:

```yaml
environment:
  CLAUDE_OAUTH_PROXY_SEED_FILE: /config/credentials.json
volumes:
  - ~/.claude/.credentials.json:/config/credentials.json:ro
  - claude-oauth-data:/var/lib/claude-oauth-proxy
```

The proxy reads the seed on first start, then writes refreshed tokens to its built-in writable path. Your Claude CLI credentials are never modified. See `docs/deploy/docker-compose.md` for a complete example.

## Local Binary

Build the binary:

```bash
go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy
```

Authenticate:

```bash
./dist/claude-oauth-proxy login
```

Run the proxy:

```bash
./dist/claude-oauth-proxy serve
```

By default it listens on `http://127.0.0.1:9999` and uses the local API key `sk-proxy-local-key`.

## Kubernetes With Helm

Kubernetes users should use the Helm chart shipped in this repository.

Add the chart repo:

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
```

Install the chart:

```bash
kubectl create namespace claude-oauth-proxy

kubectl create secret generic claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --from-literal=api-key=sk-proxy-local-key

helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --create-namespace \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy
```

Bootstrap login inside the running pod:

```bash
kubectl exec -it -n claude-oauth-proxy deployment/claude-oauth-proxy -- \
  /usr/local/bin/claude-oauth-proxy login --no-browser
```

Then port-forward and point your client at the service.

More detail: `docs/deploy/kubernetes.md`

## What This Project Does

- exposes an OpenAI-compatible API for Claude-backed requests
- supports tool use / function calling (translated to Anthropic tool_use blocks)
- applies prompt caching automatically to reduce cost and latency (see `docs/caching.md`)
- stores OAuth tokens in a local file and reuses them across runs
- refreshes tokens automatically while the proxy is running
- retries transparently on 401 with a forced token refresh
- supports:
  - `GET /v1/models`
  - `POST /v1/chat/completions` (streaming and non-streaming)
- exposes health endpoints:
  - `GET /health`
  - `GET /healthz`
  - `GET /livez`
  - `GET /ready`
  - `GET /readyz`

## How Authentication Works

The first login is manual by design:

1. the proxy opens or prints a Claude OAuth URL
2. you sign in with your Claude account
3. you paste the returned `code`
4. the proxy exchanges that code for tokens
5. the tokens are stored on disk and reused on future runs

Default token path on a host machine:

```text
~/.config/claude-oauth-proxy/tokens.json
```

You can force a fresh login with:

```bash
claude-oauth-proxy serve --relogin
```

or:

```bash
claude-oauth-proxy login
```

## Image Tags

Published images use three channels:

- `latest` and `<release-number>` for published releases
- `nightly` and `nightly-<shortsha>` for non-release builds from `main`
- `develop` and `develop-<shortsha>` for non-`main` branch builds

If you want the safest default, use `latest` or a pinned release like `1.2.3`.

## Common Client Settings

Most OpenAI-compatible tools only need two environment variables:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
```

### opencode

Add to `~/.config/opencode/opencode.json`:

```json
{
  "provider": {
    "claude-proxy": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Claude OAuth Proxy",
      "options": {
        "baseURL": "http://127.0.0.1:9999/v1",
        "apiKey": "sk-proxy-local-key"
      },
      "models": {
        "claude-sonnet-4-6": {
          "name": "Claude Sonnet 4.6",
          "tool_call": true,
          "reasoning": true,
          "modalities": { "input": ["text", "image"], "output": ["text"] },
          "limit": { "context": 200000, "output": 16384 }
        }
      }
    }
  }
}
```

### aider

```bash
aider --openai-api-base http://127.0.0.1:9999/v1 \
      --openai-api-key sk-proxy-local-key \
      --model openai/claude-sonnet-4-6
```

Or set in environment:

```bash
export OPENAI_API_BASE="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
aider --model openai/claude-sonnet-4-6
```

### Continue (VS Code / JetBrains)

Add to `.continue/config.yaml`:

```yaml
models:
  - name: Claude via Proxy
    provider: openai
    model: claude-sonnet-4-6
    apiBase: http://127.0.0.1:9999/v1
    apiKey: sk-proxy-local-key
```

### Any OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:9999/v1",
    api_key="sk-proxy-local-key",
)

response = client.chat.completions.create(
    model="claude-sonnet-4-6",
    messages=[{"role": "user", "content": "Say hello in one short sentence."}],
)
print(response.choices[0].message.content)
```

### curl

```bash
curl http://127.0.0.1:9999/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-local-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [
      {"role": "user", "content": "Say hello in one short sentence."}
    ]
  }'
```

## CLI Commands

```bash
claude-oauth-proxy serve [--listen-addr :9999] [--api-key sk-proxy-local-key] [--relogin] [--no-browser] [--code <code>]
claude-oauth-proxy login [--no-browser] [--code <code>]
claude-oauth-proxy status
claude-oauth-proxy logout
claude-oauth-proxy config validate
claude-oauth-proxy version
```

## Where To Go Next

**Guides:**

- [Docker Compose deployment](docs/deploy/docker-compose.md)
- [Kubernetes / Helm deployment](docs/deploy/kubernetes.md)
- [Configuration reference](docs/configuration.md)
- [Prompt caching](docs/caching.md)
- [Helm chart details](charts/claude-oauth-proxy/README.md)
- [Testing and validation](docs/testing.md)

**Community:**

- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## Development

Run tests:

```bash
go test ./...
```

Check coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The current baseline threshold for this repo is `90%` total statement coverage.
