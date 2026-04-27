package max

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/metrics"
)

var ErrSendQueueTimeout = errors.New("max send queue timeout")

// QueueTimeoutError represents a managed throttling timeout that can be routed to DLQ.
type QueueTimeoutError struct {
	Wait time.Duration
}

func (e *QueueTimeoutError) Error() string {
	return fmt.Sprintf("%v: waited %s", ErrSendQueueTimeout, e.Wait)
}

func (e *QueueTimeoutError) Unwrap() error {
	return ErrSendQueueTimeout
}

type RateLimiterConfig struct {
	RPS           int
	Burst         int
	QueueCapacity int
	QueueWait     time.Duration
}

type RateLimitedSender struct {
	next          Sender
	metrics       *metrics.Collector
	tokens        chan struct{}
	queueSlots    chan struct{}
	queueWait     time.Duration
	queueDepth    atomic.Int64
	refillTicker  *time.Ticker
	shutdown      chan struct{}
	refillEnabled bool
}

func NewRateLimitedSender(next Sender, cfg RateLimiterConfig, m *metrics.Collector) Sender {
	if next == nil {
		return nil
	}
	if cfg.RPS <= 0 {
		return next
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 1
	}
	if cfg.QueueCapacity < 0 {
		cfg.QueueCapacity = 0
	}
	if cfg.QueueWait <= 0 {
		cfg.QueueWait = 500 * time.Millisecond
	}

	r := &RateLimitedSender{
		next:       next,
		metrics:    m,
		tokens:     make(chan struct{}, cfg.Burst),
		queueSlots: make(chan struct{}, cfg.QueueCapacity),
		queueWait:  cfg.QueueWait,
		shutdown:   make(chan struct{}),
	}

	for i := 0; i < cfg.Burst; i++ {
		r.tokens <- struct{}{}
	}

	interval := time.Second / time.Duration(cfg.RPS)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	r.refillTicker = time.NewTicker(interval)
	r.refillEnabled = true
	go r.refillLoop()

	if r.metrics != nil {
		r.metrics.SetMaxSendQueueDepth(0)
	}

	return r
}

func (r *RateLimitedSender) SendText(ctx context.Context, chatID, text string, silent bool) error {
	if err := r.acquire(ctx); err != nil {
		return err
	}
	return r.next.SendText(ctx, chatID, text, silent)
}

func (r *RateLimitedSender) SendFile(ctx context.Context, chatID string, a email.Attachment, silent bool) error {
	if err := r.acquire(ctx); err != nil {
		return err
	}
	return r.next.SendFile(ctx, chatID, a, silent)
}

func (r *RateLimitedSender) refillLoop() {
	for {
		select {
		case <-r.shutdown:
			return
		case <-r.refillTicker.C:
			select {
			case r.tokens <- struct{}{}:
			default:
			}
		}
	}
}

func (r *RateLimitedSender) acquire(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.tokens:
		return nil
	default:
	}

	select {
	case r.queueSlots <- struct{}{}:
		r.updateQueueDepth(1)
	default:
		if r.metrics != nil {
			r.metrics.IncMaxSendQueueDropped()
			r.metrics.IncMaxSendRateLimited()
		}
		return &QueueTimeoutError{Wait: 0}
	}

	defer func() {
		<-r.queueSlots
		r.updateQueueDepth(-1)
	}()

	timer := time.NewTimer(r.queueWait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		if r.metrics != nil {
			r.metrics.IncMaxSendQueueDropped()
			r.metrics.IncMaxSendRateLimited()
		}
		return &QueueTimeoutError{Wait: r.queueWait}
	case <-r.tokens:
		if r.metrics != nil {
			r.metrics.IncMaxSendRateLimited()
		}
		return nil
	}
}

func (r *RateLimitedSender) updateQueueDepth(delta int) {
	next := r.queueDepth.Add(int64(delta))
	if next < 0 {
		r.queueDepth.Store(0)
		next = 0
	}
	if r.metrics != nil {
		r.metrics.SetMaxSendQueueDepth(uint64(next))
	}
}
