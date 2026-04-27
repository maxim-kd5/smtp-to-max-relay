package dlq

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreEnqueueAndRetryLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "dlq.json"))
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	item, err := store.Enqueue("chatid1@relay.local", []byte("msg"), nil)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	due, err := store.PickDue(10, time.Now().UTC())
	if err != nil {
		t.Fatalf("PickDue failed: %v", err)
	}
	if len(due) != 1 || due[0].ID != item.ID {
		t.Fatalf("unexpected due items: %#v", due)
	}

	next := time.Now().UTC().Add(time.Second)
	if err := store.MarkRetry(item.ID, next, errTest("boom"), 3); err != nil {
		t.Fatalf("MarkRetry failed: %v", err)
	}

	due, err = store.PickDue(10, time.Now().UTC())
	if err != nil {
		t.Fatalf("PickDue failed: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("did not expect due items before next retry")
	}

	due, err = store.PickDue(10, next.Add(time.Millisecond))
	if err != nil {
		t.Fatalf("PickDue failed: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected due item after retry time")
	}

	if err := store.MarkDone(item.ID); err != nil {
		t.Fatalf("MarkDone failed: %v", err)
	}

	st := store.Stats()
	if st.Done != 1 {
		t.Fatalf("expected done=1 got %d", st.Done)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
