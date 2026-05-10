#!/bin/sh
# Periodic pg_dump backup with rotation. Runs in a loop inside a sidecar
# container; survival is up to compose's restart policy. The dump is written
# to BACKUP_DIR (mounted from the host so a `docker compose down -v` doesn't
# wipe it).
#
# Required: DATABASE_URL.
# Optional:
#   BACKUP_DIR        — output directory (default /backups)
#   BACKUP_INTERVAL   — seconds between runs (default 86400 = nightly)
#   BACKUP_RETENTION  — number of dumps to keep (default 14)
#   BACKUP_OFFSITE_CMD — if set, executed after each successful dump with the
#                        new file path as $1 (use to push to S3, B2, rsync, …)
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
BACKUP_INTERVAL="${BACKUP_INTERVAL:-86400}"
BACKUP_RETENTION="${BACKUP_RETENTION:-14}"

mkdir -p "$BACKUP_DIR"

while true; do
    ts="$(date -u +%Y%m%dT%H%M%SZ)"
    out="$BACKUP_DIR/openswiss-${ts}.sql.gz"
    tmp="${out}.tmp"
    echo "[$(date -u +%FT%TZ)] backup -> $out"
    if pg_dump --clean --if-exists --no-owner --no-privileges "$DATABASE_URL" | gzip -9 > "$tmp"; then
        mv "$tmp" "$out"
        # Rotate: keep the newest BACKUP_RETENTION files.
        ls -1t "$BACKUP_DIR"/openswiss-*.sql.gz 2>/dev/null \
            | tail -n +"$((BACKUP_RETENTION + 1))" \
            | xargs -r rm -f
        if [ -n "${BACKUP_OFFSITE_CMD:-}" ]; then
            sh -c "$BACKUP_OFFSITE_CMD \"$out\"" || \
                echo "[$(date -u +%FT%TZ)] offsite copy FAILED" >&2
        fi
    else
        rm -f "$tmp"
        echo "[$(date -u +%FT%TZ)] backup FAILED" >&2
    fi
    sleep "$BACKUP_INTERVAL"
done
