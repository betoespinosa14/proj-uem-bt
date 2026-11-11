#!/bin/bash
# Nightly backup of the live BEACON database, run by the com.beacon.backup
# launchd job (02:30 daily, and once after each deploy).
#
# Writes ~/Library/Application Support/BEACON/backups/uem-YYYY-MM-DD.dump in
# pg_dump's custom format and keeps the last 14 days. Restore with:
#   pg_restore -h localhost -U uem --clean --if-exists -d uem <file>
# (stop com.beacon.server first: sudo launchctl bootout system/com.beacon.server)
set -euo pipefail

BIN=/opt/homebrew/opt/postgresql@16/bin
DIR="$HOME/Library/Application Support/BEACON/backups"
KEEP_DAYS=14

umask 077
mkdir -p "$DIR"
out="$DIR/uem-$(date +%Y-%m-%d).dump"
tmp="$DIR/.uem-in-progress.dump"

echo "$(date '+%F %T') backup starting"
# Written under a temporary name and renamed when complete, so a dump that was
# cut short never looks like a good one.
"$BIN/pg_dump" -h localhost -U uem -w -Fc -f "$tmp" uem
"$BIN/pg_restore" --list "$tmp" >/dev/null
mv -f "$tmp" "$out"
echo "$(date '+%F %T') backup written: $out ($(du -h "$out" | cut -f1))"

find "$DIR" -name 'uem-*.dump' -mtime +$KEEP_DAYS -print -delete | sed 's/^/removed old backup: /'
