package recipient

import (
	"fmt"
	"strings"
	"sync"
)

const (
	chatIDPrefix = "chatid"
)

type ParsedRecipient struct {
	ChatID      string
	Silent      bool
	RawLocal    string
	RawAddr     string
	SourceLocal string
	AliasUsed   bool
}

type Parser interface {
	Parse(address string) (ParsedRecipient, error)
}

type parser struct {
	allowedDomain string
	mu            sync.RWMutex
	aliases       map[string]string
}

func NewParser(allowedDomain string, aliases map[string]string) *parser {
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
	sourceLocal := local
	if domain != p.allowedDomain {
		return ParsedRecipient{}, fmt.Errorf("unsupported domain: %s", domain)
	}

	p.mu.RLock()
	mapped, ok := p.aliases[local]
	p.mu.RUnlock()
	if ok {
		local = mapped
	}

	pr := ParsedRecipient{
		RawLocal:    local,
		RawAddr:     address,
		SourceLocal: sourceLocal,
		AliasUsed:   ok,
	}

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

func (p *parser) ValidateAliasTarget(local string) error {
	value := strings.TrimSpace(strings.ToLower(local))
	if value == "" {
		return fmt.Errorf("alias target must not be empty")
	}
	base := value
	if idx := strings.Index(value, "."); idx >= 0 {
		base = value[:idx]
	}
	_, err := parseChatIDBase(base, value)
	return err
}

func (p *parser) SetAlias(alias, target string) {
	key := strings.TrimSpace(strings.ToLower(alias))
	value := strings.TrimSpace(strings.ToLower(target))
	if key == "" || value == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.aliases == nil {
		p.aliases = map[string]string{}
	}
	p.aliases[key] = value
}

func (p *parser) DeleteAlias(alias string) {
	key := strings.TrimSpace(strings.ToLower(alias))
	if key == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.aliases, key)
}

func (p *parser) SnapshotAliases() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]string, len(p.aliases))
	for k, v := range p.aliases {
		out[k] = v
	}
	return out
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
