#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

cat > "$TEMP_DIR/curl" <<'EOF'
#!/bin/bash
printf '%s\n' "$@" > "$CURL_ARGS_LOG"
env > "${CURL_ARGS_LOG}.env"
cat > "${CURL_ARGS_LOG}.stdin"
EOF
chmod +x "$TEMP_DIR/curl"

mkdir "$TEMP_DIR/home"
cat > "$TEMP_DIR/home/.curlrc" <<'EOF'
url = "https://example.com/collect"
header = "X-Leaked-From-Curlrc: yes"
EOF

# Source the helpers under test without executing the installer's startup code
# or main function. The range form works with both BSD and GNU sed.
sed -n '/^validate_release_source() {/,/^# Get latest release version/p' "$ROOT_DIR/deploy/install.sh" > "$TEMP_DIR/github-api-curl.sh"
grep -Fq 'github_api_curl() {' "$TEMP_DIR/github-api-curl.sh"
grep -Fq 'github_asset_download() {' "$TEMP_DIR/github-api-curl.sh"

run_api_curl() {
    CURL_ARGS_LOG="$1" HOME="$TEMP_DIR/home" PATH="$TEMP_DIR:$PATH" UPDATE_GITHUB_TOKEN="${2:-}" \
        GITHUB_TOKEN="github-fallback" GH_TOKEN="gh-fallback" \
        bash -c 'GITHUB_REPO=hxly520/sub2api; GITHUB_API_ROOT=https://api.github.com; GITHUB_API_VERSION=2022-11-28; REQUIRE_RELEASE_CHECKSUM=true; REQUIRE_RELEASE_MANIFEST=true; UPDATE_DOCKER_IMAGE_VALUE=ghcr.io/hxly520/sub2api; UPDATE_CHANNEL_VALUE=stable; UPDATE_IN_PLACE_ENABLED_VALUE=true; print_error() { echo "$*" >&2; }; source "$1"; github_api_curl -s "$2"' bash \
        "$TEMP_DIR/github-api-curl.sh" "https://api.github.com/repos/hxly520/sub2api/releases/latest"
}

run_api_curl "$TEMP_DIR/authenticated" "update-secret"
test "$(head -n 1 "$TEMP_DIR/authenticated")" = '-q'
grep -Fxq -- '--config' "$TEMP_DIR/authenticated"
grep -Fxq -- '-' "$TEMP_DIR/authenticated"
grep -Fxq -- '--globoff' "$TEMP_DIR/authenticated"
grep -Fxq 'header = "Authorization: Bearer update-secret"' "$TEMP_DIR/authenticated.stdin"
if grep -Fq 'update-secret' "$TEMP_DIR/authenticated"; then
    echo "installer exposed the update token in curl argv" >&2
    exit 1
fi
if grep -Eq 'update-secret|github-fallback|gh-fallback' "$TEMP_DIR/authenticated.env"; then
    echo "installer exposed a token in curl environment" >&2
    exit 1
fi
test "$(grep -Fxc 'https://api.github.com/repos/hxly520/sub2api/releases/latest' "$TEMP_DIR/authenticated")" -eq 1
if grep -Fq 'example.com/collect' "$TEMP_DIR/authenticated" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/authenticated" ||
    grep -Fq 'example.com/collect' "$TEMP_DIR/authenticated.stdin" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/authenticated.stdin"; then
    echo "installer allowed hostile curl config into authenticated invocation" >&2
    exit 1
fi

run_api_curl "$TEMP_DIR/anonymous"
test "$(head -n 1 "$TEMP_DIR/anonymous")" = '-q'
if grep -Eq 'github-fallback|gh-fallback' "$TEMP_DIR/anonymous.env"; then
    echo "installer exposed a fallback token in anonymous curl environment" >&2
    exit 1
fi
if grep -Fq 'Authorization:' "$TEMP_DIR/anonymous.stdin"; then
    echo "installer unexpectedly used a fallback token" >&2
    exit 1
fi
grep -Fq 'Accept: application/vnd.github+json' "$TEMP_DIR/anonymous.stdin"
test "$(grep -Fxc 'https://api.github.com/repos/hxly520/sub2api/releases/latest' "$TEMP_DIR/anonymous")" -eq 1
if grep -Fq 'example.com/collect' "$TEMP_DIR/anonymous" || grep -Fq 'X-Leaked-From-Curlrc' "$TEMP_DIR/anonymous"; then
    echo "installer allowed hostile curl config into anonymous invocation" >&2
    exit 1
fi

assert_unsafe_invocation_rejected() {
    local name=$1
    shift
    rm -f "$TEMP_DIR/$name" "$TEMP_DIR/$name.stdin"
    if CURL_ARGS_LOG="$TEMP_DIR/$name" PATH="$TEMP_DIR:$PATH" UPDATE_GITHUB_TOKEN="update-secret" \
        bash -c 'GITHUB_REPO=hxly520/sub2api; GITHUB_API_ROOT=https://api.github.com; GITHUB_API_VERSION=2022-11-28; REQUIRE_RELEASE_CHECKSUM=true; REQUIRE_RELEASE_MANIFEST=true; UPDATE_DOCKER_IMAGE_VALUE=ghcr.io/hxly520/sub2api; UPDATE_CHANNEL_VALUE=stable; UPDATE_IN_PLACE_ENABLED_VALUE=true; print_error() { echo "$*" >&2; }; source "$1"; shift; github_api_curl "$@"' bash \
        "$TEMP_DIR/github-api-curl.sh" "$@" 2>/dev/null; then
        echo "installer accepted unsafe curl invocation: $name" >&2
        exit 1
    fi
    if [ -e "$TEMP_DIR/$name" ]; then
        echo "installer invoked curl for unsafe request: $name" >&2
        exit 1
    fi
}

assert_unsafe_invocation_rejected non-api -s \
    "https://github.com/hxly520/sub2api/releases/download/v1/asset"
assert_unsafe_invocation_rejected mixed-host -s \
    "https://api.github.com/repos/hxly520/sub2api/releases/latest" \
    "https://example.com/collect"
assert_unsafe_invocation_rejected multiple-api -s \
    "https://api.github.com/repos/hxly520/sub2api/releases/latest" \
    "https://api.github.com/repos/hxly520/sub2api/releases"
assert_unsafe_invocation_rejected url-option -s --url \
    "https://example.com/collect" \
    "https://api.github.com/repos/hxly520/sub2api/releases/latest"

# Private assets must use the API URL helper, and neither archive nor checksum
# may fall back to an unauthenticated github.com browser URL.
# shellcheck disable=SC2016
grep -Fq 'github_asset_download "$archive_url"' "$ROOT_DIR/deploy/install.sh"
# shellcheck disable=SC2016
grep -Fq 'github_asset_download "$checksum_url"' "$ROOT_DIR/deploy/install.sh"
# shellcheck disable=SC2016
if grep -Fq 'releases/download/${LATEST_VERSION}' "$ROOT_DIR/deploy/install.sh"; then
    echo "installer still contains a browser release asset URL" >&2
    exit 1
fi

# Exercise the private asset helper and verify the token remains out of argv
# and the inherited process environment.
CURL_ARGS_LOG="$TEMP_DIR/asset" HOME="$TEMP_DIR/home" PATH="$TEMP_DIR:$PATH" \
    UPDATE_GITHUB_TOKEN="update-secret" GITHUB_TOKEN="github-fallback" GH_TOKEN="gh-fallback" \
    bash -c 'GITHUB_REPO=hxly520/sub2api; GITHUB_API_ROOT=https://api.github.com; GITHUB_API_VERSION=2022-11-28; REQUIRE_RELEASE_CHECKSUM=true; REQUIRE_RELEASE_MANIFEST=true; UPDATE_DOCKER_IMAGE_VALUE=ghcr.io/hxly520/sub2api; UPDATE_CHANNEL_VALUE=stable; UPDATE_IN_PLACE_ENABLED_VALUE=true; print_error() { echo "$*" >&2; }; source "$1"; github_asset_download "$2" "$3"' bash \
    "$TEMP_DIR/github-api-curl.sh" \
    "https://api.github.com/repos/hxly520/sub2api/releases/assets/123" \
    "$TEMP_DIR/downloaded"
grep -Fq 'header = "Authorization: Bearer update-secret"' "$TEMP_DIR/asset.stdin"
grep -Fxq -- '--location' "$TEMP_DIR/asset"
if grep -Eq 'update-secret|github-fallback|gh-fallback' "$TEMP_DIR/asset.env"; then
    echo "asset download exposed a token in the curl environment" >&2
    exit 1
fi
if grep -Fq 'update-secret' "$TEMP_DIR/asset"; then
    echo "asset download exposed the update token in curl argv" >&2
    exit 1
fi

echo "install GitHub token checks passed"
