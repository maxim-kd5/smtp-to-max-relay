package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

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
	if cfg.MaxSenderMode == "http" {
		httpSender, err := max.NewHTTPSender(cfg.MaxAPIBaseURL, cfg.MaxBotToken, cfg.MaxSendTimeout)
		if err != nil {
			log.Fatalf("max sender error: %v", err)
		}
		sender = httpSender
		log.Printf("using MAX sender mode=http")
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
