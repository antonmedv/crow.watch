#!/bin/bash
set -euo pipefail

TIMESTAMP="$(date +%Y-%m-%d_%H%M%S)"
FILENAME="crow_watch_${TIMESTAMP}.sql.gz"
TMPFILE="/tmp/${FILENAME}"
S3_PREFIX="${S3_PREFIX:-backups}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
S3_DEST="${S3_PREFIX}/${FILENAME}"
S3="/usr/local/bin/s3.py"

# --- 1. Dump ---
echo "backup: dumping database to ${FILENAME}..."
pg_dump | gzip > "${TMPFILE}"
echo "backup: dump complete ($(du -h "${TMPFILE}" | cut -f1))"

# --- 2. Upload ---
echo "backup: uploading ${S3_DEST} to ${LINODE_BUCKET}..."
python3 "${S3}" put "${TMPFILE}" "${LINODE_BUCKET}" "${S3_DEST}"
echo "backup: upload complete"
rm -f "${TMPFILE}"

# --- 3. Cleanup ---
echo "backup: checking for backups older than ${BACKUP_RETENTION_DAYS} days..."
CUTOFF="$(date -d "-${BACKUP_RETENTION_DAYS} days" +%Y-%m-%d 2>/dev/null || date -v-${BACKUP_RETENTION_DAYS}d +%Y-%m-%d)"

python3 "${S3}" ls "${LINODE_BUCKET}" "${S3_PREFIX}/" | while read -r file; do
  file_date="$(echo "${file}" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)"
  if [ -z "${file_date}" ]; then
    continue
  fi
  if [ "${file_date}" \< "${CUTOFF}" ]; then
    echo "backup: deleting expired backup ${file}..."
    python3 "${S3}" rm "${LINODE_BUCKET}" "${file}" || echo "backup: warning: failed to delete ${file}"
  fi
done || echo "backup: warning: cleanup listing failed, skipping"

echo "backup: done"
