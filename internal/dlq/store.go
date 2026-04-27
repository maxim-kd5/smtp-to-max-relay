package dlq

import "context"

type Item struct {
	ID         int64
	CreatedAt  int64
	UpdatedAt  int64
	Recipient  string
	RawMessage []byte
	LastError  string
	Status     string
	Attempts   int
}

type EnqueueParams struct {
	Recipient  string
	RawMessage []byte
	LastError  string
}

type Store interface {
	Enqueue(ctx context.Context, params EnqueueParams) (int64, error)
}
