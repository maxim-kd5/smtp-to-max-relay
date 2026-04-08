package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectorHandler(t *testing.T) {
	c := NewCollector()
	c.IncReceived()
	c.IncRelayed()
	c.IncTextSent()
	c.IncFilesSent()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	c.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	for _, line := range []string{
		"smtp_relay_received_total 1",
		"smtp_relay_relayed_total 1",
		"smtp_relay_failed_total 0",
		"smtp_relay_text_sent_total 1",
		"smtp_relay_files_sent_total 1",
	} {
		if !strings.Contains(body, line) {
			t.Fatalf("expected metrics output to contain %q, got %q", line, body)
		}
	}
}
