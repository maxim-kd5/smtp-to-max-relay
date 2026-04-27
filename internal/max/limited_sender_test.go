package max

import (
	"context"
	"errors"
	"testing"
	"time"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/metrics"
)

type countingSender struct {
	calls int
}

func (c *countingSender) SendText(_ context.Context, _, _ string, _ bool) error {
	c.calls++
	return nil
}

func (c *countingSender) SendFile(_ context.Context, _ string, _ email.Attachment, _ bool) error {
	c.calls++
	return nil
}

func TestRateLimitedSenderQueueTimeoutReturnsControlledError(t *testing.T) {
	base := &countingSender{}
	m := metrics.NewCollector()
	sender := NewRateLimitedSender(base, RateLimiterConfig{
		RPS:           1,
		Burst:         1,
		QueueCapacity: 1,
		QueueWait:     50 * time.Millisecond,
	}, m)

	if err := sender.SendText(context.Background(), "1", "first", false); err != nil {
		t.Fatalf("first send should pass burst: %v", err)
	}

	if err := sender.SendText(context.Background(), "1", "queued", false); err == nil {
		t.Fatalf("expected timeout error")
	} else if !errors.Is(err, ErrSendQueueTimeout) {
		t.Fatalf("expected controlled queue timeout error, got: %v", err)
	}
}

func TestRateLimitedSenderDisabledWhenRPSZero(t *testing.T) {
	base := &countingSender{}
	sender := NewRateLimitedSender(base, RateLimiterConfig{RPS: 0}, nil)

	for i := 0; i < 5; i++ {
		if err := sender.SendText(context.Background(), "1", "x", false); err != nil {
			t.Fatalf("unexpected send error: %v", err)
		}
	}

	if base.calls != 5 {
		t.Fatalf("expected direct passthrough sends, got %d", base.calls)
	}
}
