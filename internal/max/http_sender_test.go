package max

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
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
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse content type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected content type: %q", mediaType)
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("chat_id"); got != "123" {
			t.Fatalf("unexpected multipart chat_id: %q", got)
		}
		if got := r.FormValue("silent"); got != "false" {
			t.Fatalf("unexpected multipart silent: %q", got)
		}
		if got := r.FormValue("thread_id"); got != "" {
			t.Fatalf("did not expect thread_id field, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	sender, err := NewHTTPSender(ts.URL, "token-123", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected sender creation error: %v", err)
	}

	if err := sender.SendText(context.Background(), "123", "hello", true); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	att := email.Attachment{Filename: "a.txt", Data: []byte("abc")}
	if err := sender.SendFile(context.Background(), "123", att, false); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
}

func TestParseMessagesResponseExtractsUserID(t *testing.T) {
	body := []byte(`{"messages":[{"id":"m1","text":"/help","chat_id":"777","sender":{"id":"555"}}]}`)
	msgs, err := parseMessagesResponse(body)
	if err != nil {
		t.Fatalf("parseMessagesResponse failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %d", len(msgs))
	}
	if msgs[0].UserID != "555" {
		t.Fatalf("unexpected user id: %q", msgs[0].UserID)
	}
	if msgs[0].ChatID != "777" {
		t.Fatalf("unexpected chat id: %q", msgs[0].ChatID)
	}
}

func TestBuildUserInfoReply(t *testing.T) {
	reply := BuildUserInfoReply("555", "relay.local")
	for _, fragment := range []string{"chatid555@relay.local", "chatid555.silent@relay.local"} {
		if !strings.Contains(reply, fragment) {
			t.Fatalf("reply missing %q: %q", fragment, reply)
		}
	}
	if strings.Contains(reply, "— 555@relay.local") || strings.Contains(reply, "— 555.silent@relay.local") {
		t.Fatalf("reply should use uniform chatid format: %q", reply)
	}
	if strings.Contains(reply, "!123@relay.local") {
		t.Fatalf("reply should not mention thread syntax: %q", reply)
	}
	if !ShouldReplyWithUserInfo("/help") {
		t.Fatalf("expected /help to trigger auto-reply")
	}
	if !ShouldReplyWithUserInfo("/start anything") {
		t.Fatalf("expected /start with args to trigger auto-reply")
	}
	if !ShouldReplyWithUserInfo("/help@relaybot") {
		t.Fatalf("expected /help@relaybot to trigger auto-reply")
	}
	if !ShouldReplyHello("/hello") {
		t.Fatalf("expected /hello to trigger greeting")
	}
	if ShouldReplyWithUserInfo("hello") {
		t.Fatalf("did not expect generic text to trigger auto-reply")
	}
}

func TestBuildChatInfoReplyUsesSafeAddressForNegativeChatID(t *testing.T) {
	reply := BuildChatInfoReply("-73211480961715", "relay.local")
	if !strings.Contains(reply, "chatid-73211480961715@relay.local") {
		t.Fatalf("reply missing safe negative chat address: %q", reply)
	}
}
