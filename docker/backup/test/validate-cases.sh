#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

if ! command -v restic >/dev/null 2>&1 || ! command -v sqlite3 >/dev/null 2>&1; then
    echo "SKIP: restic or sqlite3 not installed"
    exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/data"
sqlite3 "$work/data/writefreely.db" "CREATE TABLE posts (id TEXT);"
cat > "$work/data/config.ini" <<INI
[database]
type     = sqlite3
filename = $work/data/writefreely.db
INI

run_validate() {
    DATA_DIR="$work/data" \
    RESTIC_REPOSITORY="$1" \
    RESTIC_PASSWORD="${2-}" \
    bash ../scripts/validate.sh
}

assert_fails "a missing RESTIC_PASSWORD is fatal" run_validate "$work/repo" ""

run_validate "$work/repo" "test-password" >/dev/null 2>&1
assert_eq "$?" "0" "a valid setup validates, initialising the repository"
assert_eq "$([[ -f "$work/repo/config" ]] && echo yes || echo no)" "yes" \
    "the repository was initialised"

run_validate "$work/repo" "test-password" >/dev/null 2>&1
assert_eq "$?" "0" "an existing repository validates"

# A wrong password must be reported, not initialised over: initialising there
# would start a second repository and orphan every existing snapshot.
assert_fails "a wrong repository password is fatal" run_validate "$work/repo" "wrong-password"

rm -f "$work/data/config.ini"
assert_fails "a missing config.ini is fatal" run_validate "$work/repo" "test-password"

finish
