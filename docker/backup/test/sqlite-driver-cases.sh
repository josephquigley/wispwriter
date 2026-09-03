#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "SKIP: sqlite3 not installed"
    exit 0
fi

# shellcheck source=../scripts/config.sh
source ../scripts/config.sh
# shellcheck source=../drivers/sqlite3.sh
source ../drivers/sqlite3.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

DB_FILENAME="$work/writefreely.db"
sqlite3 "$DB_FILENAME" "CREATE TABLE posts (id TEXT, title TEXT); INSERT INTO posts VALUES ('a','hello');"

assert_eq "$(db_staged_name)" "writefreely.db" "staged name"

db_check
assert_eq "$?" "0" "db_check passes on a real database"

db_dump "$work/staged.db"
assert_eq "$(sqlite3 "$work/staged.db" 'SELECT title FROM posts')" "hello" "dump carries the rows"
assert_eq "$(sqlite3 "$work/staged.db" 'PRAGMA integrity_check')" "ok" "dump is internally consistent"

# The dump must be a real copy, not a link back to the live file: restic has
# to be able to read it while WriteFreely keeps writing to the original.
sqlite3 "$DB_FILENAME" "UPDATE posts SET title='changed'"
assert_eq "$(sqlite3 "$work/staged.db" 'SELECT title FROM posts')" "hello" \
    "the dump is independent of later writes"

DB_FILENAME="$work/does-not-exist.db"
assert_fails "db_check fails on a missing database" db_check

# Restore installs the snapshot and clears the previous journal.
DB_FILENAME="$work/restored.db"
touch "$work/restored.db-journal"
db_restore "$work/staged.db"
assert_eq "$(sqlite3 "$DB_FILENAME" 'SELECT title FROM posts')" "hello" "restore installs the rows"
assert_eq "$([[ -e "$work/restored.db-journal" ]] && echo present || echo gone)" "gone" \
    "restore removes the stale journal"

finish
