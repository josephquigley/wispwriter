#!/bin/bash
# Reads WriteFreely's config.ini and exports the [database] values.
#
# The sidecar takes database credentials from config.ini rather than from its
# own environment, because WriteFreely already stores them there and a second
# copy of a password is a second thing to leak and a second thing to drift.
# See config/config.go for the key names.
#
# Sourced, never executed.

# shellcheck source=./common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# These are the parser's output. They are read by the drivers and by
# backup.sh, which shellcheck cannot see from here.
# shellcheck disable=SC2034
DB_TYPE=""
# shellcheck disable=SC2034
DB_FILENAME=""
# shellcheck disable=SC2034
DB_HOST=""
# shellcheck disable=SC2034
DB_PORT=""
# shellcheck disable=SC2034
DB_USER=""
# shellcheck disable=SC2034
DB_PASSWORD=""
# shellcheck disable=SC2034
DB_NAME=""

# Expands a leading ${VAR} reference. WriteFreely does not support this today,
# but making config.ini reference the environment instead of embedding secrets
# is planned, and a parser that passed the literal text through would fail
# with an error naming a shell variable rather than a credential problem.
_expand_value() {
    local value="$1"
    if [[ "$value" =~ ^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$ ]]; then
        local name="${BASH_REMATCH[1]}"
        if [[ -z "${!name+set}" ]]; then
            log_error "config.ini references \${$name}, which is not set"
            return 1
        fi
        printf '%s' "${!name}"
        return 0
    fi
    printf '%s' "$value"
}

# load_config <path-to-config.ini>
load_config() {
    local path="$1"
    local line section="" key value

    if [[ ! -f "$path" ]]; then
        log_error "no config.ini at $path"
        return 1
    fi

    DB_TYPE=""; DB_FILENAME=""; DB_HOST=""; DB_PORT=""
    DB_USER=""; DB_PASSWORD=""; DB_NAME=""

    local seen_section=0
    while IFS= read -r line || [[ -n "$line" ]]; do
        # Strip a comment only when it starts the line. A '#' inside a value is
        # part of the value: passwords contain them.
        [[ "$line" =~ ^[[:space:]]*[#\;] ]] && continue
        [[ "$line" =~ ^[[:space:]]*$ ]] && continue

        if [[ "$line" =~ ^[[:space:]]*\[([^]]+)\][[:space:]]*$ ]]; then
            section="${BASH_REMATCH[1]}"
            [[ "$section" == "database" ]] && seen_section=1
            continue
        fi

        [[ "$section" == "database" ]] || continue
        [[ "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=[[:space:]]*(.*)$ ]] || continue

        key="${BASH_REMATCH[1]}"
        value="${BASH_REMATCH[2]}"
        # Trailing whitespace only. Nothing else is trimmed.
        value="${value%"${value##*[![:space:]]}"}"

        value="$(_expand_value "$value")" || return 1

        # shellcheck disable=SC2034
        case "$key" in
            type)     DB_TYPE="$value" ;;
            filename) DB_FILENAME="$value" ;;
            host)     DB_HOST="$value" ;;
            port)     DB_PORT="$value" ;;
            username) DB_USER="$value" ;;
            password) DB_PASSWORD="$value" ;;
            database) DB_NAME="$value" ;;
            *)        ;;
        esac
    done < "$path"

    if [[ "$seen_section" -eq 0 ]]; then
        log_error "$path has no [database] section"
        return 1
    fi
    if [[ -z "$DB_TYPE" ]]; then
        log_error "$path has no [database] type"
        return 1
    fi

    DB_HOST="${DB_HOST:-db}"
    DB_PORT="${DB_PORT:-3306}"
    return 0
}
