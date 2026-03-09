FROM alpine:3.21

COPY web/static /static

ENTRYPOINT ["cp", "-r", "/static/.", "/out/"]
