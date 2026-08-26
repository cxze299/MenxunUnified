#!/bin/sh
set -eu

ROOT="${MENXUN_ROOT:-/volume2/docker/menxun-unified}"
DATA_ROOT="${MENXUN_DATA_ROOT:-$ROOT/shared/data}"
BACKUP_ROOT="${MENXUN_BACKUP_ROOT:-$DATA_ROOT/backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
DAILY="$BACKUP_ROOT/daily"
MONTHLY="$BACKUP_ROOT/monthly"
STATUS="$BACKUP_ROOT/status.json"
mkdir -p "$DAILY" "$MONTHLY"
TMP="$DAILY/.menxun-$STAMP.sql.gz.tmp"
FINAL="$DAILY/menxun-$STAMP.sql.gz"

if /usr/local/bin/docker exec menxun-unified-db sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" exec mysqldump --single-transaction --quick --routines --triggers -u"$MYSQL_USER" "$MYSQL_DATABASE"' | gzip -9 > "$TMP"; then
  test -s "$TMP"
  mv "$TMP" "$FINAL"
else
  rm -f "$TMP"
  printf '{"ok":false,"finished_at":"%s","message":"mysqldump failed"}\n' "$(date -Iseconds)" > "$STATUS"
  exit 1
fi

find "$DAILY" -type f -name 'menxun-*.sql.gz' -mtime +13 -delete
if [ "$(date +%d)" = "01" ]; then
  cp "$FINAL" "$MONTHLY/menxun-$(date +%Y-%m).sql.gz"
  find "$MONTHLY" -type f -name 'menxun-*.sql.gz' -mtime +92 -delete
fi

ASSET_BYTES="$(du -sb "$DATA_ROOT/assets" 2>/dev/null | awk '{print $1}' || printf 0)"
DB_BYTES="$(wc -c < "$FINAL" | tr -d ' ')"
printf '{"ok":true,"finished_at":"%s","database_file":"%s","database_bytes":%s,"asset_bytes":%s}\n' \
  "$(date -Iseconds)" "$FINAL" "$DB_BYTES" "$ASSET_BYTES" > "$STATUS"
