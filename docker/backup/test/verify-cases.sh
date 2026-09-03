#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh
# shellcheck source=../scripts/common.sh
source ../scripts/common.sh
# shellcheck source=../scripts/verify.sh
source ../scripts/verify.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

DB_TYPE="mysql"
printf -- '-- fake dump\n-- Dump completed\n' > "$work/good.sql"
assert_eq "$(verify_staged "$work/good.sql" >/dev/null 2>&1; echo $?)" "0" \
    "a complete dump verifies"

# The failure this catches: mariadb-dump died halfway and left a file that is
# neither empty nor usable.
printf -- '-- fake dump\nINSERT INTO posts VALUES (1,' > "$work/truncated.sql"
assert_fails "a truncated dump is rejected" verify_staged "$work/truncated.sql"

: > "$work/empty.sql"
assert_fails "an empty dump is rejected" verify_staged "$work/empty.sql"

if command -v sqlite3 >/dev/null 2>&1; then
    DB_TYPE="sqlite3"
    sqlite3 "$work/good.db" "CREATE TABLE t (x TEXT); INSERT INTO t VALUES ('y');"
    assert_eq "$(verify_staged "$work/good.db" >/dev/null 2>&1; echo $?)" "0" \
        "a healthy database verifies"

    printf 'SQLite format 3\000this is not a database' > "$work/corrupt.db"
    assert_fails "a corrupt database is rejected" verify_staged "$work/corrupt.db"
else
    echo "SKIP: sqlite3 not installed, skipping the SQLite branch"
fi

finish
