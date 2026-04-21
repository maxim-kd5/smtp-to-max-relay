package max

import (
	"fmt"
	"strings"
)

const (
	chatInfoReply = `Привет! Я бот для доставки сообщений в чат Max через email 📩

Как пользоваться:

• Отправь письмо на один из адресов:
— %[3]s@%[2]s — с уведомлением
— %[3]s.silent@%[2]s — без уведомления

• Всё, что ты напишешь в письме (тема и текст), появится в этом чате
• Отправителя можно указать любого — он отобразится в сообщении

Технические параметры (если настраиваешь отправку вручную):
SMTP: %[2]s
Порт: 25
Авторизация: не требуется (можно указать любые логин/пароль)

ID этого чата: %[1]s
Ваш ID: %[1]s

Примеры отправки:

PowerShell:
Send-MailMessage -SmtpServer "%[2]s" -Port 25 -From "test@test.loc" -To "%[3]s@%[2]s" -Subject "Test" -Body "Message text"

curl:
curl smtp://%[2]s:25 --mail-from "test@test.loc" --mail-rcpt "%[3]s@%[2]s" --upload-file - <<EOF
Subject: Test
From: test@test.loc
To: %[3]s@%[2]s

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

func ShouldReplyWithUserInfo(text string) bool {
	cmd := ExtractCommand(text)
	return cmd == "start" || cmd == "help"
}

func ShouldReplyHello(text string) bool {
	return ExtractCommand(text) == "hello"
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

	return fmt.Sprintf(
		chatInfoReply,
		id,
		domain,
		chatAddressLocalPart(id),
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

	return fmt.Sprintf(
		chatInfoReply,
		id,
		domain,
		chatAddressLocalPart(id),
	)
}

func chatAddressLocalPart(id string) string {
	value := strings.TrimSpace(id)
	if value == "" || value == "<unknown>" {
		return "chatidunknown"
	}
	return "chatid" + value
}
