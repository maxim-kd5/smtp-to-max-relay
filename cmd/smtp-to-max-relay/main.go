package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"smtp-to-max-relay/internal/config"
	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/max"
	"smtp-to-max-relay/internal/metrics"
	"smtp-to-max-relay/internal/recipient"
	"smtp-to-max-relay/internal/relay"
	smtpsrv "smtp-to-max-relay/internal/smtp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	aliases, err := recipient.LoadAliases(cfg.AliasFilePath)
	if err != nil {
		log.Fatalf("aliases error: %v", err)
	}

	var sender max.Sender
	var httpSenderForAutoReply *max.HTTPSender
	if cfg.MaxSenderMode == "http" {
		httpSender, err := max.NewHTTPSender(cfg.MaxAPIBaseURL, cfg.MaxBotToken, cfg.MaxSendTimeout)
		if err != nil {
			log.Fatalf("max sender error: %v", err)
		}
		sender = httpSender
		log.Printf("using MAX sender mode=http")
		printChatsContext, cancel := context.WithTimeout(context.Background(), cfg.MaxSendTimeout)
		chats, err := httpSender.ListChats(printChatsContext)
		cancel()
		if err != nil {
			log.Printf("failed to list MAX chats: %v", err)
		} else {
			log.Printf("bot has access to %d chat(s):", len(chats))
			for _, c := range chats {
				log.Printf("MAX chat_id=%s title=%q", c.ID, c.Title)
			}

			for _, c := range chats {
				printMessagesContext, cancelMessages := context.WithTimeout(context.Background(), cfg.MaxSendTimeout)
				messages, err := httpSender.ListMessagesByChat(printMessagesContext, c.ID, 3)
				cancelMessages()
				if err != nil {
					log.Printf("failed to list MAX messages for chat_id=%s: %v", c.ID, err)
					continue
				}
				log.Printf("last %d message(s) for chat_id=%s:", len(messages), c.ID)
				for _, msg := range messages {
					log.Printf("MAX message_id=%s text=%q", msg.ID, msg.Text)
				}
			}
		}

		printSubscriptionsContext, cancelSubscriptions := context.WithTimeout(context.Background(), cfg.MaxSendTimeout)
		subscriptions, err := httpSender.ListSubscriptions(printSubscriptionsContext)
		cancelSubscriptions()
		if err != nil {
			log.Printf("failed to list MAX subscriptions: %v", err)
		} else {
			log.Printf("bot has %d subscription(s):", len(subscriptions))
			for _, s := range subscriptions {
				log.Printf("MAX subscription url=%q", s.URL)
			}
		}
		httpSenderForAutoReply = httpSender
	} else {
		sender = max.NewStubSender()
		log.Printf("using MAX sender mode=stub")
	}

	m := metrics.NewCollector()

	relaySvc := &relay.Service{
		Recipients:     recipient.NewParser(cfg.SMTPAllowedDomain, aliases),
		Email:          email.NewParser(cfg.SMTPMaxMessageBytes),
		Sender:         sender,
		MaxSendRetries: cfg.RelayMaxRetries,
		RetryBaseDelay: cfg.RelayRetryDelay,
		Metrics:        m,
	}

	server := smtpsrv.NewServer(cfg.SMTPListenAddr, cfg.SMTPAllowedDomain, cfg.SMTPMaxMessageBytes, relaySvc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if httpSenderForAutoReply != nil {
		go runUserInfoAutoReply(ctx, httpSenderForAutoReply, cfg.SMTPAllowedDomain)
	}

	if cfg.MetricsListenAddr != "" {
		go func() {
			log.Printf("metrics listening on %s", cfg.MetricsListenAddr)
			if err := http.ListenAndServe(cfg.MetricsListenAddr, m.Handler()); err != nil {
				log.Printf("metrics server stopped: %v", err)
			}
		}()
	}

	log.Printf("starting smtp-to-max-relay on %s", cfg.SMTPListenAddr)
	if err := server.ListenAndServe(ctx); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func runUserInfoAutoReply(ctx context.Context, sender *max.HTTPSender, allowedDomain string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	immediate := make(chan struct{}, 1)
	immediate <- struct{}{}

	seen := map[string]struct{}{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-immediate:
		case <-ticker.C:
		}

		pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		chats, err := sender.ListChats(pollCtx)
		cancel()
		if err != nil {
			log.Printf("auto-reply: list chats failed: %v", err)
			continue
		}

		for _, c := range chats {
			pollMessagesCtx, cancelMessages := context.WithTimeout(ctx, 10*time.Second)
			messages, err := sender.ListMessagesByChat(pollMessagesCtx, c.ID, 10)
			cancelMessages()
			if err != nil {
				continue
			}
			for _, msg := range messages {
				msgID := strings.TrimSpace(msg.ID)
				if msgID == "" {
					continue
				}
				if _, ok := seen[msgID]; ok {
					continue
				}

				if !max.ShouldReplyWithUserInfo(msg.Text) {
					seen[msgID] = struct{}{}
					continue
				}

				userID := strings.TrimSpace(msg.UserID)
				if userID == "" {
					userID = c.ID
				}
				reply := max.BuildUserInfoReply(userID, allowedDomain)
				sendCtx, cancelSend := context.WithTimeout(ctx, 10*time.Second)
				err := sender.SendText(sendCtx, c.ID, "", reply, true)
				cancelSend()
				if err != nil {
					log.Printf("auto-reply: send failed chat_id=%s msg_id=%s err=%v", c.ID, msgID, err)
					continue
				}
				seen[msgID] = struct{}{}
			}
		}
	}
}
