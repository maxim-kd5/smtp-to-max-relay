package max

import (
	"strings"
	"testing"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

func TestBuildUserInfoReply(t *testing.T) {
	reply := BuildUserInfoReply("555", "relay.local")

	if !strings.Contains(reply, "Ваш ID: 555") {
		t.Fatalf("expected user id in reply, got %q", reply)
	}
	if !strings.Contains(reply, "chatid555@relay.local") || !strings.Contains(reply, "chatid555.silent@relay.local") {
		t.Fatalf("expected personal chat addresses in reply, got %q", reply)
	}
	if strings.Contains(reply, "ID этого чата:") {
		t.Fatalf("did not expect chat id label in user reply: %q", reply)
	}
}

func TestBuildChatInfoReplyUsesNegativeChatID(t *testing.T) {
	reply := BuildChatInfoReply("-73211480961715", "relay.local")

	if !strings.Contains(reply, "ID этого чата: -73211480961715") {
		t.Fatalf("expected chat id in reply, got %q", reply)
	}
	if !strings.Contains(reply, "chatid-73211480961715@relay.local") || !strings.Contains(reply, "chatid-73211480961715.silent@relay.local") {
		t.Fatalf("expected negative chat addresses in reply, got %q", reply)
	}
	if strings.Contains(reply, "Ваш ID:") {
		t.Fatalf("did not expect user id label in chat reply: %q", reply)
	}
}

func TestReplyForMessageText(t *testing.T) {
	reply, ok := replyForMessageText("/hello", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "ID этого чата: 777") || !strings.Contains(reply, "chatid777@relay.local") || !strings.Contains(reply, "chatid777.silent@relay.local") {
		t.Fatalf("unexpected hello reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = replyForMessageText("/help@relay_bot", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "ID этого чата: 777") || !strings.Contains(reply, "chatid777@relay.local") {
		t.Fatalf("unexpected help reply: ok=%v reply=%q", ok, reply)
	}
	for _, cmd := range []string{"/start", "/hello", "/help", "/alias", "/unalias", "/stats7d"} {
		if !strings.Contains(reply, cmd) {
			t.Fatalf("expected help to contain command %q, got %q", cmd, reply)
		}
	}

	reply, ok = replyForMessageText("/start@relay_bot", "777", &schemes.User{UserId: 555}, "relay_bot", "relay.local")
	if !ok || !strings.Contains(reply, "Ваш ID: 555") || !strings.Contains(reply, "chatid555@relay.local") {
		t.Fatalf("unexpected start reply: ok=%v reply=%q", ok, reply)
	}
	if strings.Contains(reply, "ID этого чата:") {
		t.Fatalf("did not expect chat id label in start reply: %q", reply)
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
