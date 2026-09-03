#!/bin/bash
# MySQL/MariaDB driver. Sourced when config.ini says `type = mysql`.
#
# The client is MariaDB's, because that is what the WriteFreely compose
# stacks actually run (lscr.io/linuxserver/mariadb). The commands are
# mariadb-dump and mariadb.

db_staged_name() {
    printf 'writefreely.sql'
}

# The password goes through the environment, never the command line: an
# argument is visible to every process on the host in /proc/<pid>/cmdline.
_mysql_env() {
    MYSQL_PWD="$DB_PASSWORD"
    export MYSQL_PWD
}

db_check() {
    if [[ -z "$DB_NAME" ]]; then
        log_error "config.ini has no [database] database"
        return 1
    fi
    _mysql_env
    if ! mariadb -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" "$DB_NAME" \
        -e 'SELECT 1' >/dev/null 2>&1; then
        log_error "cannot connect to $DB_HOST:$DB_PORT/$DB_NAME as $DB_USER"
        return 1
    fi
    return 0
}

# db_dump <outfile>
db_dump() {
    local out="$1"
    _mysql_env
    if ! mariadb-dump -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
        --single-transaction --quick --routines --triggers \
        "$DB_NAME" > "$out"; then
        log_error "mariadb-dump failed"
        return 1
    fi
    return 0
}

# db_restore <infile>
db_restore() {
    local in="$1"
    _mysql_env
    if ! mariadb -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" "$DB_NAME" < "$in"; then
        log_error "restore into $DB_HOST:$DB_PORT/$DB_NAME failed"
        return 1
    fi
    return 0
}
