package recipient

import (
	"fmt"
	"strings"
)

const (
	chatIDPrefix = "chatid"
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

	chatID, err := parseChatIDBase(base, address)
	if err != nil {
		return ParsedRecipient{}, err
	}
	pr.ChatID = chatID

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

func parseChatIDBase(base, address string) (string, error) {
	if strings.HasPrefix(base, chatIDPrefix) {
		return parsePrefixedChatID(strings.TrimPrefix(base, chatIDPrefix), address)
	}

	if strings.Contains(base, "!") || strings.Contains(base, "_") {
		return "", fmt.Errorf("thread addressing is not supported: %s", address)
	}
	return "", fmt.Errorf("recipient must use chatid format: %s", address)
}

func parsePrefixedChatID(value, address string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("chat id is empty: %s", address)
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = strings.TrimPrefix(value, "-")
		if value == "" {
			return "", fmt.Errorf("chat id is empty: %s", address)
		}
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("invalid prefixed chat id: %s", address)
		}
	}
	if negative {
		return "-" + value, nil
	}
	return value, nil
}
