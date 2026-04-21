package max

import (
	"context"
	"fmt"

	"smtp-to-max-relay/internal/email"
)

type Sender interface {
	SendText(ctx context.Context, chatID, text string, silent bool) error
	SendFile(ctx context.Context, chatID string, a email.Attachment, silent bool) error
}

type StubSender struct{}

func NewStubSender() *StubSender {
	return &StubSender{}
}

func (s *StubSender) SendText(_ context.Context, chatID, text string, silent bool) error {
	fmt.Printf("[MAX:TEXT] chat=%s silent=%v text=%q\n", chatID, silent, text)
	return nil
}

func (s *StubSender) SendFile(_ context.Context, chatID string, a email.Attachment, silent bool) error {
	fmt.Printf("[MAX:FILE] chat=%s silent=%v file=%s size=%d\n", chatID, silent, a.Filename, a.SizeBytes)
	return nil
}
