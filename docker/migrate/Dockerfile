FROM golang:1.25-alpine AS build

RUN go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

FROM alpine:3.21

COPY --from=build /go/bin/goose /usr/local/bin/goose
COPY db/migrations /migrations

ENTRYPOINT ["goose"]
