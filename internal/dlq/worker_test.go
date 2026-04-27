package dlq

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWorkerRunOnceMarksDoneOnSuccess(t *testing.T) {
	store := mustNewStore(t)
	item, err := store.Enqueue("chatid1@relay.local", []byte("msg"), errors.New("boom"))
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	w := &Worker{
		Store:      store,
		Relay:      func(_ context.Context, _ string, _ []byte) error { return nil },
		BatchSize:  10,
		MaxRetries: 3,
	}
	w.runOnce(context.Background())

	got := mustGetItem(t, store, item.ID)
	if got.Status != StatusDone {
		t.Fatalf("expected done status, got %s", got.Status)
	}
}

func TestWorkerRunOnceSchedulesRetryOnError(t *testing.T) {
	store := mustNewStore(t)
	item, err := store.Enqueue("chatid1@relay.local", []byte("msg"), nil)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	w := &Worker{
		Store:      store,
		Relay:      func(_ context.Context, _ string, _ []byte) error { return errors.New("temporary") },
		BatchSize:  10,
		BaseDelay:  time.Millisecond,
		MaxDelay:   time.Second,
		MaxRetries: 3,
	}
	w.runOnce(context.Background())

	got := mustGetItem(t, store, item.ID)
	if got.Status != StatusPending {
		t.Fatalf("expected pending status, got %s", got.Status)
	}
	if got.Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", got.Attempt)
	}
	if !got.NextRetryAt.After(time.Now().UTC().Add(-100 * time.Millisecond)) {
		t.Fatalf("expected next retry in future")
	}
}

func TestWorkerRunOnceRespectsAttemptTimeout(t *testing.T) {
	store := mustNewStore(t)
	item, err := store.Enqueue("chatid1@relay.local", []byte("msg"), nil)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	w := &Worker{
		Store:          store,
		BatchSize:      10,
		BaseDelay:      time.Millisecond,
		MaxDelay:       time.Second,
		MaxRetries:     2,
		AttemptTimeout: 10 * time.Millisecond,
		Relay: func(ctx context.Context, _ string, _ []byte) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	w.runOnce(context.Background())

	got := mustGetItem(t, store, item.ID)
	if got.Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", got.Attempt)
	}
	if got.LastError == "" {
		t.Fatalf("expected timeout error to be recorded")
	}
}

func TestWorkerUpdatesBacklogMetrics(t *testing.T) {
	store := mustNewStore(t)
	if _, err := store.Enqueue("chatid1@relay.local", []byte("msg"), nil); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	m := &captureDLQMetrics{}
	w := &Worker{
		Store:     store,
		Relay:     func(_ context.Context, _ string, _ []byte) error { return nil },
		BatchSize: 10,
		Metrics:   m,
	}
	w.runOnce(context.Background())

	pending, failed, done, oldest := m.snapshot()
	if pending != 0 || failed != 0 || done != 1 {
		t.Fatalf("unexpected dlq backlog metrics pending=%d failed=%d done=%d", pending, failed, done)
	}
	if oldest != 0 {
		t.Fatalf("expected no oldest pending age after successful replay, got %d", oldest)
	}
}

func mustNewStore(t *testing.T) *FileStore {
	t.Helper()
	store, err := NewFileStore(filepath.Join(t.TempDir(), "dlq.json"))
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	return store
}

func mustGetItem(t *testing.T, s *FileStore, id string) Item {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		t.Fatalf("item %s not found", id)
	}
	return item
}

type captureDLQMetrics struct {
	mu      sync.Mutex
	pending uint64
	failed  uint64
	done    uint64
	oldest  uint64
}

func (c *captureDLQMetrics) IncDLQReplayed()     {}
func (c *captureDLQMetrics) IncDLQReplayFailed() {}
func (c *captureDLQMetrics) SetDLQBacklog(pending, failed, done, oldest uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = pending
	c.failed = failed
	c.done = done
	c.oldest = oldest
}

func (c *captureDLQMetrics) snapshot() (uint64, uint64, uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending, c.failed, c.done, c.oldest
}
