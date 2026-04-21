package max

import (
	"context"
	"log"
	"strconv"
	"time"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

func RunBotLoop(ctx context.Context, api *maxbot.Api, sender Sender, botUserID int64, allowedDomain string) {
	RunBotLoopWithUsername(ctx, api, sender, botUserID, "", allowedDomain)
}

func RunBotLoopWithUsername(ctx context.Context, api *maxbot.Api, sender Sender, botUserID int64, botUsername, allowedDomain string) {
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
			handleBotUpdate(ctx, sender, botUserID, botUsername, allowedDomain, upd)
		}
	}
}

func handleBotUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain string, upd schemes.UpdateInterface) {
	switch upd := upd.(type) {
	case *schemes.MessageCreatedUpdate:
		handleMessageCreatedUpdate(ctx, sender, botUserID, botUsername, allowedDomain, upd)
	}
}

func handleMessageCreatedUpdate(ctx context.Context, sender Sender, botUserID int64, botUsername, allowedDomain string, upd *schemes.MessageCreatedUpdate) {
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
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := sender.SendText(sendCtx, strconv.FormatInt(chatID, 10), "", reply, true); err != nil {
		log.Printf("MAX bot reply failed chat_id=%d: %v", chatID, err)
	}
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
