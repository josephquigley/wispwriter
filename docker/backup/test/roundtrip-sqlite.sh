#!/bin/bash
# End-to-end through the built SQLite image, which is what a deployment
# actually runs. Needs Docker, so it is not part of run.sh.
#
#   docker build -f Dockerfile.sqlite -t wf-backup:sqlite .
#   bash test/roundtrip-sqlite.sh
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

IMAGE="${BACKUP_IMAGE:-wf-backup:sqlite}"
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "SKIP: $IMAGE is not built"
    exit 0
fi
if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "SKIP: sqlite3 not installed"
    exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/data/keys" "$work/data/uploads" "$work/restore" "$work/repo"
sqlite3 "$work/data/writefreely.db" \
    "CREATE TABLE posts (id TEXT, title TEXT); INSERT INTO posts VALUES ('a','original');"
echo "the-key" > "$work/data/keys/email.aes256"
echo "an upload" > "$work/data/uploads/pic.png"
cat > "$work/data/config.ini" <<INI
[database]
type     = sqlite3
filename = /data/writefreely.db
INI

sidecar() {
    docker run --rm \
        -e RESTIC_REPOSITORY=/repo \
        -e RESTIC_PASSWORD=test-password \
        -e BACKUP_HOST=testhost \
        -e BACKUP_SITE=testsite \
        -e APP_URL=http://127.0.0.1:1/ \
        -v "$work/data:/data" \
        -v "$work/restore:/restore" \
        -v "$work/repo:/repo" \
        "$IMAGE" "$@"
}

sidecar verify >/dev/null 2>&1
assert_eq "$?" "0" "verify passes in the shipped image"

sidecar backup >/dev/null 2>&1
assert_eq "$?" "0" "the backup run succeeds"
assert_contains "$(sidecar snapshots 2>/dev/null)" "testhost" "the snapshot is in the repository"

sqlite3 "$work/data/writefreely.db" "UPDATE posts SET title='changed'"
echo "a different key" > "$work/data/keys/email.aes256"

sidecar restore latest --yes --components=database,keys >/dev/null 2>&1
assert_eq "$?" "0" "the restore succeeds"
assert_eq "$(sqlite3 "$work/data/writefreely.db" 'SELECT title FROM posts')" "original" \
    "the database is back to the snapshot contents"
assert_eq "$(cat "$work/data/keys/email.aes256")" "the-key" "keys are restored"
assert_eq "$(sqlite3 "$work/data/writefreely.db" 'PRAGMA integrity_check')" "ok" \
    "the restored database is healthy"

finish
