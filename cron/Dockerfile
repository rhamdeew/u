FROM alpine:3.20

RUN apk add --no-cache \
    s3cmd \
    bash \
    tar \
    coreutils

COPY backup.sh /usr/local/bin/backup.sh
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/backup.sh /usr/local/bin/entrypoint.sh

RUN mkdir -p /backups

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
