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
- `ADMIN_CHAT_ID` (optional; MAX chat ID пользователя с админским доступом; в этом чате доступны админ-команды)
- `MAX_SENDER_MODE` (`stub` by default, options: `stub`, `botapi`)
- `MAX_API_BASE_URL` (optional; when empty, the official MAX Bot API base URL is used)
- `MAX_BOT_TOKEN` (required when `MAX_SENDER_MODE` is not `stub`)
- `RELAY_MAX_RETRIES` (default `2`)
- `RELAY_RETRY_DELAY_MS` (default `300`)
- `METRICS_LISTEN_ADDR` (default `:9090`, set empty to disable)

`MAX_SENDER_MODE=botapi` uses `github.com/max-messenger/max-bot-api-client-go` for outgoing MAX messages, file uploads, and long polling bot updates.

Recipient format:

- `chatid<chat-id>@<domain>` sends to a chat with notifications enabled
- `chatid<chat-id>.silent@<domain>` sends to a chat without notifications
- `chatid-<abs-chat-id>@<domain>` is the uniform format for negative chat IDs
- Thread-style recipients like `<chat-id>!<thread-id>@<domain>` are not supported because MAX does not have message threads

Note: port `25` is the standard SMTP port for inter-server delivery. On Linux, binding to privileged ports (<1024) may require root or additional capabilities; for local development you can set `SMTP_LISTEN_ADDR=:2525`.

SMTP AUTH: relay mode does not require authentication. If a client attempts `AUTH PLAIN`, any username/password is accepted.

SMTP server does not perform outgoing SMTP delivery and does not forward emails to external recipient domains; it only converts accepted inbound messages to MAX sends.

Метрики включают счётчики принятых/успешных/ошибочных сообщений и детализацию пересылок:
`smtp_relay_delivery_total{address,delivered,max_recipient_id,max_recipient_name}`.
`max_recipient_name` — локальная часть исходного SMTP-адреса (например alias или `chatid...`).

When `MAX_SENDER_MODE=botapi`, the service also receives bot updates and replies to:

- `/start` with the user's personal relay address and MAX user ID
- `/hello`, `/help`, or bot mentions in chat with the relay address of the current chat
- admin-only alias commands in chat configured by `ADMIN_CHAT_ID`:
  - `/alias <name> <chatid...>` — add/update alias
  - `/unalias <name>` — remove alias
  - `/stats7d` — отправить статистику relay за последние 7 дней


## Docker

Build image locally:

```bash
docker build -t smtp-to-max-relay:local .
```

Run with Docker Compose:

```bash
docker compose up -d --build
```

In the provided compose example SMTP is exposed as `25:2525` (host port 25 -> container port 2525).

Example compose file is available at `docker-compose.yml`.

CI also pushes container images to GHCR on non-PR runs with tags `sha-<commit>` and `latest` (for `main`).
