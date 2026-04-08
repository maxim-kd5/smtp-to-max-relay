package relay

import (
	"context"
	"fmt"
	"strings"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/max"
	"smtp-to-max-relay/internal/recipient"
)

type Service struct {
	Recipients recipient.Parser
	Email      email.Parser
	Sender     max.Sender
}

func (s *Service) Relay(ctx context.Context, rcpt string, rawMessage []byte) error {
	pr, err := s.Recipients.Parse(rcpt)
	if err != nil {
		return fmt.Errorf("parse recipient: %w", err)
	}

	em, err := s.Email.Parse(rawMessage)
	if err != nil {
		return fmt.Errorf("parse email: %w", err)
	}

	body := em.TextBody
	if strings.TrimSpace(body) == "" {
		body = stripHTMLBasic(em.HTMLBody)
	}

	text := fmt.Sprintf("📧 %s\nОт: %s\n\n%s", fallback(em.Subject, "(без темы)"), em.From, body)
	if err := s.Sender.SendText(ctx, pr.ChatID, pr.ThreadID, text, pr.Silent); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	for _, a := range em.Attachments {
		if err := s.Sender.SendFile(ctx, pr.ChatID, pr.ThreadID, a, pr.Silent); err != nil {
			return fmt.Errorf("send file %s: %w", a.Filename, err)
		}
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
