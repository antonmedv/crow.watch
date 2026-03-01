FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/backup ./cmd/backup

FROM alpine:3.21

RUN apk add --no-cache ca-certificates postgresql17-client \
    && addgroup -S app \
    && adduser -S -G app -H -D app

COPY --from=build /bin/backup /bin/backup

RUN mkdir /data && chown app:app /data

USER app

EXPOSE 8080

ENTRYPOINT ["/bin/backup"]
