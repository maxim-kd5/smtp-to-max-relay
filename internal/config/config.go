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
	MaxBotToken         string
	MaxSendTimeout      time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		SMTPListenAddr:      getEnv("SMTP_LISTEN_ADDR", ":2525"),
		SMTPMaxMessageBytes: getEnvInt64("SMTP_MAX_MESSAGE_BYTES", 15*1024*1024),
		SMTPAllowedDomain:   getEnv("SMTP_ALLOWED_RCPT_DOMAIN", "relay.local"),
		AliasFilePath:       getEnv("ALIAS_FILE_PATH", "./config/aliases.json"),
		MaxBotToken:         getEnv("MAX_BOT_TOKEN", "dev-token"),
		MaxSendTimeout:      time.Duration(getEnvInt("MAX_SEND_TIMEOUT_SEC", 15)) * time.Second,
	}

	if cfg.SMTPAllowedDomain == "" {
		return Config{}, fmt.Errorf("SMTP_ALLOWED_RCPT_DOMAIN must not be empty")
	}
	if cfg.SMTPMaxMessageBytes <= 0 {
		return Config{}, fmt.Errorf("SMTP_MAX_MESSAGE_BYTES must be positive")
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
