#!/bin/sh
set -eu

ROOT="${MENXUN_ROOT:-/volume2/docker/menxun-unified}"
DATA_ROOT="${MENXUN_DATA_ROOT:-$ROOT/shared/data}"
BACKUP_ROOT="${MENXUN_BACKUP_ROOT:-$DATA_ROOT/backups}"
BACKUP_FILE="${1:-$(find "$BACKUP_ROOT/daily" -type f -name 'menxun-*.sql.gz' | sort | tail -1)}"
test -n "$BACKUP_FILE" && test -s "$BACKUP_FILE"
DRILL_DB="menxun_restore_drill_$(date +%Y%m%d%H%M%S)"
cleanup() {
  /usr/local/bin/docker exec menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -e "DROP DATABASE IF EXISTS `'$DRILL_DB'`"' >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
/usr/local/bin/docker exec menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -e "CREATE DATABASE `'$DRILL_DB'` CHARACTER SET utf8mb4"'
gzip -dc "$BACKUP_FILE" | /usr/local/bin/docker exec -i menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot "'$DRILL_DB'"'
COUNTS="$(/usr/local/bin/docker exec menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -Nse "SELECT CONCAT((SELECT COUNT(*) FROM `'$DRILL_DB'`.users), '\''|'\'', (SELECT COUNT(*) FROM `'$DRILL_DB'`.study_groups), '\''|'\'', (SELECT COUNT(*) FROM `'$DRILL_DB'`.checkin_records))"')"
printf 'restore_drill_ok backup=%s counts=%s\n' "$BACKUP_FILE" "$COUNTS"
