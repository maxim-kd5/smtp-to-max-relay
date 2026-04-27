package recipient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadAliases(path string) (map[string][]string, error) {
	aliases := map[string][]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return aliases, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return aliases, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for alias, value := range raw {
		targets, err := parseAliasValue(value)
		if err != nil {
			return nil, fmt.Errorf("invalid alias %q: %w", alias, err)
		}
		normalizedAlias := strings.TrimSpace(strings.ToLower(alias))
		if normalizedAlias == "" || len(targets) == 0 {
			continue
		}
		aliases[normalizedAlias] = targets
	}
	return aliases, nil
}

func SaveAliases(path string, aliases map[string][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(aliases, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func parseAliasValue(value json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(value, &single); err == nil {
		single = strings.TrimSpace(strings.ToLower(single))
		if single == "" {
			return nil, fmt.Errorf("alias target must not be empty")
		}
		return []string{single}, nil
	}

	var list []string
	if err := json.Unmarshal(value, &list); err != nil {
		return nil, fmt.Errorf("alias target must be string or string[]")
	}

	normalized := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		target := strings.TrimSpace(strings.ToLower(item))
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		normalized = append(normalized, target)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("alias target list must not be empty")
	}
	return normalized, nil
}
