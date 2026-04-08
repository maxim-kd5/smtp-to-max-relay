# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/smtp-to-max-relay ./cmd/smtp-to-max-relay

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/smtp-to-max-relay /app/smtp-to-max-relay
COPY --from=builder /src/config/aliases.json /app/config/aliases.json

ENV SMTP_LISTEN_ADDR=:2525 \
    SMTP_ALLOWED_RCPT_DOMAIN=relay.local \
    ALIAS_FILE_PATH=/app/config/aliases.json \
    MAX_SENDER_MODE=stub \
    METRICS_LISTEN_ADDR=:9090

EXPOSE 2525 9090
ENTRYPOINT ["/app/smtp-to-max-relay"]
