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
}

func Load() (Config, error) {
	cfg := Config{
		SMTPListenAddr:      getEnv("SMTP_LISTEN_ADDR", ":25"),
		SMTPMaxMessageBytes: getEnvInt64("SMTP_MAX_MESSAGE_BYTES", 15*1024*1024),
		SMTPAllowedDomain:   getEnv("SMTP_ALLOWED_RCPT_DOMAIN", "relay.local"),
		AliasFilePath:       getEnv("ALIAS_FILE_PATH", "./config/aliases.json"),
		MaxSenderMode:       getEnv("MAX_SENDER_MODE", "stub"),
		MaxAPIBaseURL:       getEnv("MAX_API_BASE_URL", ""),
		MaxBotToken:         getEnv("MAX_BOT_TOKEN", ""),
		MaxSendTimeout:      time.Duration(getEnvInt("MAX_SEND_TIMEOUT_SEC", 15)) * time.Second,
		RelayMaxRetries:     getEnvInt("RELAY_MAX_RETRIES", 2),
		RelayRetryDelay:     time.Duration(getEnvInt("RELAY_RETRY_DELAY_MS", 300)) * time.Millisecond,
	}

	if cfg.SMTPAllowedDomain == "" {
		return Config{}, fmt.Errorf("SMTP_ALLOWED_RCPT_DOMAIN must not be empty")
	}
	if cfg.SMTPMaxMessageBytes <= 0 {
		return Config{}, fmt.Errorf("SMTP_MAX_MESSAGE_BYTES must be positive")
	}
	if cfg.MaxSenderMode != "stub" && cfg.MaxSenderMode != "http" {
		return Config{}, fmt.Errorf("MAX_SENDER_MODE must be one of: stub, http")
	}
	if cfg.MaxSenderMode == "http" && cfg.MaxAPIBaseURL == "" {
		return Config{}, fmt.Errorf("MAX_API_BASE_URL must not be empty when MAX_SENDER_MODE=http")
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

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
