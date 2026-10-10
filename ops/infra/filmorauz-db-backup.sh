#!/bin/bash
# Local MongoDB daily backup. B2 upload is appended once a private bucket+key is configured.
set -euo pipefail
. /root/atlas-migrate/creds.env
BDIR=/root/db-backups
mkdir -p "$BDIR"
TS=$(date +%Y%m%d-%H%M%S)
F="$BDIR/filmorauz-$TS.archive.gz"
mongodump --uri="$ROOT_URI" --db=filmorauz --archive="$F" --gzip
echo "$(date -Is) dumped $F ($(du -h "$F" | cut -f1))"
# keep only the last 2 local archives
ls -1t "$BDIR"/filmorauz-*.archive.gz 2>/dev/null | tail -n +3 | xargs -r rm -f
# B2 upload hook (created when the private bucket + key are provided): uploads then rotates B2 to keep last 2
if [ -x /root/db-backups/b2-upload.sh ]; then /root/db-backups/b2-upload.sh "$F" || echo "WARN: b2 upload failed"; fi
