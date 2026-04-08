package recipient

import (
	"encoding/json"
	"os"
)

func LoadAliases(path string) (map[string]string, error) {
	aliases := map[string]string{}
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
	if err := json.Unmarshal(data, &aliases); err != nil {
		return nil, err
	}
	return aliases, nil
}
