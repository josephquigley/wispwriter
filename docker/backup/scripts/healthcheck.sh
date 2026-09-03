#!/bin/bash
# Docker HEALTHCHECK. Healthy only when all three hold:
#   1. the scheduler loop is alive   (beacon touched within the alive window)
#   2. the last run succeeded        (health file starts with "ok ")
#   3. a run happened recently       (health file touched within the health window)
#
# A container that is up but has silently stopped backing anything up is the
# failure worth catching, and an "up" status alone does not catch it.
#
# The thresholds are environment-overridable so that changing BACKUP_SCHEDULE
# does not need a rebuild. Size BACKUP_HEALTH_MAX_AGE to the schedule plus
# grace: the default of 8 days suits a weekly cadence.
set -u

ALIVE_FILE="${ALIVE_FILE:-/tmp/backup-alive}"
HEALTH_FILE="${HEALTH_FILE:-/tmp/backup-health}"
ALIVE_MAX_AGE="${BACKUP_ALIVE_MAX_AGE:-2}"
HEALTH_MAX_AGE="${BACKUP_HEALTH_MAX_AGE:-11520}"

find "$ALIVE_FILE" -mmin -"$ALIVE_MAX_AGE" 2>/dev/null | grep -q . || exit 1
find "$HEALTH_FILE" -mmin -"$HEALTH_MAX_AGE" 2>/dev/null | grep -q . || exit 1
grep -q '^ok ' "$HEALTH_FILE" 2>/dev/null || exit 1

exit 0
