package dlq

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type Item struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Recipient   string    `json:"recipient"`
	RawMessage  []byte    `json:"raw_message"`
	LastError   string    `json:"last_error,omitempty"`
	Status      Status    `json:"status"`
	Attempt     int       `json:"attempt"`
	NextRetryAt time.Time `json:"next_retry_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store interface {
	Enqueue(recipient string, rawMessage []byte, lastErr error) (Item, error)
	PickDue(limit int, now time.Time) ([]Item, error)
	MarkDone(id string) error
	MarkRetry(id string, nextRetryAt time.Time, lastErr error, maxRetries int) error
	Stats() Stats
}

type Stats struct {
	Pending uint64
	Failed  uint64
	Done    uint64
}

type FileStore struct {
	mu    sync.Mutex
	path  string
	items map[string]Item
}

func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path, items: map[string]Item{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStore) Enqueue(recipient string, rawMessage []byte, lastErr error) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	fingerprint := payloadFingerprint(recipient, rawMessage)
	for _, existing := range s.items {
		if existing.Fingerprint != "" &&
			existing.Fingerprint == fingerprint &&
			(existing.Status == StatusPending || existing.Status == StatusProcessing) {
			if lastErr != nil {
				existing.LastError = lastErr.Error()
			}
			existing.UpdatedAt = now
			s.items[existing.ID] = existing
			if err := s.persistLocked(); err != nil {
				return Item{}, err
			}
			return existing, nil
		}
	}

	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%x|%x", now.UnixNano(), randomBytes(8), rawMessage)))
	item := Item{
		ID:          hex.EncodeToString(h[:]),
		Fingerprint: fingerprint,
		Recipient:   recipient,
		RawMessage:  append([]byte(nil), rawMessage...),
		Status:      StatusPending,
		Attempt:     0,
		NextRetryAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if lastErr != nil {
		item.LastError = lastErr.Error()
	}
	s.items[item.ID] = item
	if err := s.persistLocked(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *FileStore) PickDue(limit int, now time.Time) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	ids := make([]string, 0)
	for id, item := range s.items {
		if item.Status == StatusPending && !item.NextRetryAt.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return s.items[ids[i]].NextRetryAt.Before(s.items[ids[j]].NextRetryAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		item := s.items[id]
		item.Status = StatusProcessing
		item.UpdatedAt = now
		s.items[id] = item
		out = append(out, item)
	}
	if len(out) > 0 {
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *FileStore) MarkDone(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("dlq item not found: %s", id)
	}
	item.Status = StatusDone
	item.UpdatedAt = time.Now().UTC()
	s.items[id] = item
	return s.persistLocked()
}

func (s *FileStore) MarkRetry(id string, nextRetryAt time.Time, lastErr error, maxRetries int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("dlq item not found: %s", id)
	}
	item.Attempt++
	item.UpdatedAt = time.Now().UTC()
	if lastErr != nil {
		item.LastError = lastErr.Error()
	}
	if maxRetries > 0 && item.Attempt >= maxRetries {
		item.Status = StatusFailed
	} else {
		item.Status = StatusPending
		item.NextRetryAt = nextRetryAt
	}
	s.items[id] = item
	return s.persistLocked()
}

func (s *FileStore) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st Stats
	for _, item := range s.items {
		switch item.Status {
		case StatusPending, StatusProcessing:
			st.Pending++
		case StatusFailed:
			st.Failed++
		case StatusDone:
			st.Done++
		}
	}
	return st
}

func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	for _, item := range items {
		if item.Status == StatusProcessing {
			item.Status = StatusPending
		}
		if item.Fingerprint == "" {
			item.Fingerprint = payloadFingerprint(item.Recipient, item.RawMessage)
		}
		s.items[item.ID] = item
	}
	return nil
}

func (s *FileStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	items := make([]Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func payloadFingerprint(recipient string, rawMessage []byte) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%x", recipient, rawMessage)))
	return hex.EncodeToString(sum[:])
}

func randomBytes(size int) []byte {
	if size <= 0 {
		return nil
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return []byte(time.Now().UTC().Format(time.RFC3339Nano))
	}
	return b
}
