#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

cat > "$TEMP_DIR/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$DOCKER_ARGS_LOG"
EOF
chmod +x "$TEMP_DIR/docker"

DOCKER_ARGS_LOG="$TEMP_DIR/version.args" PATH="$TEMP_DIR:$PATH" \
    "$ROOT_DIR/deploy/docker-compose-compat.sh" --version --short
cat > "$TEMP_DIR/version.expected" <<'EOF'
compose
version
--short
EOF
cmp "$TEMP_DIR/version.expected" "$TEMP_DIR/version.args"

DOCKER_ARGS_LOG="$TEMP_DIR/compose.args" PATH="$TEMP_DIR:$PATH" \
    "$ROOT_DIR/deploy/docker-compose-compat.sh" -f docker-compose.yml ps
cat > "$TEMP_DIR/compose.expected" <<'EOF'
compose
-f
docker-compose.yml
ps
EOF
cmp "$TEMP_DIR/compose.expected" "$TEMP_DIR/compose.args"

if PATH="$TEMP_DIR/empty" "$ROOT_DIR/deploy/docker-compose-compat.sh" ps >/dev/null 2>&1; then
    echo "docker-compose compatibility wrapper succeeded without docker" >&2
    exit 1
else
    status=$?
    test "$status" -eq 127
fi

echo "docker-compose compatibility checks passed"
