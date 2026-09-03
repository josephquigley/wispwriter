#!/bin/bash
# Minimal assertion helpers. Sourced by every *-cases.sh script.
set -uo pipefail

ASSERT_PASS=0
ASSERT_FAIL=0

assert_eq() {
    local actual="$1" expected="$2" name="$3"
    if [[ "$actual" == "$expected" ]]; then
        echo "PASS: $name"
        ASSERT_PASS=$((ASSERT_PASS + 1))
    else
        echo "FAIL: $name"
        echo "        expected: [$expected]"
        echo "        actual:   [$actual]"
        ASSERT_FAIL=$((ASSERT_FAIL + 1))
    fi
}

assert_contains() {
    local haystack="$1" needle="$2" name="$3"
    case "$haystack" in
        *"$needle"*)
            echo "PASS: $name"
            ASSERT_PASS=$((ASSERT_PASS + 1)) ;;
        *)
            echo "FAIL: $name"
            echo "        expected to contain: [$needle]"
            echo "        actual:              [$haystack]"
            ASSERT_FAIL=$((ASSERT_FAIL + 1)) ;;
    esac
}

assert_not_contains() {
    local haystack="$1" needle="$2" name="$3"
    case "$haystack" in
        *"$needle"*)
            echo "FAIL: $name"
            echo "        expected NOT to contain: [$needle]"
            ASSERT_FAIL=$((ASSERT_FAIL + 1)) ;;
        *)
            echo "PASS: $name"
            ASSERT_PASS=$((ASSERT_PASS + 1)) ;;
    esac
}

# Passes when the command exits non-zero.
assert_fails() {
    local name="$1"; shift
    if "$@" >/dev/null 2>&1; then
        echo "FAIL: $name (command unexpectedly succeeded)"
        ASSERT_FAIL=$((ASSERT_FAIL + 1))
    else
        echo "PASS: $name"
        ASSERT_PASS=$((ASSERT_PASS + 1))
    fi
}

finish() {
    echo "--- $ASSERT_PASS passed, $ASSERT_FAIL failed"
    [[ "$ASSERT_FAIL" -eq 0 ]]
}
