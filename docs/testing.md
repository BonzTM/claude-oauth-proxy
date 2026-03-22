# Testing

The project is built around a Go-first test loop.

## Main Commands

Run the full suite:

```bash
go test ./...
```

Run with coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Run a package subset:

```bash
go test ./internal/auth ./internal/provider/anthropic ./internal/adapters/http
```

Validate the Helm chart:

```bash
helm lint charts/claude-oauth-proxy
helm template claude-oauth-proxy charts/claude-oauth-proxy >/dev/null
```

Validate the Docker Compose example:

```bash
docker compose -f examples/docker-compose/docker-compose.yml config >/dev/null
```

## Coverage Baseline

The current repository baseline is `90%` total statement coverage.

Example verification command:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## What Is Covered

Current tests focus on:

- CLI routing and login behavior
- config parsing and validation
- structured logging helpers and recorder behavior
- token file persistence and expiry logic
- manual OAuth prepare/exchange/refresh flow
- Anthropic provider mapping and retry behavior
- OpenAI-compatible HTTP endpoints and error handling
- Helm chart lint and render checks in CI
- Docker Compose example validation in CI

## Useful Local Checks

Validate config:

```bash
go test ./internal/runtime
./dist/claude-oauth-proxy config validate
```

Run the binary manually after building:

```bash
go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy
./dist/claude-oauth-proxy version
./dist/claude-oauth-proxy status
```

## Streaming Smoke Test

After logging in and starting `serve`, you can do a local streaming check:

```bash
curl http://127.0.0.1:9999/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-local-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Reply with two short words."}
    ]
  }'
```
