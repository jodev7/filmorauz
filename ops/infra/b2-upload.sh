#!/bin/bash
# Upload one DB backup archive to the private B2 bucket, then keep only the last 2 there.
# Uses a dedicated B2 auth profile so it never touches the default b2 CLI auth.
# Contains no secrets: authorize once with:
#   B2_ACCOUNT_INFO=/root/.config/b2/backup_account_info b2 account authorize <keyID> <appKey>
set -euo pipefail
ARCHIVE="${1:?usage: b2-upload.sh <archive>}"
BUCKET="${BACKUP_BUCKET:-your-private-backup-bucket}"
export B2_ACCOUNT_INFO=/root/.config/b2/backup_account_info
NAME="$(basename "$ARCHIVE")"
b2 file upload --no-progress "$BUCKET" "$ARCHIVE" "$NAME" >/dev/null
echo "$(date -Is) uploaded $NAME -> b2://$BUCKET"
# rotation: names embed a sortable timestamp; keep the newest 2, remove older
mapfile -t files < <(b2 ls "b2://$BUCKET/" 2>/dev/null | sort)
n=${#files[@]}
if (( n > 2 )); then
  for f in "${files[@]:0:n-2}"; do
    [ -n "$f" ] && b2 rm "b2://$BUCKET/$f" >/dev/null 2>&1 && echo "  removed old b2://$BUCKET/$f"
  done
fi
