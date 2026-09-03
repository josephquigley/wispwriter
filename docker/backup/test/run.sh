#!/bin/bash
# Runs every *-cases.sh in this directory. Each case file is independent and
# exits non-zero on any failed assertion.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

failed=0
for case_file in ./*-cases.sh; do
    echo "=== $case_file"
    if ! bash "$case_file"; then
        failed=1
    fi
done

if [[ "$failed" -ne 0 ]]; then
    echo "SUITE FAILED"
    exit 1
fi
echo "SUITE PASSED"
