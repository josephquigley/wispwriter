#!/bin/bash
# Restore a snapshot into the data directory, one component at a time, with a
# .bak copy of whatever it replaces.
set -uo pipefail

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${DATA_DIR:-/data}"
RESTORE_DIR="${RESTORE_DIR:-/restore}"
APP_URL="${APP_URL:-http://app:8080/}"
PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# shellcheck source=./common.sh
source "$SCRIPTS_DIR/common.sh"
# shellcheck source=./config.sh
source "$SCRIPTS_DIR/config.sh"
# shellcheck source=./lock.sh
source "$SCRIPTS_DIR/lock.sh"

SNAPSHOT="latest"
ASSUME_YES=0
FORCE=0
COMPONENTS=""
STAMP="$(date +%Y%m%d-%H%M%S)"

usage() {
    cat <<'EOF'
Usage: restore <snapshot|latest> [--yes] [--force] [--components=a,b,c]

Components: database, keys, uploads, legacy-images, config

  --yes         restore every component except config.ini, without prompting
  --force       restore even though the application answers on the network
  --components  restore exactly these, without prompting for the others
EOF
}

parse_args() {
    local arg first=1
    for arg in "$@"; do
        case "$arg" in
            --yes) ASSUME_YES=1 ;;
            --force) FORCE=1 ;;
            --components=*) COMPONENTS="${arg#--components=}" ;;
            -h|--help) usage; exit 0 ;;
            -*) log_error "unknown option: $arg"; usage; exit 2 ;;
            *)  if [[ "$first" -eq 1 ]]; then
                    SNAPSHOT="$arg"; first=0
                else
                    log_error "unexpected argument: $arg"; exit 2
                fi ;;
        esac
    done
}

selected() {
    local name="$1"
    if [[ -n "$COMPONENTS" ]]; then
        case ",$COMPONENTS," in *",$name,"*) return 0 ;; *) return 1 ;; esac
    fi
    if [[ "$ASSUME_YES" -eq 1 ]]; then
        # config.ini is never swept up by a blanket yes. An older config
        # points at the pre-0.18 state directory, and dropping it onto a
        # /data install crash-loops the application on a config it cannot
        # find. Ask for it by name or not at all.
        [[ "$name" == "config" ]] && return 1
        return 0
    fi
    local prompt="Restore $name?"
    [[ "$name" == "config" ]] && \
        prompt="Restore config.ini? It can carry paths from an older layout."
    local answer
    read -r -p "$prompt [y/N] " answer
    [[ "$answer" == "y" || "$answer" == "Y" ]]
}

# The sidecar cannot stop the application container, and will not mount the
# Docker socket to gain the ability: that is root on the host handed to a
# backup script. It refuses instead.
check_app_stopped() {
    [[ "$FORCE" -eq 1 ]] && return 0
    if curl -fsS -m 5 "$APP_URL" >/dev/null 2>&1; then
        log_error "WriteFreely is answering at $APP_URL"
        log_error "stop it first (docker compose stop app), or pass --force"
        return 1
    fi
    return 0
}

# Keeps whatever is there rather than deleting it.
move_aside() {
    local path="$1"
    [[ -e "$path" ]] || return 0
    mv "$path" "${path}.bak-$STAMP" || return 1
    log "kept the previous $(basename "$path") as $(basename "$path").bak-$STAMP"
}

restore_tree() {
    local name="$1" src="$2" dest="$3"
    if [[ ! -e "$src" ]]; then
        log "the snapshot has no $name; skipping"
        return 0
    fi
    move_aside "$dest" || return 1
    if ! cp -a "$src" "$dest"; then
        log_error "restoring $name failed; putting the previous one back"
        rm -rf "$dest"
        [[ -e "${dest}.bak-$STAMP" ]] && mv "${dest}.bak-$STAMP" "$dest"
        return 1
    fi
    chown -R "$PUID:$PGID" "$dest" 2>/dev/null || true
    log_success "restored $name"
    return 0
}

restore_database() {
    local staged="$1"
    if [[ "$DB_TYPE" == "sqlite3" ]]; then
        move_aside "$DB_FILENAME" || return 1
    else
        log "about to replace the contents of $DB_HOST:$DB_PORT/$DB_NAME"
    fi
    if ! db_restore "$staged"; then
        if [[ "$DB_TYPE" == "sqlite3" && -e "${DB_FILENAME}.bak-$STAMP" ]]; then
            log_error "restoring the database failed; putting the previous one back"
            mv "${DB_FILENAME}.bak-$STAMP" "$DB_FILENAME"
        fi
        return 1
    fi
    [[ "$DB_TYPE" == "sqlite3" ]] && chown "$PUID:$PGID" "$DB_FILENAME" 2>/dev/null
    log_success "restored the database"
    return 0
}

main() {
    parse_args "$@"
    acquire_lock || exit 1
    trap release_lock EXIT

    load_config "$DATA_DIR/config.ini" || exit 1
    local driver="$SCRIPTS_DIR/../drivers/$DB_TYPE.sh"
    [[ -f "$driver" ]] || { log_error "no driver for type = $DB_TYPE"; exit 1; }
    # shellcheck source=/dev/null
    source "$driver"

    check_app_stopped || exit 1

    log "extracting $SNAPSHOT into $RESTORE_DIR"
    mkdir -p "$RESTORE_DIR"
    rm -rf "${RESTORE_DIR:?}/"*
    restic restore "$SNAPSHOT" --target "$RESTORE_DIR" || exit 1

    # restic restores absolute paths under the target, so find the two roots
    # rather than assuming where they landed.
    local staged extracted
    staged="$(find "$RESTORE_DIR" -name "$(db_staged_name)" -print 2>/dev/null | head -1)"
    extracted="$(find "$RESTORE_DIR" -type d -name "$(basename "$DATA_DIR")" -print 2>/dev/null | head -1)"

    log "snapshot contents:"
    [[ -n "$staged" ]] && log "  database: $(du -h "$staged" | cut -f1)"
    [[ -n "$extracted" ]] && log "  data:     $(du -sh "$extracted" | cut -f1)"

    local failures=0 name

    if [[ -n "$staged" ]] && selected database; then
        restore_database "$staged" || failures=$((failures + 1))
    fi

    for name in keys uploads legacy-images; do
        selected "$name" || continue
        restore_tree "$name" "$extracted/$name" "$DATA_DIR/$name" \
            || failures=$((failures + 1))
    done

    if selected config; then
        restore_tree "config.ini" "$extracted/config.ini" "$DATA_DIR/config.ini" \
            || failures=$((failures + 1))
    fi

    if [[ "$failures" -ne 0 ]]; then
        log_error "$failures component(s) failed to restore"
        exit 1
    fi

    log_success "restore complete"
    log "start WriteFreely, check it, then remove the .bak-$STAMP copies by hand"
}

main "$@"
