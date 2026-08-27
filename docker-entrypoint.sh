#!/bin/sh
#
# Prepares a WriteFreely container before handing off to the binary.
#
# The image ships no configuration, no keys and no schema, so a fresh
# container previously started, failed to find any of them, and exited --
# with the recovery steps living in a shell script that referenced
# container names and a Compose v1 binary that no longer exist.
#
# This script performs the steps that are safe to repeat on every start,
# and refuses to guess at the ones that are not.
#
#   WRITEFREELY_AUTO_MIGRATE   run pending migrations on start (default true)
#   WRITEFREELY_INIT_DB        create the schema on start (default false;
#                              set true for the very first start only)
#
set -eu

CONFIG_FILE="${WRITEFREELY_CONFIG:-config.ini}"
KEYS_DIR="${WRITEFREELY_KEYS_DIR:-keys}"
if [ "$#" -eq 0 ]; then
    echo "writefreely: entrypoint requires the binary as its first argument" >&2
    exit 2
fi
BIN="$1"

# A missing config is not something to invent. Guessing here would produce
# an instance pointed at the wrong database or serving the wrong host name,
# which is worse than a clear failure.
if [ ! -f "$CONFIG_FILE" ]; then
    echo "writefreely: no $CONFIG_FILE found." >&2
    echo "" >&2
    echo "Generate one interactively, then start the container again:" >&2
    echo "" >&2
    echo "  docker compose run --rm ${WRITEFREELY_SERVICE_HINT:-app} $BIN --config" >&2
    echo "" >&2
    echo "See docs/docker.md for the full first-run walkthrough." >&2
    exit 1
fi

# Generate keys only when they are absent. Regenerating them on an existing
# instance would invalidate every session and every encrypted value.
if [ ! -f "$KEYS_DIR/email.aes256" ]; then
    echo "writefreely: generating encryption keys"
    "$BIN" --gen-keys
fi

if [ "${WRITEFREELY_INIT_DB:-false}" = "true" ]; then
    echo "writefreely: initializing database schema"
    "$BIN" --init-db
fi

# Migrations record what they have applied, so this is a no-op once the
# database is current.
if [ "${WRITEFREELY_AUTO_MIGRATE:-true}" = "true" ]; then
    echo "writefreely: applying pending migrations"
    "$BIN" --migrate
fi

exec "$@"
