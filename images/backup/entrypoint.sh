#!/bin/bash
set -euo pipefail

BACKUP_SCHEDULE="${BACKUP_SCHEDULE:-0 3 * * *}"

# Dump all env vars into a file that cron jobs can source,
# since cron does not inherit the container's environment.
env | sed 's/^\(.*\)$/export \1/g' > /etc/backup.env

# Write the crontab entry
echo "${BACKUP_SCHEDULE} . /etc/backup.env; /usr/local/bin/backup.sh >> /proc/1/fd/1 2>&1" \
  | crontab -

echo "backup: schedule set to '${BACKUP_SCHEDULE}'"
echo "backup: running immediate backup to verify configuration..."

# Run an immediate backup so failures surface at deploy time.
/usr/local/bin/backup.sh

echo "backup: starting crond..."
exec crond -f
