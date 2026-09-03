#!/bin/bash
# Container entrypoint: command dispatch, and the scheduler loop that is the
# default mode.
set -uo pipefail

SCRIPTS_DIR="${SCRIPTS_DIR:-/scripts}"
if [[ ! -d "$SCRIPTS_DIR" ]]; then
    SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/scripts" && pwd)"
fi

# shellcheck source=./scripts/common.sh
source "$SCRIPTS_DIR/common.sh"
# shellcheck source=./scripts/health.sh
source "$SCRIPTS_DIR/health.sh"

show_help() {
    cat <<'EOF'
WriteFreely Backup - restic backups for a WriteFreely deployment

Usage: docker compose run --rm backup <command>

Commands:
  (no command)     run the scheduler, backing up on BACKUP_SCHEDULE
  backup           back up now
  restore <id>     restore a snapshot ("latest" or a snapshot id)
  snapshots        list the snapshots in the repository
  verify           run the validation checks
  stats            show repository statistics
  unlock           remove a stale restic repository lock
  help             show this message

Database credentials come from /data/config.ini, not from the environment.
EOF
}

# One cron field against one current value. Supports "*", a literal, "*/N",
# "a-b", and a comma-separated list of those.
_field_matches() {
    local field="$1" current="$2" part parts
    [[ "$field" == "*" ]] && return 0
    IFS=',' read -r -a parts <<< "$field"
    for part in "${parts[@]}"; do
        if [[ "$part" == "*" ]]; then
            return 0
        elif [[ "$part" =~ ^\*/([0-9]+)$ ]]; then
            (( current % BASH_REMATCH[1] == 0 )) && return 0
        elif [[ "$part" =~ ^([0-9]+)-([0-9]+)$ ]]; then
            (( current >= BASH_REMATCH[1] && current <= BASH_REMATCH[2] )) && return 0
        elif [[ "$part" == "$current" ]]; then
            return 0
        fi
    done
    return 1
}

# schedule_matches <cron> <minute> <hour> <day-of-month> <month> <day-of-week>
schedule_matches() {
    local cron="$1" minute="$2" hour="$3" dom="$4" month="$5" dow="$6"
    local c_min c_hour c_dom c_mon c_dow
    read -r c_min c_hour c_dom c_mon c_dow <<< "$cron"
    _field_matches "$c_min" "$minute" || return 1
    _field_matches "$c_hour" "$hour" || return 1
    _field_matches "$c_dom" "$dom" || return 1
    _field_matches "$c_mon" "$month" || return 1
    _field_matches "$c_dow" "$dow" || return 1
    return 0
}

# Polls once every 30 seconds and compares the clock against the expression,
# rather than computing the next fire time. A backup schedule has minute
# resolution, so the drift costs nothing, and the arithmetic that would remove
# it is where cron implementations go wrong.
run_scheduler() {
    local schedule="${BACKUP_SCHEDULE:-0 3 * * *}"
    log "scheduler started, schedule: $schedule"

    if ! "$SCRIPTS_DIR/validate.sh"; then
        log_error "validation failed; fix the configuration and restart"
        # Keep the beacon fresh so the healthcheck reports the real problem
        # (nothing has been backed up) rather than a container that died.
        while true; do
            touch_alive
            sleep 30
        done
    fi

    local last_run="" stamp
    while true; do
        touch_alive
        stamp="$(date '+%Y-%m-%dT%H:%M')"
        # shellcheck disable=SC2046
        if schedule_matches "$schedule" $(date '+%-M %-H %-d %-m %w'); then
            if [[ "$stamp" != "$last_run" ]]; then
                last_run="$stamp"
                log "the schedule matched; starting a backup"
                "$SCRIPTS_DIR/backup.sh" || log_error "the backup run failed"
            fi
        fi
        sleep 30
    done
}

main() {
    case "${1:-}" in
        "")         run_scheduler ;;
        backup)     exec "$SCRIPTS_DIR/backup.sh" ;;
        restore)    shift; exec "$SCRIPTS_DIR/restore.sh" "$@" ;;
        verify)     exec "$SCRIPTS_DIR/validate.sh" ;;
        snapshots)  exec restic snapshots ;;
        stats)      exec restic stats ;;
        unlock)     exec restic unlock ;;
        help|-h|--help) show_help ;;
        *)          log_error "unknown command: $1"; show_help; exit 2 ;;
    esac
}

# The tests source this file for schedule_matches, so only run when executed.
if [[ -z "${ENTRYPOINT_SOURCED_FOR_TEST:-}" ]]; then
    main "$@"
fi
