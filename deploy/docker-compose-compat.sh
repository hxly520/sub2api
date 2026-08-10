#!/bin/sh
# Preserve the legacy docker-compose command while using the installed
# Docker Compose v2 plugin.
set -eu

DOCKER_BIN="${DOCKER_BIN:-docker}"
if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
    echo "docker-compose compatibility wrapper: docker CLI not found" >&2
    exit 127
fi

case "${1:-}" in
    --version|-v)
        shift
        exec "$DOCKER_BIN" compose version "$@"
        ;;
esac
exec "$DOCKER_BIN" compose "$@"
