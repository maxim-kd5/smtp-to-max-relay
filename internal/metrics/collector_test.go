package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollectorHandler(t *testing.T) {
	c := NewCollector()
	c.IncReceived()
	c.IncRelayed()
	c.IncTextSent()
	c.IncFilesSent()
	c.IncMaxSendRateLimited()
	c.SetMaxSendQueueDepth(3)
	c.IncMaxSendQueueDropped()
	c.ObserveLatency("email_parse", 20*time.Millisecond)
	c.ObserveLatency("max_send", 120*time.Millisecond)
	c.ObserveDelivery("alerts@relay.local", true, "123", "alerts")
	c.ObserveDelivery("bad@relay.local", false, "", "")

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
		"max_send_rate_limited_total 1",
		"max_send_queue_depth 3",
		"max_send_queue_dropped_total 1",
		`smtp_relay_latency_seconds_count{stage="email_parse"} 1`,
		`smtp_relay_latency_seconds_count{stage="max_send"} 1`,
		`smtp_relay_delivery_total{address="alerts@relay.local",delivered="true",max_recipient_id="123",max_recipient_name="alerts"} 1`,
		`smtp_relay_delivery_total{address="bad@relay.local",delivered="false",max_recipient_id="",max_recipient_name=""} 1`,
	} {
		if !strings.Contains(body, line) {
			t.Fatalf("expected metrics output to contain %q, got %q", line, body)
		}
	}
}

func TestBuildLastDaysReport(t *testing.T) {
	c := NewCollector()
	c.ObserveDelivery("alerts@relay.local", true, "123", "alerts")
	c.ObserveDelivery("alerts@relay.local", false, "123", "alerts")
	c.ObserveDelivery("chatid55@relay.local", false, "55", "chatid55")

	report := c.BuildLastDaysReport(7)
	for _, want := range []string{
		"Статистика за 7 дней:",
		"Принято: 3",
		"Доставлено: 1",
		"Ошибок: 2",
		"Топ адресов:",
		"alerts@relay.local",
		"Топ получателей MAX:",
		"Агрегировано по получателям MAX:",
		"- Уникальных получателей: 2",
		"- alerts (id=123): всего 2, доставлено 1, ошибок 1",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("expected report to contain %q, got %q", want, report)
		}
	}
}
