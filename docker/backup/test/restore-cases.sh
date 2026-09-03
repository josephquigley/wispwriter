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
mkdir -p "$work/data/keys" "$work/data/uploads" "$work/restore"
sqlite3 "$work/data/writefreely.db" \
    "CREATE TABLE posts (id TEXT, title TEXT); INSERT INTO posts VALUES ('a','original');"
echo "the-key" > "$work/data/keys/email.aes256"
echo "an upload" > "$work/data/uploads/pic.png"
cat > "$work/data/config.ini" <<INI
[database]
type     = sqlite3
filename = $work/data/writefreely.db
INI

RESTIC_REPOSITORY="$work/repo"
RESTIC_PASSWORD="test-password"
export RESTIC_REPOSITORY RESTIC_PASSWORD
restic init >/dev/null 2>&1

DATA_DIR="$work/data" STAGING_DIR="$work/staging" LOCK_FILE="$work/lock" \
HEALTH_FILE="$work/health" ALIVE_FILE="$work/alive" BACKUP_HOST="testhost" \
    bash ../scripts/backup.sh >/dev/null 2>&1

# Change the live data, then restore over it.
sqlite3 "$work/data/writefreely.db" "UPDATE posts SET title='changed'"
echo "a different key" > "$work/data/keys/email.aes256"

run_restore() {
    DATA_DIR="$work/data" \
    RESTORE_DIR="$work/restore" \
    LOCK_FILE="$work/lock" \
    APP_URL="http://127.0.0.1:1/" \
    bash ../scripts/restore.sh "$@"
}

run_restore latest --yes --components=database,keys >/dev/null 2>&1
assert_eq "$?" "0" "the restore succeeds"
assert_eq "$(sqlite3 "$work/data/writefreely.db" 'SELECT title FROM posts')" "original" \
    "the database is back to the snapshot contents"
assert_eq "$(cat "$work/data/keys/email.aes256")" "the-key" "keys are restored"
assert_eq "$(find "$work/data" -maxdepth 1 -name 'keys.bak-*' | wc -l | tr -d ' ')" "1" \
    "the previous keys were kept as a .bak"
assert_eq "$(find "$work/data" -maxdepth 1 -name 'writefreely.db.bak-*' | wc -l | tr -d ' ')" "1" \
    "the previous database was kept as a .bak"

# An unselected component is left alone.
assert_eq "$(cat "$work/data/uploads/pic.png")" "an upload" "an unselected component is untouched"

# config.ini is never restored unless it is asked for by name, because an
# older one can point at a state directory that no longer exists.
echo "# edited" >> "$work/data/config.ini"
run_restore latest --yes --components=database >/dev/null 2>&1
assert_eq "$(tail -1 "$work/data/config.ini")" "# edited" "config.ini is not restored by --yes"

# A reachable application aborts the restore. A file:// URL stands in for a
# responding server, so the test needs no HTTP daemon.
DATA_DIR="$work/data" RESTORE_DIR="$work/restore" LOCK_FILE="$work/lock" \
APP_URL="file://$work/data/config.ini" \
    assert_fails "a running application aborts the restore" \
    bash ../scripts/restore.sh latest --yes --components=database

# ... unless --force is given.
DATA_DIR="$work/data" RESTORE_DIR="$work/restore" LOCK_FILE="$work/lock" \
APP_URL="file://$work/data/config.ini" \
    bash ../scripts/restore.sh latest --yes --force --components=database >/dev/null 2>&1
assert_eq "$?" "0" "--force overrides the running-application check"

finish
