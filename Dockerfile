# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /src
ARG BUILD_NUMBER=0

COPY go.mod go.sum ./
COPY third_party/max-bot-api-client-go ./third_party/max-bot-api-client-go
RUN go mod download

COPY . .
RUN if [ "$BUILD_NUMBER" = "0" ]; then BUILD_NUMBER="$(git rev-list --count HEAD 2>/dev/null || echo 0)"; fi && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -ldflags "-X smtp-to-max-relay/internal/version.BuildNumber=${BUILD_NUMBER}" \
      -o /out/smtp-to-max-relay ./cmd/smtp-to-max-relay

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
