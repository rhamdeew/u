#!/bin/sh
set -eu

BACKUP_DIR="/backups"
DATE=$(date +%Y-%m-%d)
DAY_OF_WEEK=$(date +%u)   # 1=Mon .. 7=Sun
DAY_OF_MONTH=$(date +%d)

echo "=== Backup started at $(date) ==="

# --- Create archive ---
ARCHIVE_NAME="backup-${DATE}.tar.gz"
tar czf "${BACKUP_DIR}/${ARCHIVE_NAME}" \
    -C /data u.db || {
    echo "ERROR: Failed to create archive ${ARCHIVE_NAME}"
    exit 1
}
echo "Created ${ARCHIVE_NAME}"

# --- Rotation: 1 monthly, 2 weekly (Mon), 3 daily ---

MONTHLY_COUNT=0
WEEKLY_COUNT=0

BACKUPS=$(ls -1t "${BACKUP_DIR}"/backup-*.tar.gz 2>/dev/null || true)

for f in ${BACKUPS}; do
    FILENAME=$(basename "$f")
    FILE_DATE=$(echo "$FILENAME" | sed 's/backup-\([0-9-]*\)\.tar\.gz/\1/')
    FILE_DAY_OF_MONTH=$(echo "$FILE_DATE" | cut -d'-' -f3)
    FILE_DAY_OF_WEEK=$(date -d "$FILE_DATE" +%u 2>/dev/null || echo "7")

    if [ "$FILE_DAY_OF_MONTH" = "01" ]; then
        MONTHLY_COUNT=$((MONTHLY_COUNT + 1))
        if [ "$MONTHLY_COUNT" -gt 1 ]; then
            echo "Removing old monthly: ${FILENAME}"
            rm -f "$f"
        fi
    elif [ "$FILE_DAY_OF_WEEK" = "1" ]; then
        WEEKLY_COUNT=$((WEEKLY_COUNT + 1))
        if [ "$WEEKLY_COUNT" -gt 2 ]; then
            echo "Removing old weekly: ${FILENAME}"
            rm -f "$f"
        fi
    fi
done

# Keep only 3 most recent daily backups
DAILY_COUNT=0
for f in $(ls -1t "${BACKUP_DIR}"/backup-*.tar.gz 2>/dev/null || true); do
    FILENAME=$(basename "$f")
    FILE_DATE=$(echo "$FILENAME" | sed 's/backup-\([0-9-]*\)\.tar\.gz/\1/')
    FILE_DAY_OF_MONTH=$(echo "$FILE_DATE" | cut -d'-' -f3)
    FILE_DAY_OF_WEEK=$(date -d "$FILE_DATE" +%u 2>/dev/null || echo "7")

    if [ "$FILE_DAY_OF_MONTH" = "01" ] || [ "$FILE_DAY_OF_WEEK" = "1" ]; then
        continue
    fi

    DAILY_COUNT=$((DAILY_COUNT + 1))
    if [ "$DAILY_COUNT" -gt 3 ]; then
        echo "Removing old daily: ${FILENAME}"
        rm -f "$f"
    fi
done

echo "Local rotation complete."

# --- S3 Sync ---
if [ -z "${S3_BUCKET:-}" ]; then
    echo "S3_BUCKET not set, skipping S3 sync."
else
    echo "Syncing backups to S3..."
    s3cmd --config /root/.s3cfg sync "${BACKUP_DIR}/" "s3://${S3_BUCKET}/backups/" --delete-removed
    echo "S3 sync complete."
fi

echo "=== Backup finished at $(date) ==="
