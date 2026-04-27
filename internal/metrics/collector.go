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
	latencies         map[string]*latencyHistogram
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

var latencyBucketUpperBounds = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}

type latencyHistogram struct {
	buckets []uint64
	sum     float64
	count   uint64
}

func NewCollector() *Collector {
	return &Collector{
		deliveryByAddress: map[deliveryKey]uint64{},
		latencies:         map[string]*latencyHistogram{},
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

func (c *Collector) ObserveLatency(stage string, d time.Duration) {
	stage = strings.TrimSpace(strings.ToLower(stage))
	if stage == "" {
		return
	}

	seconds := d.Seconds()
	if seconds < 0 {
		seconds = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	h, ok := c.latencies[stage]
	if !ok {
		h = &latencyHistogram{
			buckets: make([]uint64, len(latencyBucketUpperBounds)),
		}
		c.latencies[stage] = h
	}

	for i, ub := range latencyBucketUpperBounds {
		if seconds <= ub {
			h.buckets[i]++
		}
	}
	h.count++
	h.sum += seconds
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
		for _, line := range c.latencyMetricLines() {
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

func (c *Collector) latencyMetricLines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	stages := make([]string, 0, len(c.latencies))
	for stage := range c.latencies {
		stages = append(stages, stage)
	}
	sort.Strings(stages)

	lines := make([]string, 0, len(stages)*(len(latencyBucketUpperBounds)+2))
	for _, stage := range stages {
		h := c.latencies[stage]
		for i, ub := range latencyBucketUpperBounds {
			lines = append(lines, fmt.Sprintf(
				`smtp_relay_latency_seconds_bucket{stage=%q,le=%q} %d`,
				escapeLabel(stage),
				formatPromFloat(ub),
				h.buckets[i],
			))
		}
		lines = append(lines, fmt.Sprintf(
			`smtp_relay_latency_seconds_bucket{stage=%q,le="+Inf"} %d`,
			escapeLabel(stage),
			h.count,
		))
		lines = append(lines, fmt.Sprintf(
			`smtp_relay_latency_seconds_sum{stage=%q} %s`,
			escapeLabel(stage),
			formatPromFloat(h.sum),
		))
		lines = append(lines, fmt.Sprintf(
			`smtp_relay_latency_seconds_count{stage=%q} %d`,
			escapeLabel(stage),
			h.count,
		))
	}

	return lines
}

func formatPromFloat(v float64) string {
	return fmt.Sprintf("%.6f", v)
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
		if ev.Key.Delivered {
			name := ev.Key.RecipientName
			if strings.TrimSpace(name) == "" {
				name = "unknown"
			}
			id := strings.TrimSpace(ev.Key.MaxRecipient)
			if id == "" {
				id = "unknown"
			}
			byRecipient[fmt.Sprintf("%s (id=%s)", name, id)]++
		}
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
	return strings.Join(lines, "\n")
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
