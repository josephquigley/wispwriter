#!/bin/bash
# One backup run: stage the database, verify the staged copy, hand it and the
# data directory to restic, apply retention, report health.
set -uo pipefail

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${DATA_DIR:-/data}"
STAGING_DIR="${STAGING_DIR:-/tmp/backup-staging}"

# shellcheck source=./common.sh
source "$SCRIPTS_DIR/common.sh"
# shellcheck source=./config.sh
source "$SCRIPTS_DIR/config.sh"
# shellcheck source=./lock.sh
source "$SCRIPTS_DIR/lock.sh"
# shellcheck source=./health.sh
source "$SCRIPTS_DIR/health.sh"
# shellcheck source=./verify.sh
source "$SCRIPTS_DIR/verify.sh"

KEEP_DAILY="${BACKUP_KEEP_DAILY:-7}"
KEEP_WEEKLY="${BACKUP_KEEP_WEEKLY:-4}"
KEEP_MONTHLY="${BACKUP_KEEP_MONTHLY:-6}"
KEEP_YEARLY="${BACKUP_KEEP_YEARLY:-2}"
SITE="${BACKUP_SITE:-writefreely}"
HOST_TAG="${BACKUP_HOST:-$(hostname)}"

STARTED=0
SUCCEEDED=0

cleanup() {
    rm -rf "$STAGING_DIR"
    # An abort that never reached the success path must not leave a stale "ok"
    # for the healthcheck to read.
    if [[ "$STARTED" -eq 1 && "$SUCCEEDED" -ne 1 ]]; then
        write_health fail
        ping_healthcheck fail
    fi
    release_lock
}
trap cleanup EXIT

main() {
    acquire_lock || exit 1
    STARTED=1

    load_config "$DATA_DIR/config.ini" || exit 1
    local driver="$SCRIPTS_DIR/../drivers/$DB_TYPE.sh"
    [[ -f "$driver" ]] || { log_error "no driver for type = $DB_TYPE"; exit 1; }
    # shellcheck source=/dev/null
    source "$driver"

    db_check || exit 1

    rm -rf "$STAGING_DIR"
    mkdir -p "$STAGING_DIR"
    local staged_name staged
    staged_name="$(db_staged_name)"
    staged="$STAGING_DIR/$staged_name"

    log "staging the database"
    db_dump "$staged" || exit 1
    verify_staged "$staged" || exit 1

    log "backing up to the restic repository"
    # The live database and its journal siblings are excluded on purpose: the
    # staged copy is the only database in the snapshot, so no snapshot can
    # hold a torn one.
    restic backup \
        --host "$HOST_TAG" \
        --tag writefreely --tag "$SITE" --tag "$DB_TYPE" \
        --exclude "$DB_FILENAME" \
        --exclude '*.db-journal' \
        --exclude '*.db-wal' \
        --exclude '*.db-shm' \
        "$STAGING_DIR" "$DATA_DIR" || exit 1

    log "applying the retention policy"
    restic forget --prune \
        --host "$HOST_TAG" \
        --tag "$SITE" \
        --keep-daily "$KEEP_DAILY" \
        --keep-weekly "$KEEP_WEEKLY" \
        --keep-monthly "$KEEP_MONTHLY" \
        --keep-yearly "$KEEP_YEARLY" || exit 1

    SUCCEEDED=1
    write_health ok
    ping_healthcheck success
    log_success "backup complete"
}

main "$@"
