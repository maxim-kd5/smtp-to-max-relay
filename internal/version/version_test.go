package version

import "testing"

func TestBotVersion(t *testing.T) {
	old := BuildNumber
	oldSuffix := BuildSuffix
	defer func() {
		BuildNumber = old
		BuildSuffix = oldSuffix
	}()

	BuildNumber = "123"
	BuildSuffix = ""
	if got := BotVersion(); got != "0.2.123" {
		t.Fatalf("unexpected version: %q", got)
	}
}

func TestBotVersionWithSuffix(t *testing.T) {
	old := BuildNumber
	oldSuffix := BuildSuffix
	defer func() {
		BuildNumber = old
		BuildSuffix = oldSuffix
	}()

	BuildNumber = "123"
	BuildSuffix = "dev"
	if got := BotVersion(); got != "0.2.123-dev" {
		t.Fatalf("unexpected version: %q", got)
	}
}
