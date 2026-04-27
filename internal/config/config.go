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
	SMTPMaxSessions     int
	SMTPAllowedDomain   string
	AliasFilePath       string
	AdminChatID         int64
	MaxSenderMode       string
	MaxAPIBaseURL       string
	MaxBotToken         string
	MaxSendTimeout      time.Duration
	RelayMaxRetries     int
	RelayRetryDelay     time.Duration
	MetricsListenAddr   string
	DLQEnabled          bool
	DLQFilePath         string
	DLQWorkerInterval   time.Duration
	DLQMaxRetries       int
	DLQBaseDelay        time.Duration
	DLQMaxDelay         time.Duration
}

func Load() (Config, error) {
	smtpMaxMessageBytes, err := getEnvInt64("SMTP_MAX_MESSAGE_BYTES", 15*1024*1024)
	if err != nil {
		return Config{}, err
	}
	smtpMaxSessions, err := getEnvInt("SMTP_MAX_CONCURRENT_SESSIONS", 200)
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
	adminChatID, err := getEnvInt64("ADMIN_CHAT_ID", 0)
	if err != nil {
		return Config{}, err
	}

	dlqEnabled, err := getEnvBool("DLQ_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	dlqWorkerIntervalMS, err := getEnvInt("DLQ_WORKER_INTERVAL_MS", 2000)
	if err != nil {
		return Config{}, err
	}
	dlqMaxRetries, err := getEnvInt("DLQ_MAX_RETRIES", 10)
	if err != nil {
		return Config{}, err
	}
	dlqBaseDelayMS, err := getEnvInt("DLQ_BASE_DELAY_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	dlqMaxDelayMS, err := getEnvInt("DLQ_MAX_DELAY_MS", 300000)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		SMTPListenAddr:      getEnv("SMTP_LISTEN_ADDR", ":25"),
		SMTPMaxMessageBytes: smtpMaxMessageBytes,
		SMTPMaxSessions:     smtpMaxSessions,
		SMTPAllowedDomain:   getEnv("SMTP_ALLOWED_RCPT_DOMAIN", "relay.local"),
		AliasFilePath:       getEnv("ALIAS_FILE_PATH", "./config/aliases.json"),
		AdminChatID:         adminChatID,
		MaxSenderMode:       getEnv("MAX_SENDER_MODE", "stub"),
		MaxAPIBaseURL:       getEnv("MAX_API_BASE_URL", ""),
		MaxBotToken:         getEnv("MAX_BOT_TOKEN", ""),
		MaxSendTimeout:      time.Duration(maxSendTimeoutSec) * time.Second,
		RelayMaxRetries:     relayMaxRetries,
		RelayRetryDelay:     time.Duration(relayRetryDelayMS) * time.Millisecond,
		MetricsListenAddr:   getEnv("METRICS_LISTEN_ADDR", ":9090"),
		DLQEnabled:          dlqEnabled,
		DLQFilePath:         getEnv("DLQ_FILE_PATH", "./data/dlq.json"),
		DLQWorkerInterval:   time.Duration(dlqWorkerIntervalMS) * time.Millisecond,
		DLQMaxRetries:       dlqMaxRetries,
		DLQBaseDelay:        time.Duration(dlqBaseDelayMS) * time.Millisecond,
		DLQMaxDelay:         time.Duration(dlqMaxDelayMS) * time.Millisecond,
	}

	if cfg.SMTPAllowedDomain == "" {
		return Config{}, fmt.Errorf("SMTP_ALLOWED_RCPT_DOMAIN must not be empty")
	}
	if cfg.SMTPMaxMessageBytes <= 0 {
		return Config{}, fmt.Errorf("SMTP_MAX_MESSAGE_BYTES must be positive")
	}
	if cfg.SMTPMaxSessions <= 0 {
		return Config{}, fmt.Errorf("SMTP_MAX_CONCURRENT_SESSIONS must be positive")
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
	if cfg.AdminChatID < 0 {
		return Config{}, fmt.Errorf("ADMIN_CHAT_ID must be >= 0")
	}

	if cfg.DLQWorkerInterval < 0 {
		return Config{}, fmt.Errorf("DLQ_WORKER_INTERVAL_MS must be >= 0")
	}
	if cfg.DLQMaxRetries < 0 {
		return Config{}, fmt.Errorf("DLQ_MAX_RETRIES must be >= 0")
	}
	if cfg.DLQBaseDelay < 0 {
		return Config{}, fmt.Errorf("DLQ_BASE_DELAY_MS must be >= 0")
	}
	if cfg.DLQMaxDelay < 0 {
		return Config{}, fmt.Errorf("DLQ_MAX_DELAY_MS must be >= 0")
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

func getEnvBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a valid boolean: %w", key, err)
	}
	return b, nil
}
