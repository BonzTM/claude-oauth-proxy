# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in claude-oauth-proxy, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please use GitHub's private vulnerability reporting feature:

1. Go to the [Security tab](https://github.com/BonzTM/claude-oauth-proxy/security) of this repository.
2. Click "Report a vulnerability".
3. Provide a description of the vulnerability, steps to reproduce, and any potential impact.

We will acknowledge receipt within 72 hours and aim to provide a fix or mitigation plan within 30 days.

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |
| nightly | Best-effort |

## Scope

The following areas are in scope for security reports:

- OAuth token handling, storage, and refresh logic.
- API key validation and authentication bypass.
- Credential exposure through logs, error messages, or environment variable handling.
- Input validation vulnerabilities in the HTTP adapter layer.
- Token file permission or path traversal issues.

## Out of Scope

- Denial of service through large but valid requests (the proxy is a local/single-tenant tool).
- Vulnerabilities in upstream dependencies — please report those to the relevant projects directly.
- The upstream Claude OAuth flow itself — this is Anthropic's responsibility.
