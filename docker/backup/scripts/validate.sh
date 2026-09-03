#!/bin/bash
# Startup and pre-flight validation. Run on its own by `verify`, and by the
# scheduler once before the first backup, so that a misconfigured sidecar
# says so immediately instead of at the first scheduled run days later.
set -uo pipefail

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${DATA_DIR:-/data}"

# shellcheck source=./common.sh
source "$SCRIPTS_DIR/common.sh"
# shellcheck source=./config.sh
source "$SCRIPTS_DIR/config.sh"

ERRORS=0

add_error() {
    log_error "$*"
    ERRORS=$((ERRORS + 1))
}

check_env() {
    [[ -n "${RESTIC_REPOSITORY:-}" ]] || add_error "RESTIC_REPOSITORY is not set"
    [[ -n "${RESTIC_PASSWORD:-}" ]] || add_error "RESTIC_PASSWORD is not set"
}

check_config() {
    if ! load_config "$DATA_DIR/config.ini"; then
        add_error "could not read $DATA_DIR/config.ini"
        return 1
    fi
    local driver="$SCRIPTS_DIR/../drivers/$DB_TYPE.sh"
    if [[ ! -f "$driver" ]]; then
        add_error "no driver for [database] type = $DB_TYPE"
        return 1
    fi
    # shellcheck source=/dev/null
    source "$driver"
    log_success "config.ini: type = $DB_TYPE"
    return 0
}

check_database() {
    if ! db_check; then
        add_error "the database is not reachable"
        return 1
    fi
    log_success "database reachable"
}

check_data_dir() {
    [[ -d "$DATA_DIR" ]] || { add_error "$DATA_DIR is not mounted"; return 1; }
    [[ -r "$DATA_DIR" ]] || { add_error "$DATA_DIR is not readable"; return 1; }
    log_success "$DATA_DIR is readable"
}

# The staged dump needs room in /tmp. For SQLite that is the size of the
# database; for a dump it is roughly the size of the data. Ask for twice the
# database size and call it enough.
check_disk_space() {
    local need=0 have
    if [[ "$DB_TYPE" == "sqlite3" && -f "$DB_FILENAME" ]]; then
        need=$(( $(wc -c < "$DB_FILENAME") * 2 / 1024 ))
    fi
    have=$(df -Pk /tmp | awk 'NR==2 {print $4}')
    if [[ "$have" -lt "$need" ]]; then
        add_error "/tmp has ${have}K free, the staged dump needs about ${need}K"
        return 1
    fi
    log_success "/tmp has room for the staged dump"
}

check_restic() {
    if restic snapshots >/dev/null 2>&1; then
        log_success "restic repository is accessible"
        return 0
    fi
    # Distinguish "no repository yet" from "wrong password". Initialising over
    # a repository we simply cannot decrypt would quietly start a second one
    # and leave every existing snapshot orphaned.
    local output
    if output=$(restic init 2>&1); then
        log_success "initialised a new restic repository"
        return 0
    fi
    case "$output" in
        *"already initialized"*|*"already exists"*)
            add_error "the restic repository exists but cannot be opened; check RESTIC_PASSWORD" ;;
        *)
            add_error "cannot reach the restic repository: $output" ;;
    esac
    return 1
}

main() {
    log "validating the backup configuration"
    check_env
    if [[ "$ERRORS" -eq 0 ]]; then
        check_config && check_database && check_data_dir && check_disk_space
        check_restic
    fi
    if [[ "$ERRORS" -ne 0 ]]; then
        log_error "$ERRORS problem(s) found"
        exit 1
    fi
    log_success "all checks passed"
}

main "$@"
