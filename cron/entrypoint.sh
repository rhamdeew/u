#!/bin/sh
set -e

# Generate s3cmd config from environment variables
if [ -n "${S3_ACCESS_KEY:-}" ] && [ -n "${S3_SECRET_KEY:-}" ]; then
    cat > /root/.s3cfg <<EOF
[default]
access_key = ${S3_ACCESS_KEY}
secret_key = ${S3_SECRET_KEY}
host_base = ${S3_HOST}
host_bucket = ${S3_HOST}
bucket_location = ${S3_REGION}
use_https = True
EOF
    chmod 600 /root/.s3cfg
fi

echo "Cron container started. Backups scheduled for 07:00 UTC daily."

while true; do
    HOUR=$(date -u +%H)
    MIN=$(date -u +%M)
    if [ "$HOUR" = "07" ] && [ "$MIN" = "00" ]; then
        echo "=== Running scheduled backup at $(date -u) ==="
        /usr/local/bin/backup.sh || echo "Backup failed at $(date -u)"
        sleep 61
    fi
    sleep 30
done
