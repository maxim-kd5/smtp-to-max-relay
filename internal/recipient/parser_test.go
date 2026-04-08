package recipient

import "testing"

func TestParseAliasWithSilentAndBangThread(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "123!7.silent"})

	pr, err := p.Parse("alerts@relay.local")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if pr.ChatID != "123" || pr.ThreadID != "7" || !pr.Silent {
		t.Fatalf("unexpected parse result: %+v", pr)
	}
}

func TestParseUnderscoreThread(t *testing.T) {
	p := NewParser("relay.local", nil)
	pr, err := p.Parse("987_42@relay.local")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if pr.ChatID != "987" || pr.ThreadID != "42" {
		t.Fatalf("unexpected parse result: %+v", pr)
	}
}

func TestParseRejectsForeignDomain(t *testing.T) {
	p := NewParser("relay.local", nil)
	_, err := p.Parse("123@test.local")
	if err == nil {
		t.Fatalf("expected error for foreign domain")
	}
}
