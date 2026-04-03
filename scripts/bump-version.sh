#!/usr/bin/env bash
#
# Bump the proxy version and optionally update Claude Code client fingerprint
# defaults across all proxy defaults, docs, changelog, and release notes.
#
# Usage:
#   ./scripts/bump-cc-version.sh <proxy-version> --cc-version <new-cc-version>
#
# Examples:
#   ./scripts/bump-cc-version.sh 1.1.8 --cc-version 2.1.92
#
# The --cc-version flag is required and sets the Claude Code client version
# embedded in proxy defaults, documentation, and release notes.
#
# What it updates:
#   - internal/runtime/config.go     (DefaultCCVersion, DefaultUserAgent)
#   - docs/configuration.md          (env var defaults table)
#   - docs/maintainers/FINGERPRINT.md (all version references)
#   - scripts/extract-cc-fingerprint.sh (comment example)
#   - CHANGELOG.md                   (new entry + footer links)
#   - docs/release-notes/RELEASE_NOTES_<proxy-version>.md (new file)
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# --- Parse args -----------------------------------------------------------

NEW_CC=""
NEW_PROXY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --cc-version)
            NEW_CC="$2"
            shift 2
            ;;
        -h|--help)
            sed -n '2,/^$/{ s/^# //; s/^#$//; p }' "$0"
            exit 0
            ;;
        *)
            if [[ -z "$NEW_PROXY" ]]; then
                NEW_PROXY="$1"
            else
                echo "Unknown argument: $1" >&2
                exit 1
            fi
            shift
            ;;
    esac
done

if [[ -z "$NEW_PROXY" ]]; then
    echo "Usage: $0 <proxy-version> --cc-version <version>" >&2
    exit 1
fi

if [[ -z "$NEW_CC" ]]; then
    echo "Error: --cc-version is required" >&2
    echo "Usage: $0 <proxy-version> --cc-version <version>" >&2
    exit 1
fi

# Validate semver-ish format
if ! [[ "$NEW_PROXY" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Proxy version must be in X.Y.Z format, got: $NEW_PROXY" >&2
    exit 1
fi

if ! [[ "$NEW_CC" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: CC version must be in X.Y.Z format, got: $NEW_CC" >&2
    exit 1
fi

# --- Detect current versions ----------------------------------------------

OLD_CC="$(grep -oP 'DefaultCCVersion\s+=\s+"\K[0-9.]+' "$REPO_ROOT/internal/runtime/config.go")"
if [[ -z "$OLD_CC" ]]; then
    echo "Error: Could not detect current CC version from config.go" >&2
    exit 1
fi

if [[ "$OLD_CC" == "$NEW_CC" ]]; then
    echo "Error: Current version is already $OLD_CC — nothing to do." >&2
    exit 1
fi

# Find the latest proxy version from CHANGELOG.md (first ## [x.y.z] line after [Unreleased])
OLD_PROXY="$(grep -oP '## \[\K[0-9]+\.[0-9]+\.[0-9]+' "$REPO_ROOT/CHANGELOG.md" | head -1)"
if [[ -z "$OLD_PROXY" ]]; then
    echo "Error: Could not detect current proxy version from CHANGELOG.md" >&2
    exit 1
fi

if [[ "$OLD_PROXY" == "$NEW_PROXY" ]]; then
    echo "Error: Proxy version is already $OLD_PROXY — nothing to do." >&2
    exit 1
fi

TODAY="$(date -u +%Y-%m-%d)"

echo "Claude Code version: $OLD_CC → $NEW_CC"
echo "Proxy version:       $OLD_PROXY → $NEW_PROXY"
echo "Date:                $TODAY"
echo ""

# --- Update source files ---------------------------------------------------

echo "Updating internal/runtime/config.go..."
sed -i \
    -e "s|DefaultCCVersion       = \"${OLD_CC}\"|DefaultCCVersion       = \"${NEW_CC}\"|" \
    -e "s|DefaultUserAgent       = \"claude-cli/${OLD_CC} (external, cli)\"|DefaultUserAgent       = \"claude-cli/${NEW_CC} (external, cli)\"|" \
    "$REPO_ROOT/internal/runtime/config.go"

echo "Updating docs/configuration.md..."
sed -i \
    -e "s|${OLD_CC}|${NEW_CC}|g" \
    "$REPO_ROOT/docs/configuration.md"

echo "Updating docs/maintainers/FINGERPRINT.md..."
sed -i \
    -e "s|${OLD_CC}|${NEW_CC}|g" \
    "$REPO_ROOT/docs/maintainers/FINGERPRINT.md"

echo "Updating scripts/extract-cc-fingerprint.sh..."
sed -i \
    -e "s|${OLD_CC}|${NEW_CC}|g" \
    "$REPO_ROOT/scripts/extract-cc-fingerprint.sh"

# --- Update CHANGELOG.md ---------------------------------------------------

echo "Updating CHANGELOG.md..."

# Insert new version entry after [Unreleased]
CHANGELOG="$REPO_ROOT/CHANGELOG.md"
CHANGELOG_ENTRY="## [${NEW_PROXY}] - ${TODAY}

### Changed

- Updated Claude Code client fingerprint defaults from \`${OLD_CC}\` to \`${NEW_CC}\` across proxy defaults, documentation, and extraction script.

See [docs/release-notes/RELEASE_NOTES_${NEW_PROXY}.md](docs/release-notes/RELEASE_NOTES_${NEW_PROXY}.md) for the full release notes.
"

# Use awk to insert the entry after the [Unreleased] line (and its blank line)
awk -v entry="$CHANGELOG_ENTRY" '
    /^## \[Unreleased\]/ {
        print
        getline  # consume the blank line after [Unreleased]
        print ""
        printf "%s\n", entry
        next
    }
    { print }
' "$CHANGELOG" > "${CHANGELOG}.tmp" && mv "${CHANGELOG}.tmp" "$CHANGELOG"

# Update footer links
sed -i \
    -e "s|\[Unreleased\]: https://github.com/BonzTM/claude-oauth-proxy/compare/${OLD_PROXY}\.\.\.HEAD|[Unreleased]: https://github.com/BonzTM/claude-oauth-proxy/compare/${NEW_PROXY}...HEAD\n[${NEW_PROXY}]: https://github.com/BonzTM/claude-oauth-proxy/compare/${OLD_PROXY}...${NEW_PROXY}|" \
    "$CHANGELOG"

# --- Create release notes --------------------------------------------------

RELEASE_NOTES="$REPO_ROOT/docs/release-notes/RELEASE_NOTES_${NEW_PROXY}.md"
echo "Creating ${RELEASE_NOTES}..."

cat > "$RELEASE_NOTES" << EOF
# [${NEW_PROXY}] Release Notes - ${TODAY}

## Release Summary

Fingerprint maintenance release. Updates the Claude Code client version defaults from \`${OLD_CC}\` to \`${NEW_CC}\`.

## Changed

- **Claude Code version bump** — \`DefaultCCVersion\` updated from \`${OLD_CC}\` to \`${NEW_CC}\` in \`internal/runtime/config.go\`.
- **User-Agent default** — \`DefaultUserAgent\` updated from \`claude-cli/${OLD_CC} (external, cli)\` to \`claude-cli/${NEW_CC} (external, cli)\`.
- **Documentation** — Configuration reference (\`docs/configuration.md\`) and fingerprint maintenance guide (\`docs/maintainers/FINGERPRINT.md\`) updated to reflect the new version.
- **Extraction script** — Comment example in \`scripts/extract-cc-fingerprint.sh\` updated to \`${NEW_CC}\`.

## Fixed

- None.

## Added

- None.

## Admin/Operations

- None beyond the fingerprint update listed above.

## Deployment and Distribution

- Docker image: \`ghcr.io/bonztm/claude-oauth-proxy\`
- Helm chart repository: \`https://bonztm.github.io/claude-oauth-proxy\`
- Helm chart name: \`claude-oauth-proxy\`
- Helm chart reference: \`claude-oauth-proxy/claude-oauth-proxy\`
- Go build: \`go build -o dist/claude-oauth-proxy ./cmd/claude-oauth-proxy\`

\`\`\`bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy --version ${NEW_PROXY}
\`\`\`

Alternatively, set the new version via environment variables on an existing binary without rebuilding:

\`\`\`bash
export CLAUDE_OAUTH_PROXY_CC_VERSION=${NEW_CC}
export CLAUDE_OAUTH_PROXY_CC_USER_AGENT="claude-cli/${NEW_CC} (external, cli)"
\`\`\`

## Breaking Changes

None. All changes are default-value updates and fully backwards-compatible.

## Known Issues

- \`softprops/action-gh-release@v2\` and \`peaceiris/actions-gh-pages@v4\` remain on Node 20 as no upstream Node 24 release is available yet. These will be updated when new major versions are published.
- Other known issues from ${OLD_PROXY} remain unchanged; this patch does not add new user-facing behavior.

## Compatibility and Migration

- No configuration changes or migration steps are required for this release.
- Existing ${OLD_PROXY} deployments can upgrade directly to ${NEW_PROXY}.
- Users who already override \`CLAUDE_OAUTH_PROXY_CC_VERSION\` and \`CLAUDE_OAUTH_PROXY_CC_USER_AGENT\` via environment variables are unaffected by this change.

## Full Changelog

- Compare changes: https://github.com/BonzTM/claude-oauth-proxy/compare/${OLD_PROXY}...${NEW_PROXY}
- Full changelog: https://github.com/BonzTM/claude-oauth-proxy/commits/${NEW_PROXY}
EOF

# --- Summary ---------------------------------------------------------------

echo ""
echo "Done. Files updated:"
echo "  - internal/runtime/config.go"
echo "  - docs/configuration.md"
echo "  - docs/maintainers/FINGERPRINT.md"
echo "  - scripts/extract-cc-fingerprint.sh"
echo "  - CHANGELOG.md"
echo "  - docs/release-notes/RELEASE_NOTES_${NEW_PROXY}.md (new)"
echo ""
echo "Next steps:"
echo "  1. Review the changes: git diff"
echo "  2. Commit: git add -A && git commit -m 'bump claude code to ${NEW_CC}, release ${NEW_PROXY}'"
echo "  3. Tag: git tag ${NEW_PROXY}"
