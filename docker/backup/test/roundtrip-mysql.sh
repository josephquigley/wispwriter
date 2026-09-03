#!/bin/bash
# End-to-end against a real MariaDB, using the built sidecar image. Slow, and
# needs Docker, so it is not part of run.sh.
#
#   docker build -f Dockerfile.mysql -t wf-backup:mysql .
#   bash test/roundtrip-mysql.sh
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

IMAGE="${BACKUP_IMAGE:-wf-backup:mysql}"
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "SKIP: $IMAGE is not built"
    exit 0
fi

NET="wf-backup-roundtrip"
DB="wf-backup-roundtrip-db"
work="$(mktemp -d)"

cleanup() {
    docker rm -f "$DB" >/dev/null 2>&1
    docker network rm "$NET" >/dev/null 2>&1
    rm -rf "$work"
}
trap cleanup EXIT

mkdir -p "$work/data/keys" "$work/data/uploads" "$work/restore" "$work/repo"
echo "the-key" > "$work/data/keys/email.aes256"
cat > "$work/data/config.ini" <<INI
[database]
type     = mysql
username = writefreely
password = test-db-password
database = writefreely
host     = $DB
port     = 3306
INI

docker network create "$NET" >/dev/null
docker run -d --name "$DB" --network "$NET" \
    -e MARIADB_ROOT_PASSWORD=test-root-password \
    -e MARIADB_DATABASE=writefreely \
    -e MARIADB_USER=writefreely \
    -e MARIADB_PASSWORD=test-db-password \
    mariadb:11.4 >/dev/null

echo "waiting for MariaDB ..."
for _ in $(seq 1 60); do
    if docker exec "$DB" mariadb-admin ping -h 127.0.0.1 -uroot -ptest-root-password --silent >/dev/null 2>&1; then
        break
    fi
    sleep 2
done

seed() {
    docker exec -i "$DB" mariadb -uwritefreely -ptest-db-password writefreely -e "$1"
}
seed "CREATE TABLE IF NOT EXISTS posts (id INT, title TEXT); DELETE FROM posts; INSERT INTO posts VALUES (1,'original');" >/dev/null 2>&1

sidecar() {
    docker run --rm --network "$NET" \
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
assert_eq "$?" "0" "verify passes against a real MariaDB"

sidecar backup >/dev/null 2>&1
assert_eq "$?" "0" "the backup run succeeds"

assert_contains "$(sidecar snapshots 2>/dev/null)" "testhost" "the snapshot is in the repository"

seed "UPDATE posts SET title='changed';" >/dev/null 2>&1
assert_eq "$(seed 'SELECT title FROM posts;' 2>/dev/null | tail -1)" "changed" "the row was changed"

sidecar restore latest --yes --components=database >/dev/null 2>&1
assert_eq "$?" "0" "the restore succeeds"
assert_eq "$(seed 'SELECT title FROM posts;' 2>/dev/null | tail -1)" "original" \
    "the database is back to the snapshot contents"

finish
