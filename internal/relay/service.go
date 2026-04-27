package relay

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"smtp-to-max-relay/internal/dlq"
	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/max"
	"smtp-to-max-relay/internal/metrics"
	"smtp-to-max-relay/internal/recipient"
	"smtp-to-max-relay/internal/trace"
)

const maxTextMessageBytes = 4000

type Service struct {
	Recipients     recipient.Parser
	Email          email.Parser
	Sender         max.Sender
	MaxSendRetries int
	RetryBaseDelay time.Duration
	Metrics        *metrics.Collector
	DLQ            dlq.Store
}

type replayCtxKey struct{}

func WithDLQBypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayCtxKey{}, true)
}

func shouldBypassDLQ(ctx context.Context) bool {
	v, _ := ctx.Value(replayCtxKey{}).(bool)
	return v
}

func (s *Service) Relay(ctx context.Context, rcpt string, rawMessage []byte) error {
	startedAt := time.Now()
	defer func() {
		if s.Metrics != nil {
			s.Metrics.ObserveLatency("relay_total", time.Since(startedAt))
		}
	}()

	if s.Metrics != nil {
		s.Metrics.IncReceived()
	}

	pr, err := s.Recipients.Parse(rcpt)
	if err != nil {
		log.Printf("%srelay parse recipient failed rcpt=%s: %v", trace.Prefix(ctx), rcpt, err)
		if s.Metrics != nil {
			s.Metrics.IncFailed()
			s.Metrics.ObserveDelivery(rcpt, false, "", "")
		}
		return fmt.Errorf("parse recipient: %w", err)
	}

	parseStartedAt := time.Now()
	em, err := s.Email.Parse(rawMessage)
	if s.Metrics != nil {
		s.Metrics.ObserveLatency("email_parse", time.Since(parseStartedAt))
	}
	if err != nil {
		log.Printf("%srelay parse email failed rcpt=%s: %v", trace.Prefix(ctx), rcpt, err)
		if s.Metrics != nil {
			s.Metrics.IncFailed()
			targetChatID := ""
			if len(pr.Targets) > 0 {
				targetChatID = pr.Targets[0].ChatID
			}
			s.Metrics.ObserveDelivery(rcpt, false, targetChatID, pr.SourceLocal)
		}
		return fmt.Errorf("parse email: %w", err)
	}

	body := em.TextBody
	if strings.TrimSpace(body) == "" {
		body = stripHTMLBasic(em.HTMLBody)
	}

	text := fmt.Sprintf("📧 %s\nОт: %s\n\n%s", fallback(em.Subject, "(без темы)"), em.From, body)
	for _, target := range pr.Targets {
		for _, chunk := range splitTextMessage(text, maxTextMessageBytes) {
			chunk := chunk
			sendStartedAt := time.Now()
			err := s.sendWithRetry(ctx, func() error {
				return s.Sender.SendText(ctx, target.ChatID, chunk, target.Silent)
			})
			if s.Metrics != nil {
				s.Metrics.ObserveLatency("max_send", time.Since(sendStartedAt))
			}
			if err != nil {
				log.Printf("%srelay send text failed chat_id=%s rcpt=%s: %v", trace.Prefix(ctx), target.ChatID, rcpt, err)
				if s.Metrics != nil {
					s.Metrics.IncFailed()
					s.Metrics.ObserveDelivery(rcpt, false, target.ChatID, pr.SourceLocal)
				}
				if s.DLQ != nil && !shouldBypassDLQ(ctx) {
					if _, qerr := s.DLQ.Enqueue(rcpt, rawMessage, err); qerr != nil {
						return fmt.Errorf("send text: %w (dlq enqueue: %v)", err, qerr)
					}
					if s.Metrics != nil {
						s.Metrics.IncDLQEnqueued()
					}
				}
				return fmt.Errorf("send text: %w", err)
			}
		}

		if s.Metrics != nil {
			s.Metrics.IncTextSent()
		}

		for _, a := range em.Attachments {
			att := a
			sendStartedAt := time.Now()
			err := s.sendWithRetry(ctx, func() error {
				return s.Sender.SendFile(ctx, target.ChatID, att, target.Silent)
			})
			if s.Metrics != nil {
				s.Metrics.ObserveLatency("max_send", time.Since(sendStartedAt))
			}
			if err != nil {
				log.Printf("%srelay send file failed chat_id=%s file=%s rcpt=%s: %v", trace.Prefix(ctx), target.ChatID, a.Filename, rcpt, err)
				if s.Metrics != nil {
					s.Metrics.IncFailed()
					s.Metrics.ObserveDelivery(rcpt, false, target.ChatID, pr.SourceLocal)
				}
				if s.DLQ != nil && !shouldBypassDLQ(ctx) {
					if _, qerr := s.DLQ.Enqueue(rcpt, rawMessage, err); qerr != nil {
						return fmt.Errorf("send file %s: %w (dlq enqueue: %v)", a.Filename, err, qerr)
					}
					if s.Metrics != nil {
						s.Metrics.IncDLQEnqueued()
					}
				}
				return fmt.Errorf("send file %s: %w", a.Filename, err)
			}
			if s.Metrics != nil {
				s.Metrics.IncFilesSent()
			}
		}

		if s.Metrics != nil {
			s.Metrics.IncRelayed()
			s.Metrics.ObserveDelivery(rcpt, true, target.ChatID, pr.SourceLocal)
		}
		log.Printf("%srelay delivered rcpt=%s chat_id=%s attachments=%d", trace.Prefix(ctx), rcpt, target.ChatID, len(em.Attachments))
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

func splitTextMessage(text string, maxBytes int) []string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return []string{text}
	}

	parts := make([]string, 0, len(text)/maxBytes+1)
	for len(text) > maxBytes {
		cut := bestTextSplit(text, maxBytes)
		parts = append(parts, text[:cut])
		text = text[cut:]
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}

func bestTextSplit(text string, maxBytes int) int {
	if len(text) <= maxBytes {
		return len(text)
	}

	cut := maxBytes
	for cut > 0 && cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut--
	}
	if cut == 0 {
		cut = maxBytes
	}

	if idx := strings.LastIndexAny(text[:cut], "\n\r\t "); idx >= maxBytes/2 {
		return idx + 1
	}
	return cut
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
