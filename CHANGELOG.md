# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Fixed

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

[Unreleased]: https://github.com/BonzTM/claude-oauth-proxy/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/BonzTM/claude-oauth-proxy/releases/tag/v1.0.0
