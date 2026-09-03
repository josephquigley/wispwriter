#!/bin/bash
# PID lock shared by backup and restore, so neither can run while the other
# does. Sourced, never executed.

LOCK_FILE="${LOCK_FILE:-/tmp/writefreely-backup.lock}"

acquire_lock() {
    if [[ -f "$LOCK_FILE" ]]; then
        local held
        held="$(cat "$LOCK_FILE" 2>/dev/null)"
        if [[ -n "$held" ]] && kill -0 "$held" 2>/dev/null; then
            log_error "another backup or restore is running (pid $held)"
            return 1
        fi
        log "removing a stale lock from pid ${held:-unknown}"
        rm -f "$LOCK_FILE"
    fi
    echo $$ > "$LOCK_FILE"
    return 0
}

release_lock() {
    rm -f "$LOCK_FILE" 2>/dev/null || true
}
