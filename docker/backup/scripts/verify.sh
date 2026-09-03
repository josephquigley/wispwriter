#!/bin/bash
# Verification of a staged database copy, before it is handed to restic.
# Sourced, never executed.
#
# A backup nobody verifies is a backup you find out about during a restore.

# verify_staged <file>
# Reads DB_TYPE to decide what a usable copy looks like.
verify_staged() {
    local staged="$1"
    if [[ ! -s "$staged" ]]; then
        log_error "the staged dump is empty"
        return 1
    fi
    if [[ "$DB_TYPE" == "sqlite3" ]]; then
        local result
        result="$(sqlite3 "$staged" 'PRAGMA integrity_check' 2>&1)"
        if [[ "$result" != "ok" ]]; then
            log_error "the staged database failed its integrity check: $result"
            return 1
        fi
    else
        # Checking only for a non-empty file passes happily on a dump that was
        # truncated halfway through, which is the failure that matters here.
        if ! tail -5 "$staged" | grep -q 'Dump completed'; then
            log_error "the staged dump has no completion marker, so it is truncated"
            return 1
        fi
    fi
    log_success "the staged copy verified"
    return 0
}
