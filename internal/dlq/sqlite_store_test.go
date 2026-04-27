package dlq

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreEnqueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dlq.sqlite")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	id, err := store.Enqueue(context.Background(), EnqueueParams{
		Recipient:  "chatid123@relay.local",
		RawMessage: []byte("Subject: test\r\n\r\nhello"),
		LastError:  "temporary failure",
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	var (
		recipient string
		status    string
		attempts  int
	)
	if err := db.QueryRow(
		`SELECT recipient, status, attempt_count FROM dlq_messages WHERE id = ?`,
		id,
	).Scan(&recipient, &status, &attempts); err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if recipient != "chatid123@relay.local" {
		t.Fatalf("unexpected recipient: %q", recipient)
	}
	if status != "pending" {
		t.Fatalf("unexpected status: %q", status)
	}
	if attempts != 0 {
		t.Fatalf("unexpected attempt_count: %d", attempts)
	}
}
