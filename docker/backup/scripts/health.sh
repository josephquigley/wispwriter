#!/bin/bash
# Health reporting: a local beacon the Docker HEALTHCHECK reads, and an
# optional outbound ping. Sourced, never executed.
#
# Everything here is best-effort. Monitoring must never break or fail the
# backup it is monitoring.

HEALTH_FILE="${HEALTH_FILE:-/tmp/backup-health}"
ALIVE_FILE="${ALIVE_FILE:-/tmp/backup-alive}"

# write_health <ok|fail>
# Rewritten every run, so its mtime is the time of the last run.
write_health() {
    echo "$1 $(date +%s)" > "$HEALTH_FILE" 2>/dev/null || true
}

touch_alive() {
    touch "$ALIVE_FILE" 2>/dev/null || true
}

# ping_healthcheck <success|fail>
ping_healthcheck() {
    local status="$1" url="${BACKUP_HEALTHCHECK_URL:-}"
    [[ -n "$url" ]] || return 0
    [[ "$status" == "fail" ]] && url="${url}/fail"
    curl -fsS -m 10 --retry 3 "$url" >/dev/null 2>&1 || true
    return 0
}
