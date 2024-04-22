FROM golang:1.22-alpine AS golang

WORKDIR /app

ENV SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/v0.2.29/supercronic-linux-amd64 \
    SUPERCRONIC=supercronic-linux-amd64 \
    SUPERCRONIC_SHA1SUM=cd48d45c4b10f3f0bfdd3a57d054cd05ac96812b

RUN apk --no-cache add curl

RUN curl -fsSLO "$SUPERCRONIC_URL" \
    && echo "${SUPERCRONIC_SHA1SUM}  ${SUPERCRONIC}" | sha1sum -c - \
    && chmod +x "$SUPERCRONIC"

COPY . .

RUN go mod download
RUN go mod verify

RUN go build -o bin/go-luxpower-timescaledb cmd/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=golang /app/supercronic-linux-amd64 supercronic
COPY --from=golang /app/bin/go-luxpower-timescaledb .
COPY --from=golang /app/crontab .

RUN chmod +x supercronic

CMD ["/app/supercronic", "/app/crontab"]
