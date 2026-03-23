# Docker Compose Deployment

If you want to run `claude-oauth-proxy` with containers, Docker Compose is the supported path in this repository.

Use the example in `examples/docker-compose/docker-compose.yml` as your starting point.

The example defaults to the stable `latest` image tag. You can switch to another channel in `.env`:

```text
CLAUDE_OAUTH_PROXY_IMAGE_TAG=latest
```

Available image channels:

- `latest` or a release number like `1.2.3` for published releases
- `nightly` for the latest build from `main`
- `develop` for the latest build from a non-`main` branch pipeline

## Quick Start

Copy the example files into your own deployment directory:

```bash
mkdir -p claude-oauth-proxy-compose
cp examples/docker-compose/docker-compose.yml claude-oauth-proxy-compose/
cp examples/docker-compose/.env.example claude-oauth-proxy-compose/.env
cd claude-oauth-proxy-compose
mkdir -p runtime
```

Run a one-time headless login inside the container:

```bash
docker compose run --rm claude-oauth-proxy login --no-browser
```

That command prints a Claude login URL, prompts for the pasted `?code=...` value, and writes `tokens.json` into the mounted `./runtime` directory.

Start the proxy:

```bash
docker compose up -d
```

The proxy then listens on:

```text
http://127.0.0.1:9999
```

## If You Already Authenticated On Your Host

If you already have a working host login at:

```text
~/.config/claude-oauth-proxy/tokens.json
```

you can mount that existing directory instead of bootstrapping inside Compose.

Replace the example volume with:

```yaml
volumes:
  - ${HOME}/.config/claude-oauth-proxy:/config
```

The container then reuses your existing Claude session and skips the login flow on startup.

## Reusing Claude CLI Credentials

If you use the Claude CLI (`claude`) and have already authenticated, you can reuse those credentials directly. The proxy understands the Claude CLI's credential format (`~/.claude/.credentials.json`) and converts it automatically.

Mount the file read-only as a seed, and let the proxy write refreshed tokens to a separate volume:

```yaml
services:
  claude-oauth-proxy:
    image: ghcr.io/bonztm/claude-oauth-proxy:${CLAUDE_OAUTH_PROXY_IMAGE_TAG:-latest}
    container_name: claude-oauth-proxy
    restart: unless-stopped
    command:
      - serve
    ports:
      - "9999:9999"
    environment:
      CLAUDE_OAUTH_PROXY_API_KEY: ${CLAUDE_OAUTH_PROXY_API_KEY:-sk-proxy-local-key}
      CLAUDE_OAUTH_PROXY_SEED_FILE: /config/credentials.json
    volumes:
      - ~/.claude/.credentials.json:/config/credentials.json:ro
      - claude-oauth-data:/var/lib/claude-oauth-proxy

volumes:
  claude-oauth-data:
```

How this works:

- on first start, the proxy reads your Claude CLI credentials from the read-only seed file
- after the first token refresh, refreshed tokens are saved to `/var/lib/claude-oauth-proxy/tokens.json` on the named volume
- your host `~/.claude/.credentials.json` is never modified
- refreshed tokens persist across container restarts via the named volume
- if the volume is removed, the proxy falls back to the seed again

## API Key And Client Settings

The example Compose setup expects a local API key. By default the example uses:

```text
sk-proxy-local-key
```

If you want to pin a specific release instead of following `latest`, set for example:

```text
CLAUDE_OAUTH_PROXY_IMAGE_TAG=1.2.3
```

Point OpenAI-compatible clients at:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
```

## Health Checks

Useful checks after startup:

```bash
curl -sf http://127.0.0.1:9999/healthz
curl -sf http://127.0.0.1:9999/readyz
```

## Important Notes

- keep the mounted token directory writable so refreshes can be saved
- do not bake live OAuth tokens into the image
- treat the mounted runtime directory as sensitive data

---

**See also:** [README](../../README.md) · [Kubernetes / Helm deployment](kubernetes.md) · [Configuration reference](../configuration.md) · [Prompt caching](../caching.md)
