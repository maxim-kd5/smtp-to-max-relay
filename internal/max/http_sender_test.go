package max

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"smtp-to-max-relay/internal/email"
)

func TestHTTPSenderSendTextAndFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer token-123" {
			t.Fatalf("missing auth header")
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/messages/sendFile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer token-123" {
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

	if err := sender.SendText(context.Background(), "chat1", "thread1", "hello", true); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	att := email.Attachment{Filename: "a.txt", Data: []byte("abc")}
	if err := sender.SendFile(context.Background(), "chat1", "thread1", att, false); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
}
