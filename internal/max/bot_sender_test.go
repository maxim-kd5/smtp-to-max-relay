package max

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/schemes"

	"smtp-to-max-relay/internal/email"
)

func TestBotSenderSendTextAndFile(t *testing.T) {
	var (
		textPayload map[string]any
		filePayload map[string]any
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if payload["text"] == "hello" {
			textPayload = payload
		} else {
			filePayload = payload
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"recipient":{"chat_id":123},"body":{"mid":"m1"}}}`))
	})
	mux.HandleFunc("/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected upload method: %s", r.Method)
		}
		if got := r.URL.Query().Get("type"); got != string(schemes.FILE) {
			t.Fatalf("unexpected upload type: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"` + strings.TrimRight(testServerURL(r), "/") + `/upload-target"}`))
	})
	mux.HandleFunc("/upload-target", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected upload target method: %s", r.Method)
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, _, err := r.FormFile("data")
		if err != nil {
			t.Fatalf("expected uploaded form file: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "abc" {
			t.Fatalf("unexpected uploaded content: %q", string(content))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"file-token-1"}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":42,"first_name":"relaybot","username":"relay_bot","is_bot":true}`))
	})

	ts := httptest.NewServer(withURLContext(mux))
	defer ts.Close()

	sender, err := NewBotSender(ts.URL, "token-123", 5*time.Second)
	if err != nil {
		t.Fatalf("NewBotSender failed: %v", err)
	}

	if err := sender.SendText(context.Background(), "123", "hello", true); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	att := email.Attachment{Filename: "a.txt", Data: []byte("abc")}
	if err := sender.SendFile(context.Background(), "123", att, false); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	if textPayload["notify"] != false {
		t.Fatalf("expected silent text send to set notify=false, got %#v", textPayload["notify"])
	}
	attachments, ok := filePayload["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected one attachment, got %#v", filePayload["attachments"])
	}
}

func TestBotSenderSendFileWaitsForAttachmentProcessing(t *testing.T) {
	var (
		fileMessageAttempts int
		uploadCalls         int
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if _, ok := payload["attachments"]; ok {
			fileMessageAttempts++
			if fileMessageAttempts < 3 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":"attachment.not.ready","message":"errors.process.attachment.file.not.processed"}`))
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"recipient":{"chat_id":123},"body":{"mid":"m1"}}}`))
	})
	mux.HandleFunc("/uploads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"` + strings.TrimRight(testServerURL(r), "/") + `/upload-target"}`))
	})
	mux.HandleFunc("/upload-target", func(w http.ResponseWriter, r *http.Request) {
		uploadCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"file-token-1"}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":42,"first_name":"relaybot","username":"relay_bot","is_bot":true}`))
	})

	ts := httptest.NewServer(withURLContext(mux))
	defer ts.Close()

	sender, err := NewBotSender(ts.URL, "token-123", 5*time.Second)
	if err != nil {
		t.Fatalf("NewBotSender failed: %v", err)
	}

	att := email.Attachment{Filename: "event.ics", Data: []byte("BEGIN:VCALENDAR")}
	if err := sender.SendFile(context.Background(), "123", att, false); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}

	if uploadCalls != 1 {
		t.Fatalf("expected one upload, got %d", uploadCalls)
	}
	if fileMessageAttempts != 3 {
		t.Fatalf("expected 3 file send attempts, got %d", fileMessageAttempts)
	}
}

func TestReplyForMessageText(t *testing.T) {
	reply, ok := replyForMessageText("/hello", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "ID этого чата: 777") || !strings.Contains(reply, "777@relay.local") || !strings.Contains(reply, "777.silent@relay.local") {
		t.Fatalf("unexpected hello reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = replyForMessageText("/help@relay_bot", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "ID этого чата: 777") || !strings.Contains(reply, "777@relay.local") {
		t.Fatalf("unexpected help reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = replyForMessageText("/start@relay_bot", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "555@relay.local") {
		t.Fatalf("unexpected start reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = replyForMessageText("@relay_bot привет", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "ID этого чата: 777") {
		t.Fatalf("unexpected mention reply: ok=%v reply=%q", ok, reply)
	}

	if _, ok := replyForMessageText("/help@another_bot", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local"); ok {
		t.Fatalf("did not expect command for another bot to trigger reply")
	}

	if _, ok := replyForMessageText("plain text", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local"); ok {
		t.Fatalf("did not expect non-command text to trigger reply")
	}
}

func withURLContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Base-URL") == "" {
			r.Header.Set("X-Test-Base-URL", "http://"+r.Host)
		}
		next.ServeHTTP(w, r)
	})
}

func testServerURL(r *http.Request) string {
	return r.Header.Get("X-Test-Base-URL")
}
