FROM alpine:3.21

RUN apk add --no-cache postgresql17-client python3 py3-pip bash \
 && pip3 install --break-system-packages linode-cli boto3

COPY backup.sh /usr/local/bin/backup.sh
RUN chmod +x /usr/local/bin/backup.sh

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["entrypoint.sh"]
