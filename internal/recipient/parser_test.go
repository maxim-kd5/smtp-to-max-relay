package recipient

import "testing"

func TestParse(t *testing.T) {
	p := NewParser("relay.local", map[string]string{"alerts": "123!7.silent"})

	pr, err := p.Parse("alerts@relay.local")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if pr.ChatID != "123" || pr.ThreadID != "7" || !pr.Silent {
		t.Fatalf("unexpected parse result: %+v", pr)
	}
}
