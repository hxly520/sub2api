#!/bin/sh

# Toggle the Sub2API channel-monitor runtime switch.
# This controls monitoring/probing only; it does not change channels.status.
#
# Usage:
#   sub2api-monitor-switch.sh off
#   sub2api-monitor-switch.sh on
#   sub2api-monitor-switch.sh status

set -eu
umask 027

DB_CONTAINER="${SUB2API_MONITOR_DB_CONTAINER:-sub2api-postgres}"
DB_NAME="${SUB2API_MONITOR_DB_NAME:-sub2api}"
DB_USER="${SUB2API_MONITOR_DB_USER:-sub2api}"
LOCK_DIR="${SUB2API_MONITOR_LOCK_DIR:-/var/lock/sub2api-monitor-switch.lock}"
LOG_FILE="${SUB2API_MONITOR_LOG_FILE:-/var/log/sub2api-monitor-switch.log}"

PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/local/bin
export PATH

ACTION="${1:-}"

log() {
  printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S%z')" "$*" >> "$LOG_FILE"
}

if [ "$(id -u)" -ne 0 ]; then
  printf '%s\n' "This script must run as root." >&2
  exit 3
fi

if [ "$ACTION" != "on" ] && [ "$ACTION" != "off" ] && [ "$ACTION" != "status" ]; then
  printf 'Usage: %s {on|off|status}\n' "$0" >&2
  exit 2
fi

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  # Do not treat a concurrent invocation as a service failure.
  printf '%s\n' "Another sub2api-monitor-switch instance is running." >&2
  exit 0
fi

cleanup() {
  rmdir "$LOCK_DIR" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if ! command -v docker >/dev/null 2>&1; then
  log "docker command not found"
  exit 4
fi

if [ "$(docker inspect -f '{{.State.Running}}' "$DB_CONTAINER" 2>/dev/null || true)" != "true" ]; then
  log "database container is not running: $DB_CONTAINER"
  exit 5
fi

run_sql() {
  docker exec -i "$DB_CONTAINER" psql \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 \
    -Atc "$1"
}

set_monitor_enabled() {
  value="$1"
  run_sql "
BEGIN;
INSERT INTO settings (key, value, updated_at)
VALUES ('channel_monitor_enabled', '$value', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at;
COMMIT;
" >/dev/null
}

get_status() {
  run_sql "SELECT COALESCE((SELECT value FROM settings WHERE key = 'channel_monitor_enabled'), 'true');"
}

case "$ACTION" in
  off)
    if ! set_monitor_enabled false; then
      log "disable failed: database update error"
      exit 10
    fi
    STATUS="$(get_status)"
    log "channel monitor disabled, current=$STATUS"
    [ "$STATUS" = "false" ] || { log "disable failed, current=$STATUS"; exit 11; }
    ;;
  on)
    if ! set_monitor_enabled true; then
      log "enable failed: database update error"
      exit 12
    fi
    STATUS="$(get_status)"
    log "channel monitor enabled, current=$STATUS"
    [ "$STATUS" = "true" ] || { log "enable failed, current=$STATUS"; exit 13; }
    ;;
  status)
    STATUS="$(get_status)"
    printf 'channel_monitor_enabled=%s\n' "$STATUS"
    log "status channel_monitor_enabled=$STATUS"
    ;;
esac

exit 0
