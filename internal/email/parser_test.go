package email

import (
	"strings"
	"testing"
)

func TestParsePlainText(t *testing.T) {
	raw := []byte("Subject: Test\r\nFrom: sender@example.com\r\nTo: user@relay.local\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello")

	p := NewParser(1024)
	em, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if em.Subject != "Test" {
		t.Fatalf("unexpected subject: %q", em.Subject)
	}
	if em.TextBody != "Hello" {
		t.Fatalf("unexpected text body: %q", em.TextBody)
	}
}

func TestParseMultipartWithAttachment(t *testing.T) {
	boundary := "b1"
	raw := strings.Join([]string{
		"Subject: Multipart",
		"From: sender@example.com",
		"To: user@relay.local",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=b1",
		"",
		"--b1",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello plain",
		"--b1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Hello html</p>",
		"--b1",
		"Content-Type: application/octet-stream",
		"Content-Disposition: attachment; filename=test.txt",
		"",
		"abc",
		"--b1--",
		"",
	}, "\r\n")

	p := NewParser(4096)
	em, err := p.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if em.TextBody != "Hello plain" {
		t.Fatalf("unexpected text body: %q", em.TextBody)
	}
	if em.HTMLBody != "<p>Hello html</p>" {
		t.Fatalf("unexpected html body: %q", em.HTMLBody)
	}
	if len(em.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(em.Attachments))
	}
	if em.Attachments[0].Filename != "test.txt" {
		t.Fatalf("unexpected filename: %q", em.Attachments[0].Filename)
	}
	_ = boundary
}

func TestParseTooLarge(t *testing.T) {
	raw := []byte("Subject: Big\r\n\r\n1234567890")
	p := NewParser(10)
	_, err := p.Parse(raw)
	if err == nil {
		t.Fatalf("expected size error")
	}
}
