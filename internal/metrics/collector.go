package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Collector struct {
	received  atomic.Uint64
	relayed   atomic.Uint64
	failed    atomic.Uint64
	textSent  atomic.Uint64
	filesSent atomic.Uint64

	mu                sync.Mutex
	deliveryByAddress map[deliveryKey]uint64
	deliveryEvents    []deliveryEvent
}

type deliveryKey struct {
	Address       string
	Delivered     bool
	MaxRecipient  string
	RecipientName string
}

type deliveryEvent struct {
	At  time.Time
	Key deliveryKey
}

const maxStoredDeliveryEvents = 50000

func NewCollector() *Collector {
	return &Collector{
		deliveryByAddress: map[deliveryKey]uint64{},
	}
}

func (c *Collector) IncReceived()  { c.received.Add(1) }
func (c *Collector) IncRelayed()   { c.relayed.Add(1) }
func (c *Collector) IncFailed()    { c.failed.Add(1) }
func (c *Collector) IncTextSent()  { c.textSent.Add(1) }
func (c *Collector) IncFilesSent() { c.filesSent.Add(1) }

func (c *Collector) ObserveDelivery(address string, delivered bool, maxRecipientID, recipientName string) {
	key := deliveryKey{
		Address:       strings.TrimSpace(strings.ToLower(address)),
		Delivered:     delivered,
		MaxRecipient:  strings.TrimSpace(maxRecipientID),
		RecipientName: strings.TrimSpace(recipientName),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deliveryByAddress[key]++
	c.deliveryEvents = append(c.deliveryEvents, deliveryEvent{At: time.Now().UTC(), Key: key})
	if len(c.deliveryEvents) > maxStoredDeliveryEvents {
		c.deliveryEvents = c.deliveryEvents[len(c.deliveryEvents)-maxStoredDeliveryEvents:]
	}
}

func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "smtp_relay_received_total %d\n", c.received.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_relayed_total %d\n", c.relayed.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_failed_total %d\n", c.failed.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_text_sent_total %d\n", c.textSent.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_files_sent_total %d\n", c.filesSent.Load())
		for _, line := range c.deliveryMetricLines() {
			_, _ = fmt.Fprintln(w, line)
		}
	})
}

func (c *Collector) deliveryMetricLines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]deliveryKey, 0, len(c.deliveryByAddress))
	for k := range c.deliveryByAddress {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Address != keys[j].Address {
			return keys[i].Address < keys[j].Address
		}
		if keys[i].Delivered != keys[j].Delivered {
			return !keys[i].Delivered && keys[j].Delivered
		}
		if keys[i].MaxRecipient != keys[j].MaxRecipient {
			return keys[i].MaxRecipient < keys[j].MaxRecipient
		}
		return keys[i].RecipientName < keys[j].RecipientName
	})

	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf(
			"smtp_relay_delivery_total{address=%q,delivered=%q,max_recipient_id=%q,max_recipient_name=%q} %d",
			escapeLabel(k.Address),
			boolLabel(k.Delivered),
			escapeLabel(k.MaxRecipient),
			escapeLabel(k.RecipientName),
			c.deliveryByAddress[k],
		))
	}
	return lines
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func escapeLabel(v string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return replacer.Replace(v)
}

func (c *Collector) BuildLastDaysReport(days int) string {
	if days <= 0 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	c.mu.Lock()
	defer c.mu.Unlock()

	total := 0
	delivered := 0
	failed := 0
	byAddress := map[string]int{}
	byRecipient := map[string]int{}
	recipientAgg := map[string]recipientStats{}

	for _, ev := range c.deliveryEvents {
		if ev.At.Before(since) {
			continue
		}
		total++
		if ev.Key.Delivered {
			delivered++
		} else {
			failed++
		}
		byAddress[ev.Key.Address]++

		recipientKey := formatRecipientLabel(ev.Key.RecipientName, ev.Key.MaxRecipient)
		agg := recipientAgg[recipientKey]
		agg.Total++
		if ev.Key.Delivered {
			agg.Delivered++
			byRecipient[recipientKey]++
		} else {
			agg.Failed++
		}
		recipientAgg[recipientKey] = agg
	}

	lines := []string{
		fmt.Sprintf("Статистика за %d дней:", days),
		fmt.Sprintf("Принято: %d", total),
		fmt.Sprintf("Доставлено: %d", delivered),
		fmt.Sprintf("Ошибок: %d", failed),
	}

	if len(byAddress) > 0 {
		lines = append(lines, "Топ адресов:")
		for _, kv := range topK(byAddress, 5) {
			lines = append(lines, fmt.Sprintf("- %s: %d", kv.Key, kv.Value))
		}
	}
	if len(byRecipient) > 0 {
		lines = append(lines, "Топ получателей MAX:")
		for _, kv := range topK(byRecipient, 5) {
			lines = append(lines, fmt.Sprintf("- %s: %d", kv.Key, kv.Value))
		}
	}
	if len(recipientAgg) > 0 {
		lines = append(lines, "Агрегировано по получателям MAX:")
		lines = append(lines, fmt.Sprintf("- Уникальных получателей: %d", len(recipientAgg)))
		for _, kv := range topRecipientStats(recipientAgg, 5) {
			lines = append(lines, fmt.Sprintf(
				"- %s: всего %d, доставлено %d, ошибок %d",
				kv.Key,
				kv.Value.Total,
				kv.Value.Delivered,
				kv.Value.Failed,
			))
		}
	}
	return strings.Join(lines, "\n")
}

type recipientStats struct {
	Total     int
	Delivered int
	Failed    int
}

type recipientStatKV struct {
	Key   string
	Value recipientStats
}

func topRecipientStats(m map[string]recipientStats, k int) []recipientStatKV {
	items := make([]recipientStatKV, 0, len(m))
	for key, value := range m {
		items = append(items, recipientStatKV{Key: key, Value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Value.Total != items[j].Value.Total {
			return items[i].Value.Total > items[j].Value.Total
		}
		return items[i].Key < items[j].Key
	})
	if k > 0 && len(items) > k {
		items = items[:k]
	}
	return items
}

func formatRecipientLabel(name, id string) string {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = "unknown"
	}
	cleanID := strings.TrimSpace(id)
	if cleanID == "" {
		cleanID = "unknown"
	}
	return fmt.Sprintf("%s (id=%s)", cleanName, cleanID)
}

type kv struct {
	Key   string
	Value int
}

func topK(m map[string]int, k int) []kv {
	items := make([]kv, 0, len(m))
	for key, value := range m {
		items = append(items, kv{Key: key, Value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		return items[i].Key < items[j].Key
	})
	if k > 0 && len(items) > k {
		items = items[:k]
	}
	return items
}
