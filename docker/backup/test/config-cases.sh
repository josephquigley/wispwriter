#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")" || exit 1
# shellcheck source=./assert.sh
source ./assert.sh
# shellcheck source=../scripts/config.sh
source ../scripts/config.sh

load_config fixtures/sqlite.ini
assert_eq "$DB_TYPE" "sqlite3" "sqlite: type"
assert_eq "$DB_FILENAME" "/data/writefreely.db" "sqlite: filename"

load_config fixtures/mysql.ini
assert_eq "$DB_TYPE" "mysql" "mysql: type"
assert_eq "$DB_USER" "writefreely" "mysql: username"
assert_eq "$DB_PASSWORD" "s3cret" "mysql: password"
assert_eq "$DB_HOST" "db" "mysql: host"
assert_eq "$DB_PORT" "3306" "mysql: port"
assert_eq "$DB_NAME" "writefreely" "mysql: database"

export WF_TEST_DB_PASSWORD="from-the-environment"
load_config fixtures/envref.ini
assert_eq "$DB_PASSWORD" "from-the-environment" "envref: resolved"

unset WF_TEST_DB_PASSWORD
assert_fails "envref: unset variable fails loudly" load_config fixtures/envref.ini

assert_fails "missing [database] section fails" load_config fixtures/no-database-section.ini
assert_fails "missing file fails" load_config fixtures/does-not-exist.ini

load_config fixtures/awkward-password.ini
assert_eq "$DB_PASSWORD" 'a b#c d' "awkward password survives intact"

finish
