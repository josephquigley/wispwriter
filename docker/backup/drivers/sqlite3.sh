#!/bin/bash
# SQLite driver. Sourced by backup.sh and restore.sh when config.ini says
# `type = sqlite3`. Provides db_check, db_dump, db_restore, db_staged_name.

db_staged_name() {
    printf 'writefreely.db'
}

db_check() {
    if [[ -z "$DB_FILENAME" ]]; then
        log_error "config.ini has no [database] filename"
        return 1
    fi
    if [[ ! -f "$DB_FILENAME" ]]; then
        log_error "database file not found: $DB_FILENAME"
        return 1
    fi
    if ! sqlite3 "$DB_FILENAME" 'SELECT 1' >/dev/null 2>&1; then
        log_error "cannot read $DB_FILENAME"
        return 1
    fi
    return 0
}

# db_dump <outfile>
#
# Uses SQLite's online backup API through the .backup dot-command, which is
# consistent against a live writer without quiescing the application and
# without holding a read lock for the duration. Copying the file with cp is
# not equivalent: it can capture a torn page mid-transaction.
db_dump() {
    local out="$1"
    if ! sqlite3 "$DB_FILENAME" ".backup '$out'"; then
        log_error "sqlite3 .backup failed"
        return 1
    fi
    return 0
}

# db_restore <infile>
#
# Removes the previous database's journal siblings first. Installing a new
# file while the old journal is still present hands SQLite a rollback journal
# describing a database that no longer exists.
db_restore() {
    local in="$1"
    rm -f "$DB_FILENAME-journal" "$DB_FILENAME-wal" "$DB_FILENAME-shm"
    if ! cp "$in" "$DB_FILENAME"; then
        log_error "could not install $in at $DB_FILENAME"
        return 1
    fi
    return 0
}
