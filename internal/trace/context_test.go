package trace

import (
	"context"
	"testing"
)

func TestWithRequestIDAndPrefix(t *testing.T) {
	ctx := WithRequestID(context.Background(), "smtp-42")

	if got := RequestID(ctx); got != "smtp-42" {
		t.Fatalf("unexpected request id: %q", got)
	}
	if got := Prefix(ctx); got != "[req=smtp-42] " {
		t.Fatalf("unexpected prefix: %q", got)
	}
}

func TestWithRequestIDSkipsEmptyValues(t *testing.T) {
	ctx := WithRequestID(context.Background(), "  ")
	if got := RequestID(ctx); got != "" {
		t.Fatalf("expected empty request id, got: %q", got)
	}
	if got := Prefix(ctx); got != "" {
		t.Fatalf("expected empty prefix, got: %q", got)
	}
}
