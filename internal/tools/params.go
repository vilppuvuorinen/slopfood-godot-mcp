package tools

import (
	"encoding/json"
	"fmt"
)

// aliasParams rewrites snake_case argument keys to their camelCase equivalents
// so handlers accept both spellings, mirroring the reference server's
// parameterMappings. Keys already present under the camelCase name win.
func aliasParams(raw json.RawMessage, aliases map[string]string) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	for from, to := range aliases {
		value, ok := obj[from]
		if !ok {
			continue
		}
		if _, exists := obj[to]; !exists {
			obj[to] = value
		}
		delete(obj, from)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("encode normalized arguments: %w", err)
	}
	return out, nil
}
