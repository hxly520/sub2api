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

sed -n '/^validate_source() {/,/^# Main installation function/p' \
    "$ROOT_DIR/deploy/docker-deploy.sh" > "$TEMP_DIR/private-source.sh"
grep -Fq 'download_repository_file() {' "$TEMP_DIR/private-source.sh"

CURL_ARGS_LOG="$TEMP_DIR/download" PATH="$TEMP_DIR:$PATH" \
    UPDATE_GITHUB_TOKEN="update-secret" GITHUB_TOKEN="github-fallback" GH_TOKEN="gh-fallback" \
    bash -c '
        UPDATE_REPOSITORY=hxly520/sub2api
        UPDATE_GITHUB_REF=v0.1.173-52t.1
        GITHUB_API_ROOT=https://api.github.com
        GITHUB_API_VERSION=2022-11-28
        print_error() { echo "$*" >&2; }
        source "$1"
        download_repository_file deploy/docker-compose.local.yml "$2"
    ' bash "$TEMP_DIR/private-source.sh" "$TEMP_DIR/compose.yml"

grep -Fxq -- '--config' "$TEMP_DIR/download"
grep -Fxq -- '-' "$TEMP_DIR/download"
grep -Fq 'https://api.github.com/repos/hxly520/sub2api/contents/deploy/docker-compose.local.yml?ref=v0.1.173-52t.1' "$TEMP_DIR/download"
grep -Fq 'header = "Authorization: Bearer update-secret"' "$TEMP_DIR/download.stdin"
grep -Fq 'header = "Accept: application/vnd.github.raw+json"' "$TEMP_DIR/download.stdin"
if grep -Fq 'update-secret' "$TEMP_DIR/download"; then
    echo "docker deploy exposed the update token in curl argv" >&2
    exit 1
fi
if grep -Eq 'update-secret|github-fallback|gh-fallback' "$TEMP_DIR/download.env"; then
    echo "docker deploy exposed a token in the curl environment" >&2
    exit 1
fi

assert_invalid_source_rejected() {
    local name=$1
    local repository=$2
    local ref=$3
    rm -f "$TEMP_DIR/$name" "$TEMP_DIR/$name.stdin"
    if CURL_ARGS_LOG="$TEMP_DIR/$name" PATH="$TEMP_DIR:$PATH" \
        UPDATE_GITHUB_TOKEN="update-secret" \
        bash -c '
            UPDATE_REPOSITORY="$2"
            UPDATE_GITHUB_REF="$3"
            GITHUB_API_ROOT=https://api.github.com
            GITHUB_API_VERSION=2022-11-28
            print_error() { echo "$*" >&2; }
            source "$1"
            download_repository_file deploy/.env.example "$4"
        ' bash "$TEMP_DIR/private-source.sh" "$repository" "$ref" "$TEMP_DIR/output" 2>/dev/null; then
        echo "docker deploy accepted invalid source: $name" >&2
        exit 1
    fi
    test ! -e "$TEMP_DIR/$name"
}

assert_invalid_source_rejected invalid-repository 'hxly520/sub2api?x=1' main
assert_invalid_source_rejected invalid-ref hxly520/sub2api '../../main'

official_owner='Wei''-Shaw'
official_image='wei''shaw/sub2api'
if grep -Eq "${official_owner}/sub2api|${official_image}|raw\\.githubusercontent\\.com/${official_owner}/sub2api" \
    "$ROOT_DIR/deploy/docker-deploy.sh" "$ROOT_DIR/deploy/install.sh"; then
    echo "private deployment scripts still reference the official source" >&2
    exit 1
fi

echo "docker deploy private source checks passed"
