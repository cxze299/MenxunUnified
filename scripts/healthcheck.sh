#!/bin/sh
set -eu

ROOT="${MENXUN_ROOT:-/volume2/docker/menxun-unified}"
DATA_ROOT="${MENXUN_DATA_ROOT:-$ROOT/shared/data}"
PORT="${AGP_WEB_PORT:-2980}"
curl -fsS --max-time 8 "http://127.0.0.1:$PORT/api/health" >/dev/null
/usr/local/bin/docker exec menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysqladmin -u"$MYSQL_USER" ping --silent' >/dev/null
AVAILABLE_KB="$(df -Pk "$ROOT" | awk 'NR==2 {print $4}')"
TOTAL_KB="$(df -Pk "$ROOT" | awk 'NR==2 {print $2}')"
test "$AVAILABLE_KB" -gt $((TOTAL_KB * 15 / 100))
STATUS="$DATA_ROOT/backups/status.json"
test -s "$STATUS"
NOW="$(date +%s)"
MTIME="$(stat -c %Y "$STATUS")"
test $((NOW - MTIME)) -lt 129600
printf 'healthcheck_ok available_kb=%s backup_age_seconds=%s\n' "$AVAILABLE_KB" "$((NOW - MTIME))"
