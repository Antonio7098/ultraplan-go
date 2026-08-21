#!/usr/bin/env bash

set -euo pipefail

stop_ultraplan=false
assume_yes=false
keep_old=false

usage() {
  cat <<'EOF'
Usage: scripts/reset-opencode-db.sh [options]

Replace the OpenCode database with a clean database that has no sessions,
messages, parts, or retained session events.

Options:
  --stop-ultraplan  Stop UltraPlan servers before stopping OpenCode workers.
  --keep-old        Keep the old database after the replacement is verified.
  --yes             Skip the destructive confirmation prompt.
  -h, --help        Show this help.

The script preserves database-backed account and credential tables when their
schemas are compatible. It does not delete OpenCode logs, snapshots, tool
output, or configuration files.
EOF
}

while (($# > 0)); do
  case "$1" in
    --stop-ultraplan)
      stop_ultraplan=true
      ;;
    --keep-old)
      keep_old=true
      ;;
    --yes)
      assume_yes=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

for command_name in opencode sqlite3 pgrep pkill stat; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  fi
done

database_path="$(opencode db path)"
if [[ "$database_path" != /* || "$(basename "$database_path")" != "opencode.db" ]]; then
  printf 'Refusing unexpected OpenCode database path: %s\n' "$database_path" >&2
  exit 1
fi

database_dir="$(dirname "$database_path")"
if [[ ! -d "$database_dir" ]]; then
  printf 'OpenCode database directory does not exist: %s\n' "$database_dir" >&2
  exit 1
fi

if pgrep -x ultraplan >/dev/null 2>&1; then
  if [[ "$stop_ultraplan" != true ]]; then
    printf 'UltraPlan is running and may respawn OpenCode workers.\n' >&2
    printf 'Stop it first or rerun with --stop-ultraplan.\n' >&2
    pgrep -a -x ultraplan >&2 || true
    exit 1
  fi
fi

if [[ -f "$database_path" ]]; then
  database_bytes="$(stat --format='%s' "$database_path")"
else
  database_bytes=0
fi

printf 'OpenCode database: %s\n' "$database_path"
printf 'Current size: %s bytes\n' "$database_bytes"
printf 'This permanently deletes every OpenCode session and retained session event.\n'

if [[ "$assume_yes" != true ]]; then
  read -r -p 'Type DELETE to continue: ' confirmation
  if [[ "$confirmation" != "DELETE" ]]; then
    printf 'Cancelled.\n'
    exit 0
  fi
fi

if [[ "$stop_ultraplan" == true ]]; then
  pkill -x ultraplan 2>/dev/null || true
  sleep 2
  pkill -9 -x ultraplan 2>/dev/null || true
fi

pkill -x opencode 2>/dev/null || true
sleep 2
pkill -9 -x opencode 2>/dev/null || true

if pgrep -x opencode >/dev/null 2>&1; then
  printf 'OpenCode processes are still running; refusing database replacement.\n' >&2
  pgrep -a -x opencode >&2 || true
  exit 1
fi

if [[ ! -f "$database_path" ]]; then
  printf 'No existing database found. Creating a fresh one.\n'
  opencode db 'select 1' >/dev/null
  exit 0
fi

printf 'Checkpointing the existing database...\n'
sqlite3 "$database_path" 'PRAGMA busy_timeout=60000; PRAGMA wal_checkpoint(TRUNCATE);' >/dev/null

timestamp="$(date -u +'%Y%m%dT%H%M%SZ')"
old_path="${database_path}.sessions-purge-${timestamp}"
replacement_created=false

restore_old_database() {
  exit_code=$?
  if [[ "$replacement_created" == true && -f "$old_path" ]]; then
    printf 'Reset failed. Restoring the original database...\n' >&2
    rm -f "$database_path" "${database_path}-shm" "${database_path}-wal"
    mv "$old_path" "$database_path"
  fi
  exit "$exit_code"
}
trap restore_old_database ERR INT TERM

mv "$database_path" "$old_path"
rm -f "${database_path}-shm" "${database_path}-wal"
replacement_created=true

printf 'Creating a fresh OpenCode database...\n'
opencode db 'select 1' >/dev/null

printf 'Preserving compatible account and credential records...\n'
sqlite3 "$database_path" <<SQL
PRAGMA foreign_keys=OFF;
ATTACH DATABASE '$old_path' AS old;
BEGIN IMMEDIATE;
INSERT OR REPLACE INTO account SELECT * FROM old.account;
INSERT OR REPLACE INTO account_state SELECT * FROM old.account_state;
INSERT OR REPLACE INTO control_account SELECT * FROM old.control_account;
INSERT OR REPLACE INTO credential SELECT * FROM old.credential;
COMMIT;
DETACH DATABASE old;
SQL

quick_check="$(sqlite3 -readonly "$database_path" 'PRAGMA quick_check;')"
session_count="$(sqlite3 -readonly "$database_path" 'SELECT count(*) FROM session;')"
message_count="$(sqlite3 -readonly "$database_path" 'SELECT count(*) FROM message;')"
part_count="$(sqlite3 -readonly "$database_path" 'SELECT count(*) FROM part;')"
event_count="$(sqlite3 -readonly "$database_path" 'SELECT count(*) FROM event;')"

if [[ "$quick_check" != "ok" || "$session_count" != 0 || "$message_count" != 0 || "$part_count" != 0 || "$event_count" != 0 ]]; then
  printf 'Replacement validation failed. quick_check=%s sessions=%s messages=%s parts=%s events=%s\n' \
    "$quick_check" "$session_count" "$message_count" "$part_count" "$event_count" >&2
  false
fi

replacement_created=false
trap - ERR INT TERM

if [[ "$keep_old" == true ]]; then
  printf 'Old database retained at: %s\n' "$old_path"
else
  printf 'Removing the old database...\n'
  rm -f "$old_path" "${old_path}-shm" "${old_path}-wal"
fi

new_bytes="$(stat --format='%s' "$database_path")"
printf 'Reset complete.\n'
printf 'New database size: %s bytes\n' "$new_bytes"
printf 'Sessions: 0, messages: 0, parts: 0, events: 0\n'
if [[ "$stop_ultraplan" == true ]]; then
  printf 'UltraPlan was stopped and has not been restarted.\n'
fi
