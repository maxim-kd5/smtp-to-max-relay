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
	Targets     []RecipientTarget
	RawLocal    string
	RawAddr     string
	SourceLocal string
	AliasUsed   bool
}

type RecipientTarget struct {
	ChatID string
	Silent bool
	Local  string
}

type Parser interface {
	Parse(address string) (ParsedRecipient, error)
}

type parser struct {
	allowedDomain string
	mu            sync.RWMutex
	aliases       map[string][]string
}

func NewParser(allowedDomain string, aliases map[string][]string) *parser {
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

	locals := []string{local}
	if ok {
		locals = mapped
	}

	pr := ParsedRecipient{
		RawLocal:    strings.Join(locals, ","),
		RawAddr:     address,
		SourceLocal: sourceLocal,
		AliasUsed:   ok,
		Targets:     make([]RecipientTarget, 0, len(locals)),
	}

	for _, targetLocal := range locals {
		base := targetLocal
		flags := ""
		if idx := strings.Index(targetLocal, "."); idx >= 0 {
			base = targetLocal[:idx]
			flags = targetLocal[idx+1:]
		}

		chatID, err := parseChatIDBase(base, address)
		if err != nil {
			return ParsedRecipient{}, err
		}
		if chatID == "" {
			return ParsedRecipient{}, fmt.Errorf("chat id is empty")
		}

		pr.Targets = append(pr.Targets, RecipientTarget{
			ChatID: chatID,
			Silent: hasFlag(flags, "silent"),
			Local:  targetLocal,
		})
	}

	if len(pr.Targets) == 0 {
		return ParsedRecipient{}, fmt.Errorf("no recipient targets resolved")
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

func (p *parser) SetAliasGroup(alias string, targets []string) {
	key := strings.TrimSpace(strings.ToLower(alias))
	if key == "" || len(targets) == 0 {
		return
	}
	normalizedTargets := normalizeAliasTargets(targets)
	if len(normalizedTargets) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.aliases == nil {
		p.aliases = map[string][]string{}
	}
	p.aliases[key] = normalizedTargets
}

func (p *parser) AddAliasTargets(alias string, targets []string) {
	key := strings.TrimSpace(strings.ToLower(alias))
	if key == "" || len(targets) == 0 {
		return
	}
	toAdd := normalizeAliasTargets(targets)
	if len(toAdd) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.aliases == nil {
		p.aliases = map[string][]string{}
	}
	existing := p.aliases[key]
	seen := make(map[string]struct{}, len(existing)+len(toAdd))
	merged := make([]string, 0, len(existing)+len(toAdd))
	for _, item := range existing {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range toAdd {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	if len(merged) == 0 {
		return
	}
	p.aliases[key] = merged
}

func (p *parser) RemoveAliasTargets(alias string, targets []string) {
	key := strings.TrimSpace(strings.ToLower(alias))
	if key == "" || len(targets) == 0 {
		return
	}
	toRemove := normalizeAliasTargets(targets)
	if len(toRemove) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	existing, ok := p.aliases[key]
	if !ok || len(existing) == 0 {
		return
	}
	removeSet := map[string]struct{}{}
	for _, item := range toRemove {
		removeSet[item] = struct{}{}
	}
	updated := make([]string, 0, len(existing))
	for _, item := range existing {
		if _, remove := removeSet[item]; remove {
			continue
		}
		updated = append(updated, item)
	}
	if len(updated) == 0 {
		delete(p.aliases, key)
		return
	}
	p.aliases[key] = updated
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

func (p *parser) SnapshotAliases() map[string][]string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string][]string, len(p.aliases))
	for k, v := range p.aliases {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func normalizeAliasTargets(targets []string) []string {
	out := make([]string, 0, len(targets))
	seen := map[string]struct{}{}
	for _, item := range targets {
		target := strings.TrimSpace(strings.ToLower(item))
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
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
