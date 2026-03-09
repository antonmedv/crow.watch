FROM golang:1.25.8-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN mkdir -p /out && for cmd in cmd/*/; do \
      name=$(basename "$cmd"); \
      CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/"$name" ./"$cmd"; \
    done

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/ /bin/
