package version

import "testing"

func TestBotVersion(t *testing.T) {
	old := BuildNumber
	defer func() { BuildNumber = old }()

	BuildNumber = "123"
	if got := BotVersion(); got != "0.2.123" {
		t.Fatalf("unexpected version: %q", got)
	}
}
