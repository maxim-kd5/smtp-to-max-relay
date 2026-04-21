package recipient

import "testing"

func TestParseAliasWithSilentFlag(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "123.silent"})

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
	_, err := p.Parse("123@test.local")
	if err == nil {
		t.Fatalf("expected error for foreign domain")
	}
}

func TestParseRejectsAliasMappedToThreadSyntax(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "123!7.silent"})
	if _, err := p.Parse("alerts@relay.local"); err == nil {
		t.Fatalf("expected error for alias with thread syntax")
	}
}
