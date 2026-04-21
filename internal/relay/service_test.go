package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/recipient"
)

type fakeSender struct {
	texts []string
	files []email.Attachment
}

func (f *fakeSender) SendText(_ context.Context, _, text string, _ bool) error {
	f.texts = append(f.texts, text)
	return nil
}

func (f *fakeSender) SendFile(_ context.Context, _ string, a email.Attachment, _ bool) error {
	f.files = append(f.files, a)
	return nil
}

func TestRelaySendsTextAndAttachment(t *testing.T) {
	s := &Service{
		Recipients: recipient.NewParser("relay.local", map[string]string{"alerts": "chatid123.silent"}),
		Email:      email.NewParser(1024 * 1024),
		Sender:     &fakeSender{},
	}

	raw := []byte("Subject: Hello\r\nFrom: sender@example.com\r\nContent-Type: multipart/mixed; boundary=b1\r\n\r\n--b1\r\nContent-Type: text/plain\r\n\r\nBody\r\n--b1\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=f.txt\r\n\r\nabc\r\n--b1--\r\n")

	if err := s.Relay(context.Background(), "alerts@relay.local", raw); err != nil {
		t.Fatalf("relay failed: %v", err)
	}

	fs := s.Sender.(*fakeSender)
	if len(fs.texts) != 1 {
		t.Fatalf("expected 1 text send, got %d", len(fs.texts))
	}
	if len(fs.files) != 1 {
		t.Fatalf("expected 1 file send, got %d", len(fs.files))
	}
}

type flakySender struct {
	failTextFor int
	calls       int
}

func (f *flakySender) SendText(_ context.Context, _, _ string, _ bool) error {
	f.calls++
	if f.calls <= f.failTextFor {
		return errors.New("temporary")
	}
	return nil
}

func (f *flakySender) SendFile(_ context.Context, _ string, _ email.Attachment, _ bool) error {
	return nil
}

func TestRelayRetriesOnTemporarySenderError(t *testing.T) {
	flaky := &flakySender{failTextFor: 1}
	s := &Service{
		Recipients:     recipient.NewParser("relay.local", nil),
		Email:          email.NewParser(1024 * 1024),
		Sender:         flaky,
		MaxSendRetries: 2,
		RetryBaseDelay: time.Millisecond,
	}

	raw := []byte("Subject: Retry\r\nFrom: sender@example.com\r\nContent-Type: text/plain\r\n\r\nBody")
	if err := s.Relay(context.Background(), "chatid123@relay.local", raw); err != nil {
		t.Fatalf("relay should succeed after retry, got err: %v", err)
	}
	if flaky.calls != 2 {
		t.Fatalf("expected 2 send attempts, got %d", flaky.calls)
	}
}

func TestRelaySplitsLongTextMessages(t *testing.T) {
	body := strings.Repeat("0123456789", 450)
	s := &Service{
		Recipients: recipient.NewParser("relay.local", nil),
		Email:      email.NewParser(1024 * 1024),
		Sender:     &fakeSender{},
	}

	raw := []byte(fmt.Sprintf("Subject: Long\r\nFrom: sender@example.com\r\nContent-Type: text/plain\r\n\r\n%s", body))
	if err := s.Relay(context.Background(), "chatid123@relay.local", raw); err != nil {
		t.Fatalf("relay failed: %v", err)
	}

	fs := s.Sender.(*fakeSender)
	if len(fs.texts) < 2 {
		t.Fatalf("expected long message to be split, got %d parts", len(fs.texts))
	}
	for i, part := range fs.texts {
		if len(part) > maxTextMessageBytes {
			t.Fatalf("part %d exceeds limit: %d", i, len(part))
		}
	}

	got := strings.Join(fs.texts, "")
	want := fmt.Sprintf("📧 %s\nОт: %s\n\n%s", "Long", "sender@example.com", body)
	if got != want {
		t.Fatalf("unexpected reconstructed text")
	}
}
