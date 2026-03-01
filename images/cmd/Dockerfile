FROM golang:1.25-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/useradm ./cmd/useradm \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/votecalc ./cmd/votecalc \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/tokengen ./cmd/tokengen

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /bin/useradm /bin/useradm
COPY --from=build /bin/votecalc /bin/votecalc
COPY --from=build /bin/tokengen /bin/tokengen
