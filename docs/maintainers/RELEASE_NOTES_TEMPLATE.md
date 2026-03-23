# [{{VERSION}}] Release Notes - {{RELEASE_DATE}}

## Release Summary

{{RELEASE_SUMMARY}}

## Fixed

{{FIXED_ITEMS}}

## Added

{{ADDED_ITEMS}}

## Changed

{{CHANGED_ITEMS}}

## Admin/Operations

{{ADMIN_ITEMS}}

## Deployment and Distribution

- Docker image: `ghcr.io/bonztm/claude-oauth-proxy`
- Helm chart repository: `https://bonztm.github.io/claude-oauth-proxy`
- Helm chart name: `claude-oauth-proxy`
- Helm chart reference: `claude-oauth-proxy/claude-oauth-proxy`
- Go build: `go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy`

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version {{VERSION}}
```

## Breaking Changes

{{BREAKING_ITEMS}}

## Known Issues

{{KNOWN_ISSUES}}

## Compatibility and Migration

{{COMPATIBILITY_AND_MIGRATION}}

## Full Changelog

- Compare changes: {{COMPARE_URL}}
- Full changelog: {{FULL_CHANGELOG_URL}}
