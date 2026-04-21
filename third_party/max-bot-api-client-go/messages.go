package maxbot

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type messages struct {
	client *client
}

type messageResponse struct {
	Message schemes.Message `json:"message"`
}

func (m *messages) Send(ctx context.Context, message *Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}
	if message.chatID == 0 && message.userID == 0 {
		return fmt.Errorf("chat or user recipient is required")
	}

	values := url.Values{}
	if message.chatID != 0 {
		values.Set("chat_id", strconv.FormatInt(message.chatID, 10))
	}
	if message.userID != 0 {
		values.Set("user_id", strconv.FormatInt(message.userID, 10))
	}

	var resp messageResponse
	return m.client.requestJSON(ctx, http.MethodPost, "messages", values, message.message, &resp)
}
