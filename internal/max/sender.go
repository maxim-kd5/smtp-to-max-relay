package max

import (
	"context"
	"fmt"

	"smtp-to-max-relay/internal/email"
)

type Sender interface {
	SendText(ctx context.Context, chatID, threadID, text string, silent bool) error
	SendFile(ctx context.Context, chatID, threadID string, a email.Attachment, silent bool) error
}

type StubSender struct{}

func NewStubSender() *StubSender {
	return &StubSender{}
}

func (s *StubSender) SendText(_ context.Context, chatID, threadID, text string, silent bool) error {
	fmt.Printf("[MAX:TEXT] chat=%s thread=%s silent=%v text=%q\n", chatID, threadID, silent, text)
	return nil
}

func (s *StubSender) SendFile(_ context.Context, chatID, threadID string, a email.Attachment, silent bool) error {
	fmt.Printf("[MAX:FILE] chat=%s thread=%s silent=%v file=%s size=%d\n", chatID, threadID, silent, a.Filename, a.SizeBytes)
	return nil
}
