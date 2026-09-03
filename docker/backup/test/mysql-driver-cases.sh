#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh
# shellcheck source=../scripts/common.sh
source ../scripts/common.sh
# shellcheck source=../scripts/config.sh
source ../scripts/config.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Stubs that record their arguments instead of talking to a database. The
# real engine is exercised by the compose round-trip.
mkdir -p "$work/bin"
cat > "$work/bin/mariadb-dump" <<'STUB'
#!/bin/bash
printf '%s\n' "$*" > "$RECORD"
echo "-- fake dump"
echo "-- Dump completed"
STUB
cat > "$work/bin/mariadb" <<'STUB'
#!/bin/bash
printf '%s\n' "$*" > "$RECORD"
# Drain a piped restore, but never block waiting on a terminal.
[ -t 0 ] || cat > /dev/null
STUB
chmod +x "$work/bin/mariadb-dump" "$work/bin/mariadb"
PATH="$work/bin:$PATH"
export PATH

# shellcheck source=../drivers/mysql.sh
source ../drivers/mysql.sh

load_config fixtures/mysql.ini

assert_eq "$(db_staged_name)" "writefreely.sql" "staged name"

RECORD="$work/dump-args"; export RECORD
db_dump "$work/out.sql"
args="$(cat "$RECORD")"

assert_contains "$args" "--single-transaction" "the dump is transactional"
assert_contains "$args" "--routines" "the dump carries routines"
assert_contains "$args" "--triggers" "the dump carries triggers"
# A password on the command line is visible to every process on the host
# through /proc/<pid>/cmdline.
assert_not_contains "$args" "s3cret" "the password is not on the command line"
assert_eq "$(grep -c 'Dump completed' "$work/out.sql")" "1" "the dump lands in the outfile"

RECORD="$work/check-args"; export RECORD
db_check
assert_eq "$?" "0" "db_check passes when the client connects"

RECORD="$work/restore-args"; export RECORD
db_restore "$work/out.sql"
assert_contains "$(cat "$RECORD")" "writefreely" "restore names the database"

finish
