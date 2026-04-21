package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	SMTPListenAddr      string
	SMTPMaxMessageBytes int64
	SMTPAllowedDomain   string
	AliasFilePath       string
	MaxSenderMode       string
	MaxAPIBaseURL       string
	MaxBotToken         string
	MaxSendTimeout      time.Duration
	RelayMaxRetries     int
	RelayRetryDelay     time.Duration
	MetricsListenAddr   string
}

func Load() (Config, error) {
	smtpMaxMessageBytes, err := getEnvInt64("SMTP_MAX_MESSAGE_BYTES", 15*1024*1024)
	if err != nil {
		return Config{}, err
	}
	maxSendTimeoutSec, err := getEnvInt("MAX_SEND_TIMEOUT_SEC", 15)
	if err != nil {
		return Config{}, err
	}
	relayMaxRetries, err := getEnvInt("RELAY_MAX_RETRIES", 2)
	if err != nil {
		return Config{}, err
	}
	relayRetryDelayMS, err := getEnvInt("RELAY_RETRY_DELAY_MS", 300)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		SMTPListenAddr:      getEnv("SMTP_LISTEN_ADDR", ":25"),
		SMTPMaxMessageBytes: smtpMaxMessageBytes,
		SMTPAllowedDomain:   getEnv("SMTP_ALLOWED_RCPT_DOMAIN", "relay.local"),
		AliasFilePath:       getEnv("ALIAS_FILE_PATH", "./config/aliases.json"),
		MaxSenderMode:       getEnv("MAX_SENDER_MODE", "stub"),
		MaxAPIBaseURL:       getEnv("MAX_API_BASE_URL", ""),
		MaxBotToken:         getEnv("MAX_BOT_TOKEN", ""),
		MaxSendTimeout:      time.Duration(maxSendTimeoutSec) * time.Second,
		RelayMaxRetries:     relayMaxRetries,
		RelayRetryDelay:     time.Duration(relayRetryDelayMS) * time.Millisecond,
		MetricsListenAddr:   getEnv("METRICS_LISTEN_ADDR", ":9090"),
	}

	if cfg.SMTPAllowedDomain == "" {
		return Config{}, fmt.Errorf("SMTP_ALLOWED_RCPT_DOMAIN must not be empty")
	}
	if cfg.SMTPMaxMessageBytes <= 0 {
		return Config{}, fmt.Errorf("SMTP_MAX_MESSAGE_BYTES must be positive")
	}
	if cfg.MaxSenderMode != "stub" && cfg.MaxSenderMode != "botapi" {
		return Config{}, fmt.Errorf("MAX_SENDER_MODE must be one of: stub, botapi")
	}
	if cfg.MaxSenderMode != "stub" && cfg.MaxBotToken == "" {
		return Config{}, fmt.Errorf("MAX_BOT_TOKEN must not be empty when MAX_SENDER_MODE=%s", cfg.MaxSenderMode)
	}
	if cfg.RelayMaxRetries < 0 {
		return Config{}, fmt.Errorf("RELAY_MAX_RETRIES must be >= 0")
	}
	if cfg.RelayRetryDelay < 0 {
		return Config{}, fmt.Errorf("RELAY_RETRY_DELAY_MS must be >= 0")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	return n, nil
}

func getEnvInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	return n, nil
}
