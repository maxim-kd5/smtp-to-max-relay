package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"smtp-to-max-relay/internal/config"
	"smtp-to-max-relay/internal/dlq"
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

	var (
		sender      max.Sender
		botSender   *max.BotSender
		botUserID   int64
		botUsername string
	)

	switch cfg.MaxSenderMode {
	case "botapi":
		botSender, err = max.NewBotSender(cfg.MaxAPIBaseURL, cfg.MaxBotToken, cfg.MaxSendTimeout)
		if err != nil {
			log.Fatalf("max sender error: %v", err)
		}
		sender = botSender
		log.Printf("using MAX sender mode=botapi")

		notifyCtx, cancel := context.WithTimeout(context.Background(), cfg.MaxSendTimeout)
		if err := max.SendStartupNotification(notifyCtx, sender, cfg.AdminChatID); err != nil {
			log.Printf("failed to send startup notification to admin chat_id=%d: %v", cfg.AdminChatID, err)
		} else if cfg.AdminChatID != 0 {
			log.Printf("startup notification sent to admin chat_id=%d", cfg.AdminChatID)
		}
		cancel()

		infoCtx, cancel := context.WithTimeout(context.Background(), cfg.MaxSendTimeout)
		botInfo, err := botSender.API().Bots.GetBot(infoCtx)
		cancel()
		if err != nil {
			log.Printf("failed to fetch MAX bot info: %v", err)
		} else {
			botUserID = botInfo.UserId
			botUsername = botInfo.Username
			log.Printf("MAX bot connected user_id=%d first_name=%q username=%q", botInfo.UserId, botInfo.FirstName, botInfo.Username)
		}
	default:
		sender = max.NewStubSender()
		log.Printf("using MAX sender mode=stub")
	}

	m := metrics.NewCollector()
	recipients := recipient.NewParser(cfg.SMTPAllowedDomain, aliases)
	var dlqAdmin max.DLQAdmin

	relaySvc := &relay.Service{
		Recipients:     recipients,
		Email:          email.NewParser(cfg.SMTPMaxMessageBytes),
		Sender:         sender,
		MaxSendRetries: cfg.RelayMaxRetries,
		RetryBaseDelay: cfg.RelayRetryDelay,
		Metrics:        m,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.DLQEnabled {
		dlqStore, err := dlq.NewFileStore(cfg.DLQFilePath)
		if err != nil {
			log.Fatalf("dlq init error: %v", err)
		}
		relaySvc.DLQ = dlqStore
		log.Printf("DLQ enabled path=%s", cfg.DLQFilePath)

		dlqWorker := &dlq.Worker{
			Store:          dlqStore,
			Relay:          relaySvc.Relay,
			Interval:       cfg.DLQWorkerInterval,
			BaseDelay:      cfg.DLQBaseDelay,
			MaxDelay:       cfg.DLQMaxDelay,
			MaxRetries:     cfg.DLQMaxRetries,
			BatchSize:      10,
			WithReplay:     relay.WithDLQBypass,
			Metrics:        m,
			AttemptTimeout: cfg.MaxSendTimeout,
		}
		go dlqWorker.Run(ctx)

		dlqAdmin = &dlq.Admin{
			Store:      dlqStore,
			Relay:      relaySvc.Relay,
			WithReplay: relay.WithDLQBypass,
			MaxRetries: cfg.DLQMaxRetries,
			BaseDelay:  cfg.DLQBaseDelay,
			MaxDelay:   cfg.DLQMaxDelay,
		}
	}

	server := smtpsrv.NewServer(cfg.SMTPListenAddr, cfg.SMTPAllowedDomain, cfg.SMTPMaxMessageBytes, cfg.SMTPMaxSessions, relaySvc)

	if botSender != nil {
		go max.RunBotLoopWithUsername(
			ctx,
			botSender.API(),
			sender,
			botUserID,
			botUsername,
			cfg.SMTPAllowedDomain,
			cfg.AliasFilePath,
			recipients,
			m,
			dlqAdmin,
			cfg.AdminChatID,
		)
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
