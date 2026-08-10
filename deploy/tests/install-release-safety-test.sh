#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
FIXTURE_ROOT=$(mktemp -d)
trap 'rm -rf "$FIXTURE_ROOT"' EXIT

sed -n '/^download_and_extract() {/,/^# Create system user/p' \
    "$ROOT_DIR/deploy/install.sh" > "$FIXTURE_ROOT/download-and-extract.sh"
grep -Fq 'download_and_extract() {' "$FIXTURE_ROOT/download-and-extract.sh"

mkdir -p "$FIXTURE_ROOT/archive"
cat > "$FIXTURE_ROOT/archive/sub2api" <<'EOF'
#!/bin/sh
echo "Sub2API 0.1.172-52t.1 (commit: 0123456789abcdef0123456789abcdef01234567, built: fixture)"
EOF
chmod +x "$FIXTURE_ROOT/archive/sub2api"
tar -czf "$FIXTURE_ROOT/sub2api_0.1.172-52t.1_linux_amd64.tar.gz" \
    -C "$FIXTURE_ROOT/archive" sub2api

ARCHIVE_NAME="sub2api_0.1.172-52t.1_linux_amd64.tar.gz"
ARCHIVE_PATH="$FIXTURE_ROOT/$ARCHIVE_NAME"
ARCHIVE_SIZE=$(wc -c < "$ARCHIVE_PATH" | tr -d '[:space:]')
ARCHIVE_SHA256=$(sha256sum "$ARCHIVE_PATH" | awk '{print $1}')
printf '%s  %s\n' "$ARCHIVE_SHA256" "$ARCHIVE_NAME" > "$FIXTURE_ROOT/checksums.txt"

write_manifest() {
    local version="$1"
    local source_commit="${2:-0123456789abcdef0123456789abcdef01234567}"
    cat > "$FIXTURE_ROOT/update-manifest.json" <<EOF
{"schema_version":1,"version":"$version","source_commit":"$source_commit","policy":"image-update-required","reasons":["fixture"]}
EOF
}

# Helpers and variables below are consumed by the dynamically sourced installer
# function, which static analysis cannot follow.
# shellcheck disable=SC1091,SC2034,SC2329
run_download_fixture() {
    local install_dir="$1"
    (
        source "$FIXTURE_ROOT/download-and-extract.sh"
        INSTALL_DIR="$install_dir"
        SERVICE_USER="fixture"
        LATEST_VERSION="v0.1.172-52t.1"
        OS="linux"
        ARCH="amd64"
        REQUIRE_RELEASE_CHECKSUM="true"
        REQUIRE_RELEASE_MANIFEST="true"

        msg() { printf '%s' "$1"; }
        print_info() { :; }
        print_success() { :; }
        print_warning() { :; }
        print_error() { printf '%s\n' "$*" >&2; }
        validate_release_tag_format() {
            [[ "$1" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-52t\.([1-9][0-9]*)$ ]]
        }
        fetch_release_json() {
            printf '%s' '{"tag_name":"v0.1.172-52t.1","draft":false,"prerelease":false,"assets":[]}'
        }
        release_asset_url() {
            case "$2" in
                "$ARCHIVE_NAME") printf '%s' archive-url ;;
                checksums.txt) printf '%s' checksum-url ;;
                update-manifest.json) printf '%s' manifest-url ;;
                *) return 1 ;;
            esac
        }
        release_asset_size() { printf '%s' "$ARCHIVE_SIZE"; }
        github_asset_download() {
            case "$1" in
                archive-url) cp "$ARCHIVE_PATH" "$2" ;;
                checksum-url) cp "$FIXTURE_ROOT/checksums.txt" "$2" ;;
                manifest-url) cp "$FIXTURE_ROOT/update-manifest.json" "$2" ;;
                *) return 1 ;;
            esac
        }
        calculate_sha256() { sha256sum "$1" | awk '{print $1}'; }
        persist_update_environment() { :; }

        download_and_extract
    )
}

write_manifest "0.1.172-52t.1"
run_download_fixture "$FIXTURE_ROOT/install-valid"
test -x "$FIXTURE_ROOT/install-valid/sub2api"
"$FIXTURE_ROOT/install-valid/sub2api" --version | grep -Fq 'Sub2API 0.1.172-52t.1'

mkdir -p "$FIXTURE_ROOT/install-invalid"
printf '%s\n' '#!/bin/sh' 'echo old-binary' > "$FIXTURE_ROOT/install-invalid/sub2api"
chmod +x "$FIXTURE_ROOT/install-invalid/sub2api"
write_manifest "0.1.999-52t.1"
if run_download_fixture "$FIXTURE_ROOT/install-invalid" >/dev/null 2>&1; then
    echo "installer accepted a mismatched release manifest" >&2
    exit 1
fi
"$FIXTURE_ROOT/install-invalid/sub2api" | grep -Fq 'old-binary'

mkdir -p "$FIXTURE_ROOT/install-invalid-commit"
printf '%s\n' '#!/bin/sh' 'echo old-binary' > "$FIXTURE_ROOT/install-invalid-commit/sub2api"
chmod +x "$FIXTURE_ROOT/install-invalid-commit/sub2api"
write_manifest "0.1.172-52t.1" "89abcdef0123456789abcdef0123456789abcdef"
if run_download_fixture "$FIXTURE_ROOT/install-invalid-commit" >/dev/null 2>&1; then
    echo "installer accepted a binary whose embedded commit did not match the manifest" >&2
    exit 1
fi
"$FIXTURE_ROOT/install-invalid-commit/sub2api" | grep -Fq 'old-binary'

# Network and validation work must complete before the only service restart,
# and restart failure must have an automatic binary restore path.
if sed -n '/^upgrade() {/,/^# Uninstall function/p' "$ROOT_DIR/deploy/install.sh" |
    grep -Fq 'systemctl stop sub2api'; then
    echo "installer still stops the service before update preflight" >&2
    exit 1
fi
grep -Fq 'restore_binary_backup' "$ROOT_DIR/deploy/install.sh"
grep -Fq 'wait_for_service_stability' "$ROOT_DIR/deploy/install.sh"

echo "install release safety checks passed"
