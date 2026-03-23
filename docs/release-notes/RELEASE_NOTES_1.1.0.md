# [1.1.0] Release Notes - 2026-03-23

## Release Summary

This release adds extended thinking (reasoning) support, fixes cache and tool call token reporting for OpenAI-compatible clients, matches the Claude Code client fingerprint for seamless upstream compatibility, and ships maintainer tooling for keeping the fingerprint current.

## Highlights

- **Extended thinking / reasoning** — New `reasoning_effort` field (`low`/`medium`/`high`) maps to Anthropic's extended thinking with appropriate budget tokens. Thinking tokens are estimated and reported as `completion_tokens_details.reasoning_tokens` in usage responses.
- **Cache token display fixed** — `prompt_tokens` now correctly includes all input tokens (cached + non-cached), and `prompt_tokens_details.cached_tokens` follows the OpenAI format so clients like opencode display cache metrics correctly.
- **Client fingerprint matching** — Upstream requests now match Claude Code's User-Agent, Stainless headers, and billing header format. Configurable via 6 new environment variables so the fingerprint stays current without rebuilding.
- **Fingerprint extraction script** — `scripts/extract-cc-fingerprint.sh` derives all fingerprint values from an installed Claude Code and outputs ready-to-paste env vars.

## Added

- `reasoning_effort` request field (`low`/`medium`/`high`) enabling Anthropic extended thinking with budget tokens of 1024/8192/32768.
- `completion_tokens_details.reasoning_tokens` in usage responses (both streaming and non-streaming), estimated from thinking block content.
- `prompt_tokens_details.cached_tokens` in usage responses for OpenAI-format cache metric display.
- `index` field on streaming tool call chunks for OpenAI spec compliance.
- Client fingerprint matching: `User-Agent`, `X-Stainless-*`, `x-app` headers, and per-turn billing header suffix.
- 6 new environment variables for fingerprint overrides (`CC_VERSION`, `CC_USER_AGENT`, `CC_SDK_VERSION`, `CC_RUNTIME_VERSION`, `CC_OS`, `CC_ARCH`).
- `scripts/extract-cc-fingerprint.sh` for deriving fingerprint values from installed Claude Code.
- `docs/maintainers/FINGERPRINT.md` explaining each fingerprint value and update procedures.
- Prompt caching documentation (`docs/caching.md`) with cache breakpoint strategy, metrics, and minimum token thresholds.
- Client integration examples in README for opencode, aider, Continue, and OpenAI Python SDK.
- CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md.
- CHANGELOG.md with Keep a Changelog format.
- GitHub issue templates (bug report, feature request), PR template, and discussions link.
- Release notes template at `docs/maintainers/RELEASE_NOTES_TEMPLATE.md`.
- Cross-navigation links across all documentation pages.

## Fixed

- `prompt_tokens` now includes all input tokens (cached + non-cached) instead of only the non-cached portion from Anthropic's `input_tokens`. This fixes negative token displays in clients that compute `prompt_tokens - cached_tokens`.
- Streaming tool call chunks now include the required `index` field, fixing validation errors in opencode and other strict OpenAI-compatible clients.
- Helm chart release workflow no longer requires Chart.yaml version to match the release tag at commit time. Chart version is stamped at release time from the tag, matching the soundspan pattern.
- Helm chart release now waits for the container image to be published in GHCR before packaging.

## Changed

- Billing header updated from static `cc_version=2.1.77` to dynamic `cc_version={version}.{turn}` format matching Claude Code's per-call suffix.
- Updated model references from `claude-sonnet-4-5` to `claude-sonnet-4-6` across documentation.
- Fixed `CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA` default in docs to match actual value (`oauth-2025-04-20,prompt-caching-2024-07-31`).
- Refresh interval/skew descriptions in configuration docs updated from "Reserved" to actual behavior.
- README "Where To Go Next" section now uses proper markdown links and includes community docs.

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version 1.1.0
```

## Breaking Changes

None. All changes are backwards-compatible. The fingerprint headers and reasoning support are additive.

## Known Issues

- Reasoning token counts are estimated from thinking block character length (~4 chars/token). Anthropic does not expose a separate thinking token count in usage, so the estimate may differ from actual billing.
- Extended thinking requires `temperature` to be 1 (the default). If a client sets a custom temperature alongside `reasoning_effort`, the proxy overrides it to 1.

## Compatibility and Migration

- No configuration changes required for existing deployments.
- Fingerprint env vars are optional — defaults match Claude Code 2.1.81.
- Clients already sending `reasoning_effort` (e.g. opencode with `"reasoning": true`) will now have thinking enabled automatically.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.0.0...v1.1.0
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/v1.1.0
