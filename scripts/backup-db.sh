#!/usr/bin/env bash
# Dumps the Postgres database to backups/bidspot-<timestamp>.dump (pg_dump custom format) and
# keeps the newest KEEP dumps (default 14). Safe while the app runs, and works with it turned
# down too: only the db container is started, never the backend. For a nightly backup:
#
#   0 4 * * * cd /path/to/bidspot && scripts/backup-db.sh >> backups/backup.log 2>&1
#
# Restore into the running stack (replaces what is there):
#
#   docker compose exec -T db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < backups/bidspot-….dump
set -euo pipefail

cd "$(dirname "$0")/.."
keep=${KEEP:-14}
mkdir -p backups
chmod 700 backups
out="backups/bidspot-$(date +%Y%m%d-%H%M%S).dump"

# the stack may be down (a final export before moving to another server): bring up the database
# only. --no-recreate leaves a running container alone, e.g. one started with the dev overlay.
docker compose up -d --wait --no-recreate db >/dev/null

docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' >"$out.part"
mv "$out.part" "$out"
chmod 600 "$out"
echo "$(date '+%F %T') wrote $out ($(du -h "$out" | cut -f1))"

ls -1t backups/bidspot-*.dump | tail -n +$((keep + 1)) | xargs -r rm --
