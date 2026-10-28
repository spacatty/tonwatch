#!/usr/bin/env bash
# Loads a dump made by scripts/backup-db.sh into this stack's database, replacing what is there.
# This is how data moves between machines (dev → prod): back up on one, copy the file, restore on
# the other.
#
#   scripts/restore-db.sh backups/tonwatch-20261028-230636.dump           into an empty database
#   scripts/restore-db.sh backups/tonwatch-20261028-230636.dump --force   over one that has accounts
#
# If the database already holds names, a safety dump of it is taken first. The backend is stopped
# while the restore runs, which happens in one transaction: it lands whole or not at all.
set -euo pipefail

cd "$(dirname "$0")/.."
dump=${1:-}
force=${2:-}
if [[ -z $dump || ! -f $dump ]]; then
  echo "usage: scripts/restore-db.sh <file.dump> [--force]" >&2
  exit 1
fi

docker compose up -d --wait --no-recreate db >/dev/null

count() { # rows in a table, 0 when it does not exist yet
  docker compose exec -T db sh -c "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -tAc 'SELECT COUNT(*) FROM $1'" 2>/dev/null | tr -d '[:space:]' || true
}
users=$(count users)
names=$(count usernames)
users=${users:-0}
names=${names:-0}

if ((users > 0)) && [[ $force != --force ]]; then
  echo "This database already has $users account(s) and $names names. Run again with --force to replace them." >&2
  exit 1
fi
if ((names > 0)); then
  echo "Taking a safety dump of the current database first…"
  scripts/backup-db.sh
fi

backend_up=$(docker compose ps -q --status running backend || true)
[[ -n $backend_up ]] && docker compose stop backend >/dev/null

echo "Restoring $dump…"
docker compose exec -T db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error' <"$dump"
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -qc ANALYZE'

echo "Restored: $(count users) account(s), $(count usernames) names, $(count events) events."
if [[ -n $backend_up ]]; then
  docker compose start backend >/dev/null
  echo "Backend started again."
fi
