package relay

import (
	"context"
	"fmt"
	"strings"
	"time"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/max"
	"smtp-to-max-relay/internal/metrics"
	"smtp-to-max-relay/internal/recipient"
)

type Service struct {
	Recipients     recipient.Parser
	Email          email.Parser
	Sender         max.Sender
	MaxSendRetries int
	RetryBaseDelay time.Duration
	Metrics        *metrics.Collector
}

func (s *Service) Relay(ctx context.Context, rcpt string, rawMessage []byte) error {
	if s.Metrics != nil {
		s.Metrics.IncReceived()
	}

	pr, err := s.Recipients.Parse(rcpt)
	if err != nil {
		if s.Metrics != nil {
			s.Metrics.IncFailed()
		}
		return fmt.Errorf("parse recipient: %w", err)
	}

	em, err := s.Email.Parse(rawMessage)
	if err != nil {
		if s.Metrics != nil {
			s.Metrics.IncFailed()
		}
		return fmt.Errorf("parse email: %w", err)
	}

	body := em.TextBody
	if strings.TrimSpace(body) == "" {
		body = stripHTMLBasic(em.HTMLBody)
	}

	text := fmt.Sprintf("📧 %s\nОт: %s\n\n%s", fallback(em.Subject, "(без темы)"), em.From, body)
	if err := s.sendWithRetry(ctx, func() error {
		return s.Sender.SendText(ctx, pr.ChatID, pr.ThreadID, text, pr.Silent)
	}); err != nil {
		if s.Metrics != nil {
			s.Metrics.IncFailed()
		}
		return fmt.Errorf("send text: %w", err)
	}

	if s.Metrics != nil {
		s.Metrics.IncTextSent()
	}

	for _, a := range em.Attachments {
		att := a
		if err := s.sendWithRetry(ctx, func() error {
			return s.Sender.SendFile(ctx, pr.ChatID, pr.ThreadID, att, pr.Silent)
		}); err != nil {
			if s.Metrics != nil {
				s.Metrics.IncFailed()
			}
			return fmt.Errorf("send file %s: %w", a.Filename, err)
		}
		if s.Metrics != nil {
			s.Metrics.IncFilesSent()
		}
	}

	if s.Metrics != nil {
		s.Metrics.IncRelayed()
	}
	return nil
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func stripHTMLBasic(s string) string {
	r := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<p>", "\n", "</p>", "\n")
	out := r.Replace(s)
	out = strings.ReplaceAll(out, "<", "")
	out = strings.ReplaceAll(out, ">", "")
	return strings.TrimSpace(out)
}

func (s *Service) sendWithRetry(ctx context.Context, fn func() error) error {
	maxRetries := s.MaxSendRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	delay := s.RetryBaseDelay
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}

	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt == maxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
