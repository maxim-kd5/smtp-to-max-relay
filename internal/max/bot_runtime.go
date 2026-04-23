package max

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"

	"smtp-to-max-relay/internal/recipient"
)

type AliasAdmin interface {
	ValidateAliasTarget(local string) error
	SetAlias(alias, target string)
	DeleteAlias(alias string)
	SnapshotAliases() map[string]string
}

type StatsReporter interface {
	BuildLastDaysReport(days int) string
}

var numericAliasTargetPattern = regexp.MustCompile(`^-?\d+(\.silent)?$`)

func RunBotLoopWithUsername(
	ctx context.Context,
	api *maxbot.Api,
	sender Sender,
	botUserID int64,
	botUsername,
	allowedDomain,
	aliasFilePath string,
	aliasAdmin AliasAdmin,
	statsReporter StatsReporter,
	aliasAdminChatID int64,
) {
	if api == nil || sender == nil {
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-api.GetErrors():
				if !ok {
					return
				}
				if err != nil {
					log.Printf("MAX bot polling error: %v", err)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case upd, ok := <-api.GetUpdates(ctx):
			if !ok {
				return
			}
			handleBotUpdate(ctx, sender, botUserID, botUsername, allowedDomain, aliasFilePath, aliasAdmin, statsReporter, aliasAdminChatID, upd)
		}
	}
}

func handleBotUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, aliasAdminChatID int64, upd schemes.UpdateInterface) {
	switch upd := upd.(type) {
	case *schemes.MessageCreatedUpdate:
		handleMessageCreatedUpdate(ctx, sender, botUserID, botUsername, allowedDomain, aliasFilePath, aliasAdmin, statsReporter, aliasAdminChatID, upd)
	}
}

func handleMessageCreatedUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, aliasAdminChatID int64, upd *schemes.MessageCreatedUpdate) {
	if upd == nil {
		return
	}
	if upd.Message.Sender != nil {
		if upd.Message.Sender.IsBot {
			return
		}
		if botUserID != 0 && upd.Message.Sender.UserId == botUserID {
			return
		}
	}

	chatID := upd.Message.Recipient.ChatId
	if chatID == 0 {
		return
	}

	reply, ok := replyForMessageText(
		upd.Message.Body.Text,
		strconv.FormatInt(chatID, 10),
		upd.Message.Sender,
		botUsername,
		allowedDomain,
	)
	if !ok {
		reply, ok = maybeHandleAdminAliasCommand(
			upd.Message.Body.Text,
			upd.Message.Sender,
			chatID,
			aliasFilePath,
			aliasAdmin,
			statsReporter,
			aliasAdminChatID,
		)
	}
	if !ok {
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := sender.SendText(sendCtx, strconv.FormatInt(chatID, 10), reply, true); err != nil {
		log.Printf("MAX bot reply failed chat_id=%d: %v", chatID, err)
	}
}

func maybeHandleAdminAliasCommand(text string, sender *schemes.User, chatID int64, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, adminChatID int64) (string, bool) {
	if adminChatID == 0 {
		return "", false
	}
	if sender == nil || chatID != adminChatID {
		return "", false
	}

	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return "", false
	}
	switch strings.ToLower(parts[0]) {
	case "/stats7d":
		if statsReporter == nil {
			return "Статистика недоступна", true
		}
		return statsReporter.BuildLastDaysReport(7), true
	case "/alias":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		if len(parts) != 3 {
			return "Использование: /alias <имя> <chatid...>", true
		}
		name := normalizeAliasName(parts[1])
		target, err := normalizeAliasTarget(parts[2])
		if name == "" {
			return "Имя алиаса должно состоять из букв/цифр/._-", true
		}
		if err != nil {
			return err.Error(), true
		}
		if err := aliasAdmin.ValidateAliasTarget(target); err != nil {
			return fmt.Sprintf("Некорректный target алиаса: %v", err), true
		}
		aliasAdmin.SetAlias(name, target)
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Алиас сохранён в памяти, но не записан в файл: %v", err), true
		}
		return fmt.Sprintf("Алиас сохранён: %s -> %s", name, target), true
	case "/unalias":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		if len(parts) != 2 {
			return "Использование: /unalias <имя>", true
		}
		name := normalizeAliasName(parts[1])
		if name == "" {
			return "Имя алиаса должно состоять из букв/цифр/._-", true
		}
		aliasAdmin.DeleteAlias(name)
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Алиас удалён из памяти, но не записан в файл: %v", err), true
		}
		return fmt.Sprintf("Алиас удалён: %s", name), true
	case "/aliases":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		return buildAliasesListReply(aliasAdmin.SnapshotAliases()), true
	}
	return "", false
}

func buildAliasesListReply(aliases map[string]string) string {
	if len(aliases) == 0 {
		return "Список алиасов пуст"
	}

	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names)+2)
	lines = append(lines, "Алиасы (имя -> chatid -> чат):")
	for _, name := range names {
		target := aliases[name]
		chatID := extractChatIDFromAliasTarget(target)
		lines = append(lines, fmt.Sprintf("- %s -> %s -> %s", name, chatID, "(название чата недоступно через Bot API)"))
	}
	return strings.Join(lines, "\n")
}

func extractChatIDFromAliasTarget(target string) string {
	v := strings.TrimSpace(strings.ToLower(target))
	if idx := strings.Index(v, "."); idx >= 0 {
		v = v[:idx]
	}
	if strings.HasPrefix(v, "chatid") {
		v = strings.TrimPrefix(v, "chatid")
	}
	if v == "" {
		return "unknown"
	}
	return v
}

func normalizeAliasName(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return ""
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return v
}

func normalizeAliasTarget(value string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(value))
	if target == "" {
		return "", fmt.Errorf("target алиаса пустой")
	}
	if strings.HasPrefix(target, "chatid") {
		return target, nil
	}
	if numericAliasTargetPattern.MatchString(target) {
		return "chatid" + target, nil
	}
	return "", fmt.Errorf("target алиаса должен быть chatid..., либо числом (например 260920412 или -73211480961715.silent)")
}

func replyForMessageText(text, chatID string, sender *schemes.User, botUsername, allowedDomain string) (string, bool) {
	cmd := ExtractCommand(text)
	if cmd != "" && !CommandTargetsBot(text, botUsername) {
		return "", false
	}

	switch cmd {
	case "hello":
		return BuildChatInfoReply(chatID, allowedDomain), true
	case "help":
		return BuildChatInfoReply(chatID, allowedDomain), true
	case "start":
		userID := ""
		if sender != nil && sender.UserId != 0 {
			userID = strconv.FormatInt(sender.UserId, 10)
		}
		return BuildUserInfoReply(userID, allowedDomain), true
	}

	if MessageMentionsBot(text, botUsername) {
		return BuildChatInfoReply(chatID, allowedDomain), true
	}

	return "", false
}
