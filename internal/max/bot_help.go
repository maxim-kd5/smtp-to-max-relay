package max

import (
	"fmt"
	"strings"
)

const (
	infoReplyTemplate = `Привет! Я бот для доставки сообщений в MAX через email 📩

%[1]s

Отправь письмо на один из адресов:
- %[2]s@%[3]s
- %[2]s.silent@%[3]s

%[4]s: %[5]s

И я перешлю сообщение сюда. 

Я могу слать сообщения как напрямую тебе так и в групповой чат, для этого нужно добавить меня администратором в групповой чат с доступом к чтению сообщений, после чего в чат отправить сообщение /help, затем я пришлю информацию с email после меня можно из администраторов чата убрать.

Команды бота:
- /start — показать персональный адрес
- /hello или /help — показать адрес текущего чата
- /alias <name> <chatid...> — (admin) добавить/обновить alias
- /unalias <name> — (admin) удалить alias
- /stats7d — (admin) статистика relay за 7 дней

Технические параметры (если настраиваешь отправку вручную):
SMTP: %[3]s
Порт: 25
Авторизация: не требуется (можно указать любые логин/пароль)

Примеры отправки:

PowerShell:
Send-MailMessage -SmtpServer "%[3]s" -Port 25 -From "test@test.loc" -To "%[2]s@%[3]s" -Subject "Test" -Body "Message text"

curl:
curl smtp://%[3]s:25 --mail-from "test@test.loc" --mail-rcpt "%[2]s@%[3]s" --upload-file - <<EOF
Subject: Test
From: test@test.loc
To: %[2]s@%[3]s

Message text
EOF

Используй для уведомлений, интеграций и автоматических сообщений.
`
)

func ExtractCommandParts(text string) (command string, target string) {
	t := strings.TrimSpace(strings.ToLower(text))
	if t == "" {
		return "", ""
	}

	cmd := strings.Fields(t)[0]
	if !strings.HasPrefix(cmd, "/") {
		return "", ""
	}

	cmd = strings.TrimPrefix(cmd, "/")
	if at := strings.Index(cmd, "@"); at >= 0 {
		target = normalizeBotUsername(cmd[at+1:])
		cmd = cmd[:at]
	}
	return cmd, target
}

func ExtractCommand(text string) string {
	cmd, _ := ExtractCommandParts(text)
	return cmd
}

func CommandTargetsBot(text, botUsername string) bool {
	_, target := ExtractCommandParts(text)
	return target == "" || target == normalizeBotUsername(botUsername)
}

func MessageMentionsBot(text, botUsername string) bool {
	username := normalizeBotUsername(botUsername)
	if username == "" {
		return false
	}

	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}

	return strings.Contains(t, "@"+username) ||
		strings.HasPrefix(t, username+" ") ||
		strings.HasPrefix(t, username+",") ||
		strings.HasPrefix(t, username+":")
}

func normalizeBotUsername(username string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.ToLower(username)), "@")
}

func BuildUserInfoReply(userID, allowedDomain string) string {
	id := strings.TrimSpace(userID)
	domain := strings.TrimSpace(allowedDomain)
	if id == "" {
		id = "<unknown>"
	}
	if domain == "" {
		domain = "relay.local"
	}

	return buildInfoReply(
		"Твой личный адрес для доставки сообщений через relay:",
		chatAddressLocalPart(id),
		domain,
		"Ваш ID",
		id,
	)
}

func BuildChatInfoReply(chatID, allowedDomain string) string {
	id := strings.TrimSpace(chatID)
	domain := strings.TrimSpace(allowedDomain)
	if id == "" {
		id = "<unknown>"
	}
	if domain == "" {
		domain = "relay.local"
	}

	return buildInfoReply(
		"Адрес этого чата для доставки сообщений через relay:",
		chatAddressLocalPart(id),
		domain,
		"ID этого чата",
		id,
	)
}

func chatAddressLocalPart(id string) string {
	value := strings.TrimSpace(id)
	if value == "" || value == "<unknown>" {
		return "chatidunknown"
	}
	return "chatid" + value
}

func buildInfoReply(title, localPart, domain, idLabel, idValue string) string {
	return fmt.Sprintf(infoReplyTemplate, title, localPart, domain, idLabel, idValue)
}
