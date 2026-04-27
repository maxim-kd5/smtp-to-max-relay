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
	"smtp-to-max-relay/internal/version"
)

type AliasAdmin interface {
	ValidateAliasTarget(local string) error
	SetAliasGroup(alias string, targets []string)
	AddAliasTargets(alias string, targets []string)
	RemoveAliasTargets(alias string, targets []string)
	DeleteAlias(alias string)
	SnapshotAliases() map[string][]string
}

type StatsReporter interface {
	BuildLastDaysReport(days int) string
}

type DLQAdmin interface {
	Summary() string
	List(limit int) string
	Show(id string) string
	Replay(ctx context.Context, id string) string
	ReplayDry(ctx context.Context, id string) string
	ReplayBatch(ctx context.Context, limit int, mode string) string
}

var numericAliasTargetPattern = regexp.MustCompile(`^-?\d+(\.silent)?$`)

func SendStartupNotification(ctx context.Context, sender Sender, adminChatID int64) error {
	if sender == nil || adminChatID == 0 {
		return nil
	}

	text := fmt.Sprintf("✅ smtp-to-max-relay запущен. Версия бота: %s", version.BotVersion())
	return sender.SendText(ctx, strconv.FormatInt(adminChatID, 10), text, true)
}

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
	dlqAdmin DLQAdmin,
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
			handleBotUpdate(ctx, sender, botUserID, botUsername, allowedDomain, aliasFilePath, aliasAdmin, statsReporter, dlqAdmin, aliasAdminChatID, upd)
		}
	}
}

func handleBotUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, dlqAdmin DLQAdmin, aliasAdminChatID int64, upd schemes.UpdateInterface) {
	switch upd := upd.(type) {
	case *schemes.MessageCreatedUpdate:
		handleMessageCreatedUpdate(ctx, sender, botUserID, botUsername, allowedDomain, aliasFilePath, aliasAdmin, statsReporter, dlqAdmin, aliasAdminChatID, upd)
	}
}

func handleMessageCreatedUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, dlqAdmin DLQAdmin, aliasAdminChatID int64, upd *schemes.MessageCreatedUpdate) {
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
		reply, ok = maybeHandleAdminAliasCommandWithDLQ(
			ctx,
			upd.Message.Body.Text,
			upd.Message.Sender,
			chatID,
			aliasFilePath,
			aliasAdmin,
			statsReporter,
			dlqAdmin,
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
	return maybeHandleAdminAliasCommandWithDLQ(context.Background(), text, sender, chatID, aliasFilePath, aliasAdmin, statsReporter, nil, adminChatID)
}

func maybeHandleAdminAliasCommandWithDLQ(ctx context.Context, text string, sender *schemes.User, chatID int64, aliasFilePath string, aliasAdmin AliasAdmin, statsReporter StatsReporter, dlqAdmin DLQAdmin, adminChatID int64) (string, bool) {
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
	command := strings.ToLower(parts[0])
	log.Printf("admin command audit user_id=%d chat_id=%d command=%q text=%q", sender.UserId, chatID, command, strings.TrimSpace(text))
	switch command {
	case "/dlq":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		return dlqAdmin.Summary(), true
	case "/dlq_list":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		limit := 10
		if len(parts) == 2 {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n <= 0 {
				return "Использование: /dlq_list <limit>", true
			}
			limit = n
		}
		return dlqAdmin.List(limit), true
	case "/dlq_show":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		if len(parts) != 2 {
			return "Использование: /dlq_show <id>", true
		}
		return dlqAdmin.Show(parts[1]), true
	case "/replay_dry":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		if len(parts) != 2 {
			return "Использование: /replay_dry <id>", true
		}
		return dlqAdmin.ReplayDry(ctx, parts[1]), true
	case "/replay":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		if len(parts) != 2 {
			return "Использование: /replay <id>", true
		}
		token := registerConfirm(chatID, 5*time.Minute, func() string {
			return dlqAdmin.Replay(ctx, parts[1])
		})
		log.Printf("admin command pending-confirm user_id=%d chat_id=%d command=%q token=%s", sender.UserId, chatID, command, token)
		return fmt.Sprintf("Требуется подтверждение: /confirm %s (действует 5 минут)", token), true
	case "/replay_batch":
		if dlqAdmin == nil {
			return "DLQ недоступен", true
		}
		if len(parts) < 2 || len(parts) > 3 {
			return "Использование: /replay_batch <limit> [only_failed|only_pending]", true
		}
		limit, err := strconv.Atoi(parts[1])
		if err != nil || limit <= 0 {
			return "Использование: /replay_batch <limit> [only_failed|only_pending]", true
		}
		mode := ""
		if len(parts) == 3 {
			mode = parts[2]
		}
		token := registerConfirm(chatID, 5*time.Minute, func() string {
			return dlqAdmin.ReplayBatch(ctx, limit, mode)
		})
		log.Printf("admin command pending-confirm user_id=%d chat_id=%d command=%q token=%s", sender.UserId, chatID, command, token)
		return fmt.Sprintf("Требуется подтверждение: /confirm %s (действует 5 минут)", token), true
	case "/confirm":
		if len(parts) != 2 {
			return "Использование: /confirm <token>", true
		}
		reply, ok := consumeConfirm(chatID, parts[1])
		if !ok {
			return "Токен подтверждения не найден или истёк", true
		}
		log.Printf("admin command confirmed user_id=%d chat_id=%d token=%s", sender.UserId, chatID, parts[1])
		return reply, true
	case "/stats7d":
		if statsReporter == nil {
			return "Статистика недоступна", true
		}
		return statsReporter.BuildLastDaysReport(7), true
	case "/stats30d":
		if statsReporter == nil {
			return "Статистика недоступна", true
		}
		return statsReporter.BuildLastDaysReport(30), true
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
		aliasAdmin.SetAliasGroup(name, []string{target})
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Алиас сохранён в памяти, но не записан в файл: %v", err), true
		}
		return fmt.Sprintf("Алиас сохранён: %s -> %s", name, target), true
	case "/alias_group":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		if len(parts) != 3 {
			return "Использование: /alias_group <имя> <chatid1,chatid2,...>", true
		}
		name := normalizeAliasName(parts[1])
		targets, err := normalizeAliasTargetsArg(parts[2])
		if name == "" {
			return "Имя алиаса должно состоять из букв/цифр/._-", true
		}
		if err != nil {
			return err.Error(), true
		}
		aliasAdmin.SetAliasGroup(name, targets)
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Группа алиаса сохранена в памяти, но не записана в файл: %v", err), true
		}
		return fmt.Sprintf("Группа алиаса сохранена: %s -> %s", name, strings.Join(targets, ",")), true
	case "/alias_add":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		if len(parts) != 3 {
			return "Использование: /alias_add <имя> <chatid...>", true
		}
		name := normalizeAliasName(parts[1])
		targets, err := normalizeAliasTargetsArg(parts[2])
		if name == "" {
			return "Имя алиаса должно состоять из букв/цифр/._-", true
		}
		if err != nil {
			return err.Error(), true
		}
		aliasAdmin.AddAliasTargets(name, targets)
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Target'ы добавлены в памяти, но не записаны в файл: %v", err), true
		}
		return fmt.Sprintf("Target'ы добавлены: %s -> %s", name, strings.Join(targets, ",")), true
	case "/alias_remove":
		if aliasAdmin == nil {
			return "Управление алиасами недоступно", true
		}
		if len(parts) != 3 {
			return "Использование: /alias_remove <имя> <chatid...>", true
		}
		name := normalizeAliasName(parts[1])
		targets, err := normalizeAliasTargetsArg(parts[2])
		if name == "" {
			return "Имя алиаса должно состоять из букв/цифр/._-", true
		}
		if err != nil {
			return err.Error(), true
		}
		aliasAdmin.RemoveAliasTargets(name, targets)
		if err := recipient.SaveAliases(aliasFilePath, aliasAdmin.SnapshotAliases()); err != nil {
			return fmt.Sprintf("Target'ы удалены в памяти, но не записаны в файл: %v", err), true
		}
		return fmt.Sprintf("Target'ы удалены: %s -> %s", name, strings.Join(targets, ",")), true
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

func buildAliasesListReply(aliases map[string][]string) string {
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
		targetIDs := make([]string, 0, len(aliases[name]))
		for _, target := range aliases[name] {
			targetIDs = append(targetIDs, extractChatIDFromAliasTarget(target))
		}
		lines = append(lines, fmt.Sprintf("- %s -> %s -> %s", name, strings.Join(targetIDs, ","), "(название чата недоступно через Bot API)"))
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

func normalizeAliasTargetsArg(value string) ([]string, error) {
	rawItems := strings.Split(value, ",")
	targets := make([]string, 0, len(rawItems))
	seen := map[string]struct{}{}
	for _, item := range rawItems {
		target, err := normalizeAliasTarget(item)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("нужен хотя бы один target алиаса")
	}
	return targets, nil
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
