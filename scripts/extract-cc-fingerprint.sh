#!/usr/bin/env bash
#
# Extract Claude Code fingerprint values for claude-oauth-proxy.
#
# Usage:
#   ./scripts/extract-cc-fingerprint.sh
#   ./scripts/extract-cc-fingerprint.sh /path/to/cli.js
#
# Outputs environment variable export lines that can be pasted into .env
# or used directly with the proxy.
#
# How each value is derived:
#
#   CC_VERSION
#     The bundled VERSION constant in cli.js. Found via:
#       grep -oP 'VERSION:"[^"]+"' cli.js | head -1
#     This is the Claude Code product version (e.g. 2.1.87).
#
#   CC_USER_AGENT
#     Built by pE() in cli.js as: claude-cli/${VERSION} (external, cli)
#     The "external" qualifier and "cli" entrypoint match the default
#     CLAUDE_CODE_ENTRYPOINT env var fallback in production.
#
#   CC_SDK_VERSION
#     The @anthropic-ai/sdk package version bundled into cli.js.
#     The JS SDK stores it in a variable used for X-Stainless-Package-Version.
#     Found via: grep -oP ',tn="[^"]+"' cli.js  (minified variable name may change)
#
#   CC_RUNTIME_VERSION
#     The Node.js version on the machine running the proxy. Claude Code sends
#     process.version which is the Node runtime version. We use: node --version
#
#   CC_OS / CC_ARCH
#     Claude Code sends process.platform / process.arch normalized:
#       Linux/Darwin/Windows for OS; x64/arm64 for arch.
#     We detect from the current machine with uname.
#
set -euo pipefail

CLI_JS="${1:-/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js}"

if [ ! -f "${CLI_JS}" ]; then
    # Try homebrew path
    CLI_JS="$(dirname "$(which claude 2>/dev/null || true)")/../lib/node_modules/@anthropic-ai/claude-code/cli.js"
fi

if [ ! -f "${CLI_JS}" ]; then
    echo "Could not find Claude Code cli.js. Pass the path as an argument." >&2
    echo "  Usage: $0 /path/to/cli.js" >&2
    exit 1
fi

echo "# Extracting from: ${CLI_JS}" >&2

# CC_VERSION: prefer `claude --version` (installed version) over the bundled
# VERSION constant in cli.js. The billing header uses the installed version.
CC_VERSION="$(claude --version 2>/dev/null | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' || true)"
if [ -z "${CC_VERSION}" ]; then
    CC_VERSION="$(grep -oP 'VERSION:"[^"]+"' "${CLI_JS}" | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+')"
    echo "# Warning: 'claude --version' not available, using bundled VERSION from cli.js" >&2
fi

# CC_SDK_VERSION: JS SDK package version (X-Stainless-Package-Version)
# The minified JS stores this as a short variable assigned a semver string,
# referenced near "X-Stainless-Package-Version". Extract the value.
CC_SDK_VERSION="$(grep -oP 'Stainless-Package-Version":[a-zA-Z0-9_]+' "${CLI_JS}" | head -1 | grep -oP ':[a-zA-Z0-9_]+$' | tr -d ':' | xargs -I{} grep -oP "{}=\"[^\"]+\"" "${CLI_JS}" | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+')" || true
if [ -z "${CC_SDK_VERSION}" ]; then
    # Fallback: look for semver near stainless package version pattern
    CC_SDK_VERSION="$(grep -oP ',tn="[0-9]+\.[0-9]+\.[0-9]+"' "${CLI_JS}" | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+')" || true
fi

# CC_RUNTIME_VERSION: Node.js version
CC_RUNTIME_VERSION="$(node --version 2>/dev/null || echo "v22.14.0")"

# CC_OS: normalize uname to match Node.js process.platform -> Stainless OS
case "$(uname -s)" in
    Linux*)  CC_OS="Linux" ;;
    Darwin*) CC_OS="MacOS" ;;
    MINGW*|MSYS*|CYGWIN*) CC_OS="Windows" ;;
    *)       CC_OS="Other:$(uname -s)" ;;
esac

# CC_ARCH: normalize uname -m to match Node.js process.arch -> Stainless arch
case "$(uname -m)" in
    x86_64)  CC_ARCH="x64" ;;
    aarch64|arm64) CC_ARCH="arm64" ;;
    *)       CC_ARCH="other:$(uname -m)" ;;
esac

CC_USER_AGENT="claude-cli/${CC_VERSION} (external, cli)"

echo ""
echo "# Claude Code fingerprint values (extracted $(date -u +%Y-%m-%d))"
echo "# Source: ${CLI_JS}"
echo "# Claude Code version: ${CC_VERSION}"
echo "#"
echo "# Paste into .env or export in your shell before starting the proxy."
echo "# Only needed if the defaults in the proxy binary are outdated."
echo ""
echo "CLAUDE_OAUTH_PROXY_CC_VERSION=${CC_VERSION}"
echo "CLAUDE_OAUTH_PROXY_CC_USER_AGENT=${CC_USER_AGENT}"
echo "CLAUDE_OAUTH_PROXY_CC_SDK_VERSION=${CC_SDK_VERSION}"
echo "CLAUDE_OAUTH_PROXY_CC_RUNTIME_VERSION=${CC_RUNTIME_VERSION}"
echo "CLAUDE_OAUTH_PROXY_CC_OS=${CC_OS}"
echo "CLAUDE_OAUTH_PROXY_CC_ARCH=${CC_ARCH}"
