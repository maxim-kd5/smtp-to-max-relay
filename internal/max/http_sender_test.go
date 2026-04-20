package max

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"smtp-to-max-relay/internal/email"
)

func TestHTTPSenderSendTextAndFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.URL.Query().Get("chat_id"); got != "123" {
			t.Fatalf("unexpected chat_id query: %q", got)
		}
		if r.Header.Get("Authorization") != "token-123" {
			t.Fatalf("missing auth header")
		}
		var payload map[string]any
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid json payload: %v", err)
		}
		if payload["notify"] != false {
			t.Fatalf("expected notify=false when silent=true, got: %#v", payload["notify"])
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/messages/sendFile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "token-123" {
			t.Fatalf("missing auth header")
		}
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	sender, err := NewHTTPSender(ts.URL, "token-123", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected sender creation error: %v", err)
	}

	if err := sender.SendText(context.Background(), "123", "7", "hello", true); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	att := email.Attachment{Filename: "a.txt", Data: []byte("abc")}
	if err := sender.SendFile(context.Background(), "123", "7", att, false); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
}
