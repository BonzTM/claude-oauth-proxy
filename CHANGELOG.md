# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Fixed

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
- Automatic prompt caching with cache breakpoints and `prompt_tokens_details.cached_tokens` in responses
- OAuth login flow with browser and headless modes
- Token persistence, background refresh, and automatic 401 retry
- Claude CLI credential reuse via seed file
- Docker Compose and Helm chart deployment paths
- CLI: `serve`, `login`, `logout`, `status`, `config validate`, `version`
- Health and readiness endpoints
- Full documentation: configuration reference, caching guide, deployment guides, testing guide
- Community docs: CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md, GitHub issue/PR templates

See [docs/release-notes/RELEASE_NOTES_1.0.0.md](docs/release-notes/RELEASE_NOTES_1.0.0.md) for the full release notes.

[Unreleased]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/BonzTM/claude-oauth-proxy/releases/tag/v1.0.0
