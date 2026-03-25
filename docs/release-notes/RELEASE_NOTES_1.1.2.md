# [1.1.2] Release Notes - 2026-03-25

## Release Summary

Patch release adding theoretical cost tracking. When enabled, the proxy fetches per-model pricing from the OpenRouter API at startup and attaches cost breakdowns to every response — both as structured log entries and in the response body/headers. Cache-aware pricing applies Anthropic's 1.25x write / 0.1x read multipliers.

## Added

- **Cost tracking flag** — `--cost-tracking` CLI flag on `serve` and `CLAUDE_OAUTH_PROXY_COST_TRACKING=true` env var. Disabled by default; no behavior change for existing deployments.
- **Per-request cost in response body** — When enabled, the `usage` object in both streaming and non-streaming responses includes a `cost` field:
  ```json
  {
    "usage": {
      "prompt_tokens": 1500,
      "completion_tokens": 200,
      "cost": {
        "input_cost": 0.003600,
        "output_cost": 0.003000,
        "cache_write_cost": 0.001875,
        "cache_read_cost": 0.000030,
        "total_cost": 0.008505,
        "currency": "USD",
        "model": "claude-sonnet-4-20250514",
        "input_price_per_1m": 3.0,
        "output_price_per_1m": 15.0
      }
    }
  }
  ```
- **`X-Request-Cost` response header** — Non-streaming responses include a header (e.g. `X-Request-Cost: 0.008505 USD`) for lightweight cost visibility without parsing the body.
- **Structured cost logging** — Every request with pricing data emits a `cost.tracked` log event with per-component cost fields. When pricing is unavailable for a model, a `cost.pricing_not_found` event is logged instead.
- **Cache-aware pricing** — Cost calculation applies Anthropic cache pricing rules: cache creation tokens at 1.25x the input rate, cache read tokens at 0.1x the input rate. Regular input tokens are the remainder after subtracting cache tokens.
- **OpenRouter pricing source** — Pricing is fetched from `GET https://openrouter.ai/api/v1/models` at startup and cached in memory. Model lookup tries the exact model name first, then falls back to `anthropic/{model}` (the OpenRouter naming convention).
- **`CLAUDE_OAUTH_PROXY_OPENROUTER_URL`** — Optional env var to override the OpenRouter API endpoint. Defaults to `https://openrouter.ai/api/v1/models`.

## Changed

- `X-Request-Cost` added to CORS `Access-Control-Expose-Headers` so browser-based clients can read the header.

## Architecture

The feature follows the existing decorator/middleware pattern:

- `internal/cost/cost.go` — `Calculate()` function: pure computation from usage + pricing.
- `internal/cost/openrouter.go` — `OpenRouterSource`: fetches and caches pricing, implements `PricingSource` interface.
- `internal/cost/middleware.go` — `WithCostTracking()`: wraps `provider.Service` (same pattern as `provider.WithLogging`). Intercepts non-streaming responses and wraps streaming to attach cost on the final chunk.

The middleware is wired conditionally in `service_factory.go` only when `CostTracking` is true. When disabled, zero additional overhead.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.2
```

## Breaking Changes

None. All changes are backwards-compatible.

- Cost tracking is opt-in via flag/env var. Disabled by default.
- The `usage.cost` field is `omitempty` — clients that don't expect it will not see it when cost tracking is off.
- New env vars (`COST_TRACKING`, `OPENROUTER_URL`) default to disabled/empty respectively.

## Known Issues

- Pricing data is fetched once at startup and cached for the lifetime of the process. If OpenRouter pricing changes mid-run, a restart is required to pick up new prices. A periodic refresh could be added in a future release.
- If the OpenRouter API is unreachable at startup, cost tracking is still enabled but all requests will log `cost.pricing_not_found`. The proxy continues to function normally — cost data is best-effort.
- Reasoning token counts remain estimated (~4 chars/token heuristic). Anthropic does not expose a separate thinking token count, so cost for thinking tokens is folded into the output token cost.

## Compatibility and Migration

- No configuration changes required for existing deployments.
- To enable: set `CLAUDE_OAUTH_PROXY_COST_TRACKING=true` or pass `--cost-tracking` to `serve`.
- Outbound HTTPS access to `openrouter.ai` is required at startup when cost tracking is enabled.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/1.1.1...1.1.2
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/1.1.2
