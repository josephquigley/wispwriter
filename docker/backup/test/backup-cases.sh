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
mkdir -p "$work/data/keys" "$work/data/uploads" "$work/data/legacy-images/2024/01"
sqlite3 "$work/data/writefreely.db" \
    "CREATE TABLE posts (id TEXT, title TEXT); INSERT INTO posts VALUES ('a','hello');"
echo "not-a-real-key" > "$work/data/keys/email.aes256"
echo "an upload" > "$work/data/uploads/pic.png"
echo "a legacy image" > "$work/data/legacy-images/2024/01/old.jpg"
cat > "$work/data/config.ini" <<INI
[database]
type     = sqlite3
filename = $work/data/writefreely.db
INI

RESTIC_REPOSITORY="$work/repo"
RESTIC_PASSWORD="test-password"
export RESTIC_REPOSITORY RESTIC_PASSWORD
restic init >/dev/null 2>&1

run_backup() {
    DATA_DIR="$work/data" \
    STAGING_DIR="$work/staging" \
    LOCK_FILE="$work/lock" \
    HEALTH_FILE="$work/health" \
    ALIVE_FILE="$work/alive" \
    BACKUP_SITE="testsite" \
    BACKUP_HOST="testhost" \
    bash ../scripts/backup.sh
}

run_backup >/dev/null 2>&1
assert_eq "$?" "0" "the backup run succeeds"
assert_eq "$(cut -d' ' -f1 < "$work/health")" "ok" "the health beacon records success"
assert_eq "$(restic snapshots --json | grep -c '"hostname":"testhost"')" "1" "one snapshot exists"

files="$(restic ls latest 2>/dev/null)"

assert_contains "$files" "/staging/writefreely.db" "the staged database is in the snapshot"
assert_not_contains "$files" "/data/writefreely.db" "the live database is excluded"
assert_contains "$files" "keys/email.aes256" "keys are backed up"
assert_contains "$files" "uploads/pic.png" "uploads are backed up"
assert_contains "$files" "legacy-images/2024/01/old.jpg" "legacy images are backed up"
assert_contains "$files" "config.ini" "config.ini is backed up"

assert_eq "$([[ -d "$work/staging" ]] && echo present || echo gone)" "gone" \
    "the staging directory is cleaned up"

# A journal file left beside the database must never enter a snapshot.
touch "$work/data/writefreely.db-journal"
run_backup >/dev/null 2>&1
assert_not_contains "$(restic ls latest 2>/dev/null)" "writefreely.db-journal" \
    "journal files are excluded"
rm -f "$work/data/writefreely.db-journal"

# A database the driver cannot read must fail the run and say so in the
# beacon, rather than leaving a stale "ok" for the healthcheck.
mv "$work/data/writefreely.db" "$work/hidden.db"
assert_fails "an unreadable database fails the run" run_backup
assert_eq "$(cut -d' ' -f1 < "$work/health")" "fail" "a failed run records fail"
mv "$work/hidden.db" "$work/data/writefreely.db"

finish
