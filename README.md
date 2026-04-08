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
- `MAX_BOT_TOKEN` (required for real sender, stub is used by default)

Current baseline uses a stub MAX sender and is ready for integration with `max-bot-api-client-go`.


Note: port `25` is the standard SMTP port for inter-server delivery. On Linux, binding to privileged ports (<1024) may require root or additional capabilities; for local development you can set `SMTP_LISTEN_ADDR=:2525`.
