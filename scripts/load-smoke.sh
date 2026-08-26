#!/bin/sh
set -eu

BASE_URL="${1:-http://127.0.0.1:2980}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
i=1
while [ "$i" -le 30 ]; do
  (curl -fsS --max-time 10 "$BASE_URL/api/health" > "$TMP/$i.out") &
  i=$((i + 1))
done
wait
test "$(find "$TMP" -type f -size +0c | wc -l | tr -d ' ')" -eq 30
printf 'load_smoke_ok concurrent_requests=30\n'
