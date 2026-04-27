package dlq

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Admin struct {
	Store      Store
	Relay      RelayFunc
	WithReplay func(context.Context) context.Context
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func (a *Admin) Summary() string {
	if a == nil || a.Store == nil {
		return "DLQ недоступен"
	}
	st := a.Store.Stats()
	oldest := "нет"
	if age, ok := a.Store.OldestPendingAge(time.Now().UTC()); ok {
		oldest = age.Round(time.Second).String()
	}
	return fmt.Sprintf("DLQ: pending=%d failed=%d done=%d oldest_pending=%s", st.Pending, st.Failed, st.Done, oldest)
}

func (a *Admin) List(limit int) string {
	if a == nil || a.Store == nil {
		return "DLQ недоступен"
	}
	items := a.Store.List(limit)
	if len(items) == 0 {
		return "DLQ пуст"
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	lines := []string{"Последние элементы DLQ:"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- id=%s status=%s attempts=%d updated=%s err=%s",
			shortID(item.ID), item.Status, item.Attempt, item.UpdatedAt.Format(time.RFC3339), fallback(item.LastError, "-")))
	}
	return strings.Join(lines, "\n")
}

func (a *Admin) Replay(ctx context.Context, id string) string {
	if a == nil || a.Store == nil || a.Relay == nil {
		return "DLQ replay недоступен"
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "Использование: /replay <id>"
	}
	item, ok := a.Store.Get(id)
	if !ok {
		return fmt.Sprintf("Элемент DLQ не найден: %s", id)
	}
	replayCtx := ctx
	if a.WithReplay != nil {
		replayCtx = a.WithReplay(ctx)
	}
	if err := a.Relay(replayCtx, item.Recipient, item.RawMessage); err != nil {
		next := time.Now().UTC().Add(nextDelay(item.Attempt+1, a.BaseDelay, a.MaxDelay))
		if markErr := a.Store.MarkRetry(item.ID, next, err, a.MaxRetries); markErr != nil {
			return fmt.Sprintf("Replay не удался: %v (и не удалось обновить DLQ: %v)", err, markErr)
		}
		return fmt.Sprintf("Replay не удался: %v", err)
	}
	if err := a.Store.MarkDone(item.ID); err != nil {
		return fmt.Sprintf("Replay отправлен, но не удалось обновить DLQ: %v", err)
	}
	return fmt.Sprintf("Replay успешно выполнен: %s", id)
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func nextDelay(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	if attempt < 1 {
		attempt = 1
	}
	d := base << (attempt - 1)
	if d < base || d > max {
		d = max
	}
	return d
}
