package recipient

import (
	"fmt"
	"strings"
)

type ParsedRecipient struct {
	ChatID   string
	Silent   bool
	RawLocal string
	RawAddr  string
}

type Parser interface {
	Parse(address string) (ParsedRecipient, error)
}

type parser struct {
	allowedDomain string
	aliases       map[string]string
}

func NewParser(allowedDomain string, aliases map[string]string) Parser {
	return &parser{
		allowedDomain: strings.ToLower(strings.TrimSpace(allowedDomain)),
		aliases:       aliases,
	}
}

func (p *parser) Parse(address string) (ParsedRecipient, error) {
	addr := strings.TrimSpace(strings.ToLower(address))
	parts := strings.Split(addr, "@")
	if len(parts) != 2 {
		return ParsedRecipient{}, fmt.Errorf("invalid recipient format: %s", address)
	}

	local, domain := parts[0], parts[1]
	if domain != p.allowedDomain {
		return ParsedRecipient{}, fmt.Errorf("unsupported domain: %s", domain)
	}

	if mapped, ok := p.aliases[local]; ok {
		local = mapped
	}

	pr := ParsedRecipient{RawLocal: local, RawAddr: address}

	base := local
	flags := ""
	if idx := strings.Index(local, "."); idx >= 0 {
		base = local[:idx]
		flags = local[idx+1:]
	}
	pr.Silent = hasFlag(flags, "silent")

	if strings.Contains(base, "!") || strings.Contains(base, "_") {
		return ParsedRecipient{}, fmt.Errorf("thread addressing is not supported: %s", address)
	}
	pr.ChatID = base

	if pr.ChatID == "" {
		return ParsedRecipient{}, fmt.Errorf("chat id is empty")
	}

	return pr, nil
}

func hasFlag(flags, target string) bool {
	if flags == "" {
		return false
	}
	for _, f := range strings.Split(flags, ".") {
		if strings.TrimSpace(f) == target {
			return true
		}
	}
	return false
}
