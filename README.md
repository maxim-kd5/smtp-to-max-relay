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
- `SMTP_MAX_CONCURRENT_SESSIONS` (default `200`)
- `SMTP_ALLOWED_RCPT_DOMAIN` (default `relay.local`)
- `ALIAS_FILE_PATH` (default `./config/aliases.json`)
- `ACL_FILE_PATH` (default `./config/acl.json`)
- `ADMIN_CHAT_IDS` (optional; comma-separated bootstrap super-admin chat IDs, e.g. `100,-200`)
- `ADMIN_USER_IDS` (optional; comma-separated bootstrap super-admin user IDs)
- `MAX_SENDER_MODE` (`stub` by default, options: `stub`, `botapi`)
- `MAX_API_BASE_URL` (optional; when empty, the official MAX Bot API base URL is used)
- `MAX_BOT_TOKEN` (required when `MAX_SENDER_MODE` is not `stub`)
- `RELAY_MAX_RETRIES` (default `2`)
- `RELAY_RETRY_DELAY_MS` (default `300`)
- `METRICS_LISTEN_ADDR` (default `:9090`, set empty to disable)
- `DLQ_ENABLED` (default `true`; persist failed deliveries for replay)
- `DLQ_FILE_PATH` (default `./data/dlq.json`)
- `DLQ_WORKER_INTERVAL_MS` (default `2000`)
- `DLQ_MAX_RETRIES` (default `10`)
- `DLQ_BASE_DELAY_MS` (default `1000`)
- `DLQ_MAX_DELAY_MS` (default `300000`)

`MAX_SENDER_MODE=botapi` uses `github.com/max-messenger/max-bot-api-client-go` for outgoing MAX messages, file uploads, and long polling bot updates.

Recipient format:

- `chatid<chat-id>@<domain>` sends to a chat with notifications enabled
- `chatid<chat-id>.silent@<domain>` sends to a chat without notifications
- `chatid-<abs-chat-id>@<domain>` is the uniform format for negative chat IDs
- Thread-style recipients like `<chat-id>!<thread-id>@<domain>` are not supported because MAX does not have message threads

Note: port `25` is the standard SMTP port for inter-server delivery. On Linux, binding to privileged ports (<1024) may require root or additional capabilities; for local development you can set `SMTP_LISTEN_ADDR=:2525`.

SMTP AUTH: relay mode does not require authentication. If a client attempts `AUTH PLAIN`, any username/password is accepted.

SMTP server does not perform outgoing SMTP delivery and does not forward emails to external recipient domains; it only converts accepted inbound messages to MAX sends.

When delivery to MAX fails after immediate retries, the raw message is saved to DLQ and retried in background with exponential backoff.

Метрики включают счётчики принятых/успешных/ошибочных сообщений и детализацию пересылок:
`smtp_relay_delivery_total{address,delivered,max_recipient_id,max_recipient_name}`.
`max_recipient_name` — локальная часть исходного SMTP-адреса (например alias или `chatid...`).
Для DLQ дополнительно экспортируются `smtp_relay_dlq_pending`, `smtp_relay_dlq_failed`, `smtp_relay_dlq_done`.
Также публикуется гистограмма задержек `smtp_relay_latency_seconds` со stage:
`email_parse`, `max_send`, `relay_total`.

When `MAX_SENDER_MODE=botapi`, the service also receives bot updates and replies to:

- `/start` with the user's personal relay address and MAX user ID
- `/hello`, `/help`, or bot mentions in chat with the relay address of the current chat
- ACL-protected admin commands (roles: `super_admin`, `alias_admin`, `dlq_admin`, `stats_viewer`):
  - `/alias <name> <chatid...|number>` — add/update alias (число автоматически преобразуется в `chatid<number>`)
  - `/unalias <name>` — remove alias
  - `/aliases` — список алиасов
  - `/stats7d` — отправить статистику relay за последние 7 дней
  - `/stats30d` — отправить статистику relay за последние 30 дней
  - `/dlq` — показать текущий backlog DLQ
  - `/dlq_list <limit>` — показать последние элементы DLQ
  - `/replay <id>` — вручную выполнить replay конкретного элемента DLQ
  - `/grant <role> <user|chat> <id>` — выдать роль (только `super_admin`)
  - `/revoke <role> <user|chat> <id>` — отозвать роль (только `super_admin`)
  - `/whoami` — показать роли текущего пользователя/чата (только `super_admin`)

Пример:
- `/alias admin 260920412` сохранится как `admin -> chatid260920412`


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

CI also pushes container images to GHCR on non-PR runs with tags:
- `sha-<commit>` for each commit
- `dev` for each commit
- `latest` for `main`

## Bot versioning

Bot version format is `0.2.<build-number>` (or `0.2.<build-number>-<suffix>` for suffix builds like `dev`).

- `0.2` — fixed major/minor train
- `<build-number>` — commit counter (`git rev-list --count HEAD`), injected automatically during container build/CI.
