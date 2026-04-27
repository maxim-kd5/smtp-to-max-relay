package dlq

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAdminShowAndDryRun(t *testing.T) {
	store, err := NewFileStore(t.TempDir() + "/dlq.json")
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	item, err := store.Enqueue("chatid123@example.com", []byte("raw"), errors.New("boom"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	a := &Admin{
		Store: store,
		DryRun: func(_ context.Context, rcpt string, _ []byte) (string, error) {
			return "route=" + rcpt, nil
		},
	}
	show := a.Show(item.ID)
	if !strings.Contains(show, "recipient: chatid123@example.com") {
		t.Fatalf("unexpected show output: %q", show)
	}
	if !strings.Contains(show, "attempts: 0") {
		t.Fatalf("unexpected show output: %q", show)
	}

	dry := a.ReplayDry(context.Background(), item.ID)
	if !strings.Contains(dry, "Dry-run OK") {
		t.Fatalf("unexpected dry output: %q", dry)
	}
}

func TestAdminReplayBatch(t *testing.T) {
	store, err := NewFileStore(t.TempDir() + "/dlq.json")
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	item, err := store.Enqueue("chatid123@example.com", []byte("raw"), errors.New("boom"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	a := &Admin{
		Store:      store,
		Relay:      func(context.Context, string, []byte) error { return nil },
		WithReplay: func(ctx context.Context) context.Context { return ctx },
		BaseDelay:  time.Second,
		MaxDelay:   time.Minute,
	}
	out := a.ReplayBatch(context.Background(), 1, "only_pending")
	if !strings.Contains(out, "ok=1") {
		t.Fatalf("unexpected replay batch output: %q", out)
	}
	got, ok := store.Get(item.ID)
	if !ok || got.Status != StatusDone {
		t.Fatalf("expected item done, got=%+v ok=%v", got, ok)
	}
}
