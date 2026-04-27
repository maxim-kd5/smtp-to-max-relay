package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.SMTPListenAddr != ":25" {
		t.Fatalf("unexpected SMTP listen addr: %q", cfg.SMTPListenAddr)
	}
	if cfg.SMTPMaxMessageBytes != 15*1024*1024 {
		t.Fatalf("unexpected max message bytes: %d", cfg.SMTPMaxMessageBytes)
	}
	if cfg.SMTPMaxSessions != 200 {
		t.Fatalf("unexpected max sessions: %d", cfg.SMTPMaxSessions)
	}
	if cfg.MaxSenderMode != "stub" {
		t.Fatalf("unexpected sender mode: %q", cfg.MaxSenderMode)
	}
}

func TestLoadRejectsInvalidIntegerEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SMTP_MAX_MESSAGE_BYTES", "not-a-number")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SMTP_MAX_MESSAGE_BYTES must be a valid integer") {
		t.Fatalf("expected invalid integer error, got %v", err)
	}
}

func TestLoadRejectsLegacyHTTPMode(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("MAX_SENDER_MODE", "http")
	t.Setenv("MAX_BOT_TOKEN", "token")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MAX_SENDER_MODE must be one of: stub, botapi") {
		t.Fatalf("expected sender mode validation error, got %v", err)
	}
}

func TestLoadRequiresTokenForBotAPI(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("MAX_SENDER_MODE", "botapi")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MAX_BOT_TOKEN must not be empty") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestLoadRejectsInvalidAdminChatIDs(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("ADMIN_CHAT_IDS", "oops")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_CHAT_IDS must contain comma-separated integers") {
		t.Fatalf("expected invalid admin chat ids error, got %v", err)
	}
}

func TestLoadRejectsInvalidAdminUserIDs(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("ADMIN_USER_IDS", "0")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_USER_IDS must contain positive integers") {
		t.Fatalf("expected invalid admin user ids error, got %v", err)
	}
}

func TestLoadRejectsInvalidMaxConcurrentSessions(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SMTP_MAX_CONCURRENT_SESSIONS", "0")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SMTP_MAX_CONCURRENT_SESSIONS must be positive") {
		t.Fatalf("expected max concurrent sessions validation error, got %v", err)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"SMTP_LISTEN_ADDR",
		"SMTP_MAX_MESSAGE_BYTES",
		"SMTP_MAX_CONCURRENT_SESSIONS",
		"SMTP_ALLOWED_RCPT_DOMAIN",
		"ALIAS_FILE_PATH",
		"ADMIN_CHAT_IDS",
		"ADMIN_USER_IDS",
		"ACL_FILE_PATH",
		"MAX_SENDER_MODE",
		"MAX_API_BASE_URL",
		"MAX_BOT_TOKEN",
		"MAX_SEND_TIMEOUT_SEC",
		"RELAY_MAX_RETRIES",
		"RELAY_RETRY_DELAY_MS",
		"METRICS_LISTEN_ADDR",
		"DLQ_MAX_DELAY_MS",
		"DLQ_BASE_DELAY_MS",
		"DLQ_MAX_RETRIES",
		"DLQ_WORKER_INTERVAL_MS",
		"DLQ_FILE_PATH",
		"DLQ_ENABLED",
	} {
		t.Setenv(key, "")
	}
}
