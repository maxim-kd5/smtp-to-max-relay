package dlq

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	statusPending = "pending"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("dlq sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create dlq directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Enqueue(ctx context.Context, params EnqueueParams) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("dlq store is not initialized")
	}
	now := time.Now().UTC().Unix()
	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO dlq_messages
		(created_at_unix, updated_at_unix, recipient, raw_message, last_error, status, attempt_count)
		VALUES (?, ?, ?, ?, ?, ?, 0)`,
		now,
		now,
		strings.TrimSpace(params.Recipient),
		params.RawMessage,
		strings.TrimSpace(params.LastError),
		statusPending,
	)
	if err != nil {
		return 0, fmt.Errorf("insert dlq message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("fetch inserted id: %w", err)
	}
	return id, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("dlq store is not initialized")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS dlq_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at_unix INTEGER NOT NULL,
	updated_at_unix INTEGER NOT NULL,
	recipient TEXT NOT NULL,
	raw_message BLOB NOT NULL,
	last_error TEXT NOT NULL,
	status TEXT NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_dlq_status_created
	ON dlq_messages(status, created_at_unix);
`)
	if err != nil {
		return fmt.Errorf("migrate dlq schema: %w", err)
	}
	return nil
}
