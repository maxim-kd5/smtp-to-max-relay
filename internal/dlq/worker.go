package dlq

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"
)

type RelayFunc func(ctx context.Context, rcpt string, rawMessage []byte) error

type Metrics interface {
	IncDLQReplayed()
	IncDLQReplayFailed()
	SetDLQBacklog(pending, failed, done uint64)
}

type Worker struct {
	Store          Store
	Relay          RelayFunc
	Interval       time.Duration
	BaseDelay      time.Duration
	MaxDelay       time.Duration
	MaxRetries     int
	BatchSize      int
	WithReplay     func(context.Context) context.Context
	Metrics        Metrics
	RandomJitter   time.Duration
	AttemptTimeout time.Duration

	rndMu sync.Mutex
	rnd   *rand.Rand
}

func (w *Worker) Run(ctx context.Context) {
	if w.Store == nil || w.Relay == nil {
		return
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	defer w.updateBacklogMetrics()

	items, err := w.Store.PickDue(w.BatchSize, time.Now().UTC())
	if err != nil {
		log.Printf("dlq pick due failed: %v", err)
		return
	}
	for _, item := range items {
		replayCtx := ctx
		if w.WithReplay != nil {
			replayCtx = w.WithReplay(ctx)
		}
		attemptCtx := replayCtx
		cancel := func() {}
		if timeout := w.AttemptTimeout; timeout > 0 {
			var cancelFn context.CancelFunc
			attemptCtx, cancelFn = context.WithTimeout(replayCtx, timeout)
			cancel = cancelFn
		}
		err := w.Relay(attemptCtx, item.Recipient, item.RawMessage)
		cancel()
		if err == nil {
			if markErr := w.Store.MarkDone(item.ID); markErr != nil {
				log.Printf("dlq mark done failed id=%s: %v", item.ID, markErr)
			}
			if w.Metrics != nil {
				w.Metrics.IncDLQReplayed()
			}
			continue
		}
		next := time.Now().UTC().Add(w.nextDelay(item.Attempt + 1))
		if markErr := w.Store.MarkRetry(item.ID, next, err, w.MaxRetries); markErr != nil {
			log.Printf("dlq mark retry failed id=%s: %v", item.ID, markErr)
		}
		if w.Metrics != nil {
			w.Metrics.IncDLQReplayFailed()
		}
	}
}

func (w *Worker) updateBacklogMetrics() {
	if w.Store == nil || w.Metrics == nil {
		return
	}
	st := w.Store.Stats()
	w.Metrics.SetDLQBacklog(st.Pending, st.Failed, st.Done)
}

func (w *Worker) nextDelay(attempt int) time.Duration {
	base := w.BaseDelay
	if base <= 0 {
		base = time.Second
	}
	max := w.MaxDelay
	if max <= 0 {
		max = 5 * time.Minute
	}
	if attempt < 1 {
		attempt = 1
	}
	d := base << (attempt - 1)
	if d < base {
		d = max
	}
	if d > max {
		d = max
	}
	j := w.RandomJitter
	if j <= 0 {
		j = 250 * time.Millisecond
	}
	return d + time.Duration(w.randInt63n(int64(j)))
}

func (w *Worker) randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	w.rndMu.Lock()
	defer w.rndMu.Unlock()
	if w.rnd == nil {
		w.rnd = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	}
	return w.rnd.Int63n(n)
}
