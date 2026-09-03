#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# `touch -d "10 minutes ago"` is GNU-only and this suite also runs on a Mac,
# so ages are set with utime instead.
age_file() {
    perl -e 'utime time - $ARGV[0], time - $ARGV[0], $ARGV[1] or die $!' "$1" "$2"
}

# run_case <name> <expected-exit> <setup>
run_case() {
    local name="$1" expected="$2" setup="$3" actual
    rm -f "$work/alive" "$work/health"
    eval "$setup"
    ALIVE_FILE="$work/alive" HEALTH_FILE="$work/health" \
        bash ../scripts/healthcheck.sh
    actual=$?
    assert_eq "$actual" "$expected" "$name"
}

# The setup strings are single-quoted on purpose: run_case evals them after
# clearing the previous case's files, so they must expand then, not now.
# shellcheck disable=SC2016
run_case "healthy: fresh beacon and an ok run" 0 \
    'touch "$work/alive"; echo "ok $(date +%s)" > "$work/health"'
# shellcheck disable=SC2016
run_case "unhealthy: nothing at all" 1 'true'
# shellcheck disable=SC2016
run_case "unhealthy: the last run failed" 1 \
    'touch "$work/alive"; echo "fail $(date +%s)" > "$work/health"'
# shellcheck disable=SC2016
run_case "unhealthy: the scheduler beacon is stale" 1 \
    'touch "$work/alive"; echo "ok $(date +%s)" > "$work/health"; age_file 600 "$work/alive"'
# shellcheck disable=SC2016
run_case "unhealthy: the last run is too old" 1 \
    'touch "$work/alive"; echo "ok $(date +%s)" > "$work/health"; age_file 777600 "$work/health"'
# shellcheck disable=SC2016
run_case "unhealthy: the health file is malformed" 1 \
    'touch "$work/alive"; echo "garbage" > "$work/health"'

finish
