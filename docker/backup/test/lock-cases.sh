#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh
# shellcheck source=../scripts/common.sh
source ../scripts/common.sh
# shellcheck source=../scripts/lock.sh
source ../scripts/lock.sh
# shellcheck source=../scripts/health.sh
source ../scripts/health.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

LOCK_FILE="$work/lock"
acquire_lock
assert_eq "$(cat "$LOCK_FILE")" "$$" "the lock records our pid"

# A lock held by a live process blocks a second acquisition.
sleep 60 &
live_pid=$!
echo "$live_pid" > "$LOCK_FILE"
assert_fails "a live lock blocks" acquire_lock
kill "$live_pid" 2>/dev/null
wait "$live_pid" 2>/dev/null

# A lock left by a dead process is stale and gets taken over.
echo 999999 > "$LOCK_FILE"
acquire_lock
assert_eq "$(cat "$LOCK_FILE")" "$$" "a stale lock is reclaimed"

release_lock
assert_eq "$([[ -e "$LOCK_FILE" ]] && echo present || echo gone)" "gone" "release removes the lock"

HEALTH_FILE="$work/health"
write_health ok
assert_eq "$(cut -d' ' -f1 < "$HEALTH_FILE")" "ok" "health records ok"
write_health fail
assert_eq "$(cut -d' ' -f1 < "$HEALTH_FILE")" "fail" "health records fail"

ALIVE_FILE="$work/alive"
touch_alive
assert_eq "$([[ -e "$ALIVE_FILE" ]] && echo present || echo gone)" "present" "the alive beacon is written"

# Monitoring must never break the backup: an unreachable URL is not an error.
BACKUP_HEALTHCHECK_URL="http://127.0.0.1:1/nope"
ping_healthcheck success
assert_eq "$?" "0" "an unreachable healthcheck URL does not fail the run"

finish
