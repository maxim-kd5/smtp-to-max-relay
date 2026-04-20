package max

import (
	"fmt"
	"strings"
)

func ShouldReplyWithUserInfo(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	return t == "/start" || t == "/help" || t == "help" || t == "start"
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
		"Ваш ID: %s\n\nПримеры email для отправки в MAX:\n• %s@%s — в личный чат\n• %s!123@%s — в тред 123\n• %s.silent@%s — без уведомления",
		id,
		id, domain,
		id, domain,
		id, domain,
	)
}
