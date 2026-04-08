# smtp-to-max-relay

SMTP relay service that receives email and forwards it to MAX messenger chats.

## Quick start

```bash
go mod tidy
go run ./cmd/smtp-to-max-relay
```

Environment variables:

- `SMTP_LISTEN_ADDR` (default `:25`)
- `SMTP_MAX_MESSAGE_BYTES` (default `15728640`)
- `SMTP_ALLOWED_RCPT_DOMAIN` (default `relay.local`)
- `ALIAS_FILE_PATH` (default `./config/aliases.json`)
- `MAX_SENDER_MODE` (`stub` by default, options: `stub`, `http`)
- `MAX_API_BASE_URL` (required when `MAX_SENDER_MODE=http`)
- `MAX_BOT_TOKEN` (required when `MAX_SENDER_MODE=http`)
- `RELAY_MAX_RETRIES` (default `2`)
- `RELAY_RETRY_DELAY_MS` (default `300`)
- `METRICS_LISTEN_ADDR` (default `:9090`, set empty to disable)

Current baseline uses a stub MAX sender and is ready for integration with `max-bot-api-client-go`.


Note: port `25` is the standard SMTP port for inter-server delivery. On Linux, binding to privileged ports (<1024) may require root or additional capabilities; for local development you can set `SMTP_LISTEN_ADDR=:2525`.


SMTP AUTH: relay mode does not require authentication. If a client attempts `AUTH PLAIN`, any username/password is accepted.


SMTP server does not perform outgoing SMTP delivery and does not forward emails to external recipient domains; it only converts accepted inbound messages to MAX sends.
