package smtp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	netsmtp "net/smtp"
	"strings"
	"sync"
	"testing"
	"time"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/recipient"
	"smtp-to-max-relay/internal/relay"
)

type captureSender struct {
	mu    sync.Mutex
	texts []string
}

func (c *captureSender) SendText(_ context.Context, _, _ string, text string, _ bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.texts = append(c.texts, text)
	return nil
}

func (c *captureSender) SendFile(_ context.Context, _, _ string, _ email.Attachment, _ bool) error {
	return nil
}

func (c *captureSender) textCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.texts)
}

func TestIntegration_SMTPServerToSMTPServer(t *testing.T) {
	addr, stop, sender := startTestServer(t)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	mustReadPrefix(t, r, "220")
	mustWriteLine(t, w, "HELO test-server")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "MAIL FROM:<srv@example.com>")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "RCPT TO:<123@relay.local>")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "DATA")
	mustReadPrefix(t, r, "354")
	mustWriteLine(t, w, "Subject: server-to-server")
	mustWriteLine(t, w, "From: srv@example.com")
	mustWriteLine(t, w, "")
	mustWriteLine(t, w, "hello from smtp server")
	mustWriteLine(t, w, ".")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "QUIT")
	mustReadPrefix(t, r, "221")

	waitForTexts(t, sender, 1)
}

func TestIntegration_SMTPClientToSMTPServer_NoAuth(t *testing.T) {
	addr, stop, sender := startTestServer(t)
	defer stop()

	msg := []byte("Subject: client-no-auth\r\nFrom: cli@example.com\r\n\r\nhello")
	if err := netsmtp.SendMail(addr, nil, "cli@example.com", []string{"123@relay.local"}, msg); err != nil {
		t.Fatalf("SendMail without auth failed: %v", err)
	}

	waitForTexts(t, sender, 1)
}

func TestIntegration_SMTPClientToSMTPServer_WithAnyAuth(t *testing.T) {
	addr, stop, sender := startTestServer(t)
	defer stop()

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort failed: %v", err)
	}
	auth := netsmtp.PlainAuth("", "any-user", "any-pass", host)

	msg := []byte("Subject: client-auth\r\nFrom: auth@example.com\r\n\r\nhello")
	if err := netsmtp.SendMail(addr, auth, "auth@example.com", []string{"123@relay.local"}, msg); err != nil {
		t.Fatalf("SendMail with auth failed: %v", err)
	}

	waitForTexts(t, sender, 1)
}

func startTestServer(t *testing.T) (string, func(), *captureSender) {
	t.Helper()

	addr := mustGetFreeAddr(t)
	sender := &captureSender{}
	svc := &relay.Service{
		Recipients: recipient.NewParser("relay.local", nil),
		Email:      email.NewParser(1024 * 1024),
		Sender:     sender,
	}

	srv := NewServer(addr, "relay.local", 1024*1024, svc)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	waitForServer(t, addr)

	stop := func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("server stop failed: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("server stop timeout")
		}
	}

	return addr, stop, sender
}

func mustGetFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("server did not start on %s", addr)
}

func waitForTexts(t *testing.T, sender *captureSender, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sender.textCount() >= n {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected at least %d sent texts, got %d", n, sender.textCount())
}

func mustReadPrefix(t *testing.T, r *bufio.Reader, prefix string) {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("unexpected response: %q (expected prefix %s)", strings.TrimSpace(line), prefix)
	}
}

func mustWriteLine(t *testing.T, w *bufio.Writer, line string) {
	t.Helper()
	if _, err := fmt.Fprintf(w, "%s\r\n", line); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
}

func TestIntegration_SMTPClientRejectsExternalRecipientDomain(t *testing.T) {
	addr, stop, sender := startTestServer(t)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	mustReadPrefix(t, r, "220")
	mustWriteLine(t, w, "HELO ext-client")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "MAIL FROM:<ext@example.com>")
	mustReadPrefix(t, r, "250")
	mustWriteLine(t, w, "RCPT TO:<someone@example.com>")
	mustReadPrefix(t, r, "550")
	mustWriteLine(t, w, "QUIT")
	mustReadPrefix(t, r, "221")

	if sender.textCount() != 0 {
		t.Fatalf("expected no relayed messages, got %d", sender.textCount())
	}
}
