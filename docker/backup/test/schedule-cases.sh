#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh

ENTRYPOINT_SOURCED_FOR_TEST=1
export ENTRYPOINT_SOURCED_FOR_TEST
# shellcheck source=../entrypoint.sh
source ../entrypoint.sh

# check <match|no-match> <cron> <minute> <hour> <dom> <month> <dow>
check() {
    local expect="$1" cron="$2"; shift 2
    if schedule_matches "$cron" "$@"; then
        assert_eq "match" "$expect" "$cron at $*"
    else
        assert_eq "no-match" "$expect" "$cron at $*"
    fi
}

check match    "0 3 * * *"    0  3 15 6 4
check no-match "0 3 * * *"    1  3 15 6 4
check no-match "0 3 * * *"    0  4 15 6 4
check match    "0 0 * * 5"    0  0 15 6 5
check no-match "0 0 * * 5"    0  0 15 6 4
check match    "*/15 * * * *" 30 9  1 1 1
check no-match "*/15 * * * *" 31 9  1 1 1
check match    "0 3 1 * *"    0  3  1 6 4
check no-match "0 3 1 * *"    0  3  2 6 4
check match    "0 1,3 * * *"  0  3 15 6 4
check no-match "0 1,3 * * *"  0  2 15 6 4
check match    "0 9-17 * * *" 0 12 15 6 4
check no-match "0 9-17 * * *" 0 18 15 6 4

finish
