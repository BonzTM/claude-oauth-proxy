# Client Fingerprint Maintenance

The proxy matches Claude Code's request fingerprint so that upstream requests appear indistinguishable from a standard Claude Code CLI session. This document explains each value, where it comes from, and how to update them.

## Quick Update

Run the extraction script after upgrading Claude Code:

```bash
./scripts/extract-cc-fingerprint.sh
```

The script outputs environment variable export lines. Either:
- paste them into your `.env` file (takes effect on next restart, no rebuild needed), or
- update the defaults in `internal/runtime/config.go` and rebuild.

## Values and Their Sources

### `CC_VERSION` (env: `CLAUDE_OAUTH_PROXY_CC_VERSION`)

**What it is:** The Claude Code product version (e.g. `2.1.85`).

**Where it's used:**
- Billing header: `cc_version=2.1.85.{turn}`
- User-Agent: `claude-cli/2.1.85 (external, cli)`

**How to find it:**
```bash
claude --version
# Output: 2.1.85 (Claude Code)
```

### `CC_USER_AGENT` (env: `CLAUDE_OAUTH_PROXY_CC_USER_AGENT`)

**What it is:** The `User-Agent` header sent with every API request.

**Format:** `claude-cli/{CC_VERSION} (external, {entrypoint})`

**Where it comes from:** The `pE()` function in Claude Code's `cli.js`, which builds:
```
claude-cli/${VERSION} (external, ${CLAUDE_CODE_ENTRYPOINT})
```
The entrypoint defaults to `cli` for standard usage.

**How to derive:**
```bash
echo "claude-cli/$(claude --version 2>/dev/null | grep -oP '[0-9.]+') (external, cli)"
```

### `CC_SDK_VERSION` (env: `CLAUDE_OAUTH_PROXY_CC_SDK_VERSION`)

**What it is:** The `@anthropic-ai/sdk` JS package version bundled into Claude Code. Sent as the `X-Stainless-Package-Version` header.

**Where it comes from:** A minified variable in `cli.js` referenced near `X-Stainless-Package-Version`. The extraction script finds it by tracing the variable name from the header assignment.

**How to find it manually:**
```bash
# The variable name changes between builds, but this pattern is stable:
grep -oP ',tn="[0-9]+\.[0-9]+\.[0-9]+"' /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js
# Output: ,tn="0.74.0"
```

### `CC_RUNTIME_VERSION` (env: `CLAUDE_OAUTH_PROXY_CC_RUNTIME_VERSION`)

**What it is:** The Node.js version, sent as `X-Stainless-Runtime-Version`. Claude Code sends `process.version`.

**How to find it:**
```bash
node --version
# Output: v25.8.1
```

### `CC_OS` (env: `CLAUDE_OAUTH_PROXY_CC_OS`)

**What it is:** The OS identifier sent as `X-Stainless-OS`. Claude Code normalizes `process.platform` to `Linux`, `MacOS`, or `Windows`.

### `CC_ARCH` (env: `CLAUDE_OAUTH_PROXY_CC_ARCH`)

**What it is:** The architecture sent as `X-Stainless-Arch`. Claude Code normalizes `process.arch` to `x64` or `arm64`.

## Other Headers Set by the Proxy

| Header | Value | Notes |
|--------|-------|-------|
| `x-app` | `cli` | Static; identifies the client application type |
| `X-Stainless-Lang` | `js` | Static; the real SDK is JS, not Go |
| `X-Stainless-Runtime` | `node` | Static; matches the JS SDK runtime |
| `anthropic-beta` | `oauth-2025-04-20,...` | Configurable via `CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA` |

## Billing Header Format

The billing header is injected as the first system block on every request:

```
x-anthropic-billing-header: cc_version=2.1.85.{turn}; cc_entrypoint=cli; cch=00000;
```

- `cc_version`: `{CC_VERSION}.{turn_number}` — Claude Code appends a per-call suffix
- `cc_entrypoint`: always `cli` for standard usage
- `cch`: hardcoded to `00000` in Claude Code itself (not computed)

The turn number increments with each request to the proxy, matching Claude Code's behavior of appending a call counter.

## When to Update

Update fingerprint values when:
- Claude Code releases a new version (`claude --version` changes)
- The bundled `@anthropic-ai/sdk` version changes (check with extraction script)
- You upgrade Node.js on the machine running the proxy

The env var overrides mean you never need to rebuild the proxy binary just for a fingerprint update — set the new values in your environment and restart.

---

**See also:** [Configuration reference](../configuration.md) · [README](../../README.md)
