package maxbot

import (
	"context"
	"net/http"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type bots struct {
	client *client
}

func (b *bots) GetBot(ctx context.Context) (*schemes.BotInfo, error) {
	var info schemes.BotInfo
	if err := b.client.requestJSON(ctx, http.MethodGet, "me", nil, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
