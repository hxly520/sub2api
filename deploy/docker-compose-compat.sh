#!/bin/sh
# Preserve the legacy docker-compose command while using the installed
# Docker Compose v2 plugin.
case "${1:-}" in
    --version|-v)
        shift
        exec docker compose version "$@"
        ;;
esac
exec docker compose "$@"
