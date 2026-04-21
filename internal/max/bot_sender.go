package max

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"

	"smtp-to-max-relay/internal/email"
)

type BotSender struct {
	api *maxbot.Api
}

const (
	attachmentReadyMaxAttempts = 5
	attachmentReadyRetryDelay  = 300 * time.Millisecond
)

func NewBotSender(baseURL, token string, sendTimeout time.Duration) (*BotSender, error) {
	clientTimeout := 35 * time.Second
	if sendTimeout > 0 && sendTimeout+5*time.Second > clientTimeout {
		clientTimeout = sendTimeout + 5*time.Second
	}

	opts := []maxbot.Option{
		maxbot.WithHTTPClient(&http.Client{Timeout: clientTimeout}),
		maxbot.WithApiTimeout(30 * time.Second),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, maxbot.WithBaseURL(baseURL))
	}

	api, err := maxbot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create bot api client: %w", err)
	}

	return &BotSender{api: api}, nil
}

func (s *BotSender) API() *maxbot.Api {
	return s.api
}

func (s *BotSender) SendText(ctx context.Context, chatID, text string, silent bool) error {
	chatIDInt, err := parseChatID(chatID)
	if err != nil {
		return err
	}

	msg := maxbot.NewMessage().
		SetChat(chatIDInt).
		SetText(text).
		SetNotify(!silent)

	if err := s.api.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("send text message: %w", err)
	}
	return nil
}

func (s *BotSender) SendFile(ctx context.Context, chatID string, a email.Attachment, silent bool) error {
	chatIDInt, err := parseChatID(chatID)
	if err != nil {
		return err
	}

	fileName := strings.TrimSpace(a.Filename)
	if fileName == "" {
		fileName = "attachment.bin"
	}

	uploaded, err := s.api.Uploads.UploadMediaFromReaderWithName(ctx, schemes.FILE, bytes.NewReader(a.Data), fileName)
	if err != nil {
		return fmt.Errorf("upload attachment %q: %w", fileName, err)
	}

	msg := maxbot.NewMessage().
		SetChat(chatIDInt).
		SetNotify(!silent).
		AddFile(uploaded)

	return s.sendFileMessage(ctx, msg)
}

func (s *BotSender) sendFileMessage(ctx context.Context, msg *maxbot.Message) error {
	delay := attachmentReadyRetryDelay
	for attempt := 0; attempt < attachmentReadyMaxAttempts; attempt++ {
		err := s.api.Messages.Send(ctx, msg)
		if err == nil {
			return nil
		}

		var apiErr *maxbot.APIError
		if !errors.As(err, &apiErr) || !apiErr.IsAttachmentNotReady() || attempt == attachmentReadyMaxAttempts-1 {
			return fmt.Errorf("send file message: %w", err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("send file message: %w", ctx.Err())
		case <-time.After(delay):
		}

		if delay < 2*time.Second {
			delay *= 2
		}
	}

	return fmt.Errorf("send file message: exhausted retries")
}

func parseChatID(chatID string) (int64, error) {
	chatIDInt, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chat_id %q: %w", chatID, err)
	}
	return chatIDInt, nil
}
