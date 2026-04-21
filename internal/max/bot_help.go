package max

import (
	"fmt"
	"strings"
)

const (
	greetingReply = "\u041f\u0440\u0438\u0432\u0435\u0442! \u2728"
	chatInfoReply = "ID \u044d\u0442\u043e\u0433\u043e \u0447\u0430\u0442\u0430: %s\n\n\u041f\u0440\u0438\u043c\u0435\u0440\u044b email \u0434\u043b\u044f \u043e\u0442\u043f\u0440\u0430\u0432\u043a\u0438 \u0432 \u044d\u0442\u043e\u0442 \u0447\u0430\u0442:\n%s@%s \u2014 \u0432 \u043b\u0438\u0447\u043d\u044b\u0439 \u0447\u0430\u0442\n\n%s.silent@%s \u2014 \u0431\u0435\u0437 \u0443\u0432\u0435\u0434\u043e\u043c\u043b\u0435\u043d\u0438\u044f"
	userInfoReply = "\u0412\u0430\u0448 ID: %s\n\n\u041f\u0440\u0438\u043c\u0435\u0440\u044b email \u0434\u043b\u044f \u043e\u0442\u043f\u0440\u0430\u0432\u043a\u0438 \u0432 MAX:\n- %s@%s \u2014 \u0432 \u043b\u0438\u0447\u043d\u044b\u0439 \u0447\u0430\u0442\n- %s!123@%s \u2014 \u0432 \u0442\u0440\u0435\u0434 123\n- %s.silent@%s \u2014 \u0431\u0435\u0437 \u0443\u0432\u0435\u0434\u043e\u043c\u043b\u0435\u043d\u0438\u044f"
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
		userInfoReply,
		id,
		id, domain,
		id, domain,
		id, domain,
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
		id, domain,
		id, domain,
	)
}
