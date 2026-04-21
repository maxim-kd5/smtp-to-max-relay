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

func TestParseEncodedHeadersAndBase64Body(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"Subject: =?utf-8?B?0YLQtdGB0YI=?=",
		"From: =?utf-8?B?0JrRg9C00YDRj9Cy0YbQtdCy?= <nit@roskar.ru>",
		"To: =?utf-8?B?0J/QvtC70YPRh9Cw0YLQtdC70Yw=?= <user@relay.local>",
		"Content-Type: text/html; charset=utf-8",
		"Content-Transfer-Encoding: base64",
		"",
		"PGRpdj7Qn9GA0LjQstC10YI8L2Rpdj4=",
	}, "\r\n"))

	p := NewParser(4096)
	em, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if em.Subject != "тест" {
		t.Fatalf("unexpected decoded subject: %q", em.Subject)
	}
	if em.From != "Кудрявцев <nit@roskar.ru>" {
		t.Fatalf("unexpected decoded from: %q", em.From)
	}
	if em.To[0] != "Получатель <user@relay.local>" {
		t.Fatalf("unexpected decoded to: %q", em.To[0])
	}
	if em.HTMLBody != "<div>Привет</div>" {
		t.Fatalf("unexpected html body: %q", em.HTMLBody)
	}
}

func TestParseMultipartQuotedPrintablePart(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"Subject: Multipart qp",
		"From: sender@example.com",
		"To: user@relay.local",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=b1",
		"",
		"--b1",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		"=D0=9F=D1=80=D0=B8=D0=B2=D0=B5=D1=82",
		"--b1--",
		"",
	}, "\r\n"))

	p := NewParser(4096)
	em, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if em.TextBody != "Привет" {
		t.Fatalf("unexpected decoded text body: %q", em.TextBody)
	}
}

func TestParseMultipartRelatedInlineImage(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"Subject: Inline image",
		"From: sender@example.com",
		"To: user@relay.local",
		"MIME-Version: 1.0",
		"Content-Type: multipart/related; boundary=rel1",
		"",
		"--rel1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Hello<img src=\"cid:image1\"></p>",
		"--rel1",
		"Content-Type: image/png; name=\"photo.png\"",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <image1>",
		"",
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+nmK0AAAAASUVORK5CYII=",
		"--rel1--",
		"",
	}, "\r\n"))

	p := NewParser(8192)
	em, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if em.HTMLBody != "<p>Hello<img src=\"cid:image1\"></p>" {
		t.Fatalf("unexpected html body: %q", em.HTMLBody)
	}
	if len(em.Attachments) != 1 {
		t.Fatalf("expected 1 extracted inline image, got %d", len(em.Attachments))
	}
	if em.Attachments[0].Filename != "photo.png" {
		t.Fatalf("unexpected inline image filename: %q", em.Attachments[0].Filename)
	}
	if em.Attachments[0].ContentType != "image/png" {
		t.Fatalf("unexpected inline image content type: %q", em.Attachments[0].ContentType)
	}
}
