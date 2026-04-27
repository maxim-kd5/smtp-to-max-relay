package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"smtp-to-max-relay/internal/dlq"
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
		Recipients: recipient.NewParser("relay.local", map[string][]string{"alerts": []string{"chatid123.silent"}}),
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

func TestRelayFanOutAliasGroup(t *testing.T) {
	s := &Service{
		Recipients: recipient.NewParser("relay.local", map[string][]string{"alerts": []string{"chatid111", "chatid222.silent"}}),
		Email:      email.NewParser(1024 * 1024),
		Sender:     &fakeSender{},
	}

	raw := []byte("Subject: Group\r\nFrom: sender@example.com\r\nContent-Type: text/plain\r\n\r\nBody")
	if err := s.Relay(context.Background(), "alerts@relay.local", raw); err != nil {
		t.Fatalf("relay failed: %v", err)
	}

	fs := s.Sender.(*fakeSender)
	if len(fs.texts) != 2 {
		t.Fatalf("expected fan-out text sends to 2 recipients, got %d", len(fs.texts))
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

func TestRelaySendsInlineImageFromHTMLBody(t *testing.T) {
	s := &Service{
		Recipients: recipient.NewParser("relay.local", nil),
		Email:      email.NewParser(1024 * 1024),
		Sender:     &fakeSender{},
	}

	raw := []byte(strings.Join([]string{
		"Subject: Inline photo",
		"From: sender@example.com",
		"To: chatid123@relay.local",
		"MIME-Version: 1.0",
		"Content-Type: multipart/related; boundary=rel1",
		"",
		"--rel1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Body<img src=\"cid:image1\"></p>",
		"--rel1",
		"Content-Type: image/png; name=\"photo.png\"",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <image1>",
		"",
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+nmK0AAAAASUVORK5CYII=",
		"--rel1--",
		"",
	}, "\r\n"))

	if err := s.Relay(context.Background(), "chatid123@relay.local", raw); err != nil {
		t.Fatalf("relay failed: %v", err)
	}

	fs := s.Sender.(*fakeSender)
	if len(fs.texts) != 1 {
		t.Fatalf("expected 1 text send, got %d", len(fs.texts))
	}
	if len(fs.files) != 1 {
		t.Fatalf("expected 1 inline image send, got %d", len(fs.files))
	}
	if fs.files[0].Filename != "photo.png" {
		t.Fatalf("unexpected inline image filename: %q", fs.files[0].Filename)
	}
	if fs.files[0].ContentType != "image/png" {
		t.Fatalf("unexpected inline image content type: %q", fs.files[0].ContentType)
	}
}

type fakeDLQ struct{ enqueued int }

func (f *fakeDLQ) Enqueue(_ string, _ []byte, _ error) (dlq.Item, error) {
	f.enqueued++
	return dlq.Item{ID: "1"}, nil
}
func (f *fakeDLQ) PickDue(_ int, _ time.Time) ([]dlq.Item, error) { return nil, nil }
func (f *fakeDLQ) MarkDone(_ string) error                        { return nil }
func (f *fakeDLQ) MarkRetry(_ string, _ time.Time, _ error, _ int) error {
	return nil
}
func (f *fakeDLQ) Stats() dlq.Stats                                   { return dlq.Stats{} }
func (f *fakeDLQ) OldestPendingAge(_ time.Time) (time.Duration, bool) { return 0, false }
func (f *fakeDLQ) Get(_ string) (dlq.Item, bool)                      { return dlq.Item{}, false }
func (f *fakeDLQ) List(_ int) []dlq.Item                              { return nil }

type alwaysFailSender struct{}

func (a *alwaysFailSender) SendText(_ context.Context, _, _ string, _ bool) error {
	return errors.New("fail")
}
func (a *alwaysFailSender) SendFile(_ context.Context, _ string, _ email.Attachment, _ bool) error {
	return nil
}

func TestRelayEnqueuesDLQOnSendError(t *testing.T) {
	q := &fakeDLQ{}
	s := &Service{
		Recipients: recipient.NewParser("relay.local", nil),
		Email:      email.NewParser(1024 * 1024),
		Sender:     &alwaysFailSender{},
		DLQ:        q,
	}

	raw := []byte("Subject: X\r\nFrom: sender@example.com\r\nContent-Type: text/plain\r\n\r\nBody")
	if err := s.Relay(context.Background(), "chatid123@relay.local", raw); err == nil {
		t.Fatalf("expected relay error")
	}
	if q.enqueued != 1 {
		t.Fatalf("expected 1 dlq enqueue, got %d", q.enqueued)
	}
}
