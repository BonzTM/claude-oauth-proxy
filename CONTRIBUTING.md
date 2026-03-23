# Contributing to claude-oauth-proxy

Thank you for your interest in contributing to claude-oauth-proxy.

## Getting Started

1. Fork the repository and clone your fork.
2. Install Go 1.24+.
3. Run `go test ./...` to verify everything builds and passes.
4. Create a branch for your change.

## Development Workflow

```bash
# Build the binary
go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy

# Run tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Run a specific package's tests
go test ./internal/provider/anthropic/...
```

## Branch Strategy

All development happens on the `main` branch:

- **All PRs should target `main`**
- Every push to `main` triggers a nightly Docker build
- Stable releases are created via version tags

## Versioning Notes

- Current development target: **`Unreleased`**
- Use `nightly` images for testing unreleased features
- Keep docs current with behavior changes (update `README.md` and `CHANGELOG.md` in the same PR when applicable)

## Pull Requests

- Keep PRs focused on a single change.
- Include tests for new behavior.
- Ensure `go test ./...` passes before submitting.
- Follow the existing code style — no linter configuration is imposed, but consistency with surrounding code is expected.
- The current coverage baseline is `90%` total statement coverage.

## What to Contribute

- Bug fixes with a failing test that demonstrates the fix.
- Documentation improvements.
- New OpenAI-compatible endpoint translations.
- Client integration examples and guides.

## Architecture Notes

- `internal/openai/types.go` defines the OpenAI-compatible request/response types. Changes here affect the proxy's API surface.
- `internal/provider/anthropic/service.go` handles translation between OpenAI and Anthropic formats, including prompt caching and tool call mapping.
- `internal/auth/` manages the OAuth token lifecycle — refresh, persistence, and the seed file fallback.
- Changes to the Anthropic SDK integration should verify cache breakpoint behavior and streaming tool call translation.

## Community

Please read the [Code of Conduct](CODE_OF_CONDUCT.md) before participating. To report security vulnerabilities, see the [Security Policy](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

**See also:** [README](README.md) · [Testing](docs/testing.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [Security Policy](SECURITY.md)
