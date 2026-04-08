package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Collector struct {
	received  atomic.Uint64
	relayed   atomic.Uint64
	failed    atomic.Uint64
	textSent  atomic.Uint64
	filesSent atomic.Uint64
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) IncReceived()  { c.received.Add(1) }
func (c *Collector) IncRelayed()   { c.relayed.Add(1) }
func (c *Collector) IncFailed()    { c.failed.Add(1) }
func (c *Collector) IncTextSent()  { c.textSent.Add(1) }
func (c *Collector) IncFilesSent() { c.filesSent.Add(1) }

func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "smtp_relay_received_total %d\n", c.received.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_relayed_total %d\n", c.relayed.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_failed_total %d\n", c.failed.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_text_sent_total %d\n", c.textSent.Load())
		_, _ = fmt.Fprintf(w, "smtp_relay_files_sent_total %d\n", c.filesSent.Load())
	})
}
