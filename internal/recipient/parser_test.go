package recipient

import "testing"

func TestParseAliasWithSilentFlag(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "chatid123.silent"})

	pr, err := p.Parse("alerts@relay.local")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if pr.ChatID != "123" || !pr.Silent {
		t.Fatalf("unexpected parse result: %+v", pr)
	}
}

func TestParseRejectsThreadSyntax(t *testing.T) {
	p := NewParser("relay.local", nil)
	for _, addr := range []string{"123!7@relay.local", "987_42@relay.local"} {
		if _, err := p.Parse(addr); err == nil {
			t.Fatalf("expected error for thread-style recipient %q", addr)
		}
	}
}

func TestParseRejectsForeignDomain(t *testing.T) {
	p := NewParser("relay.local", nil)
	_, err := p.Parse("chatid123@test.local")
	if err == nil {
		t.Fatalf("expected error for foreign domain")
	}
}

func TestParseRejectsAliasMappedToThreadSyntax(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "chatid123!7.silent"})
	if _, err := p.Parse("alerts@relay.local"); err == nil {
		t.Fatalf("expected error for alias with thread syntax")
	}
}

func TestParseRejectsPlainNumericChatID(t *testing.T) {
	p := NewParser("relay.local", nil)
	for _, addr := range []string{"123@relay.local", "-73211480961715@relay.local", "123.silent@relay.local"} {
		if _, err := p.Parse(addr); err == nil {
			t.Fatalf("expected error for plain numeric recipient %q", addr)
		}
	}
}

func TestParsePrefixedChatID(t *testing.T) {
	p := NewParser("relay.local", nil)

	for _, tc := range []struct {
		addr   string
		chatID string
		silent bool
	}{
		{addr: "chatid123@relay.local", chatID: "123"},
		{addr: "chatid123.silent@relay.local", chatID: "123", silent: true},
		{addr: "chatid-73211480961715@relay.local", chatID: "-73211480961715"},
		{addr: "chatid-73211480961715.silent@relay.local", chatID: "-73211480961715", silent: true},
	} {
		pr, err := p.Parse(tc.addr)
		if err != nil {
			t.Fatalf("unexpected err for %q: %v", tc.addr, err)
		}
		if pr.ChatID != tc.chatID || pr.Silent != tc.silent {
			t.Fatalf("unexpected parse result for %q: %+v", tc.addr, pr)
		}
	}
}

func TestParseRejectsInvalidPrefixedChatID(t *testing.T) {
	p := NewParser("relay.local", nil)
	for _, addr := range []string{
		"chatid@relay.local",
		"chatid-@relay.local",
		"chatid-abc@relay.local",
		"chatid-12x@relay.local",
	} {
		if _, err := p.Parse(addr); err == nil {
			t.Fatalf("expected error for invalid prefixed recipient %q", addr)
		}
	}
}

func TestValidateAliasTarget(t *testing.T) {
	p := NewParser("relay.local", nil)
	if err := p.ValidateAliasTarget("chatid123.silent"); err != nil {
		t.Fatalf("expected valid alias target, got %v", err)
	}
	if err := p.ValidateAliasTarget("chatid123!7.silent"); err == nil {
		t.Fatalf("expected invalid thread alias target")
	}
}
