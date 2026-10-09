package tools

import (
	"encoding/json"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// decodeArgs unmarshals raw tool arguments into T. An absent or JSON-null
// payload yields T's zero value.
func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var args T
	if len(raw) == 0 || string(raw) == "null" {
		return args, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, fmt.Errorf("invalid arguments: %w", err)
	}
	return args, nil
}

// decodeArgsPresent is decodeArgs plus the set of keys present in raw, so
// callers can distinguish an explicit null from an omitted optional argument.
func decodeArgsPresent[T any](raw json.RawMessage) (T, map[string]json.RawMessage, error) {
	args, err := decodeArgs[T](raw)
	if err != nil {
		return args, nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return args, nil, nil
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil {
		return args, nil, fmt.Errorf("invalid arguments: %w", err)
	}
	return args, present, nil
}

// requireFields takes name/value pairs and returns the required-arguments
// result naming every field when any value is empty. All required fields are
// strings, so an empty string means missing.
func requireFields(nameValue ...string) (mcp.ToolResult, bool) {
	if len(nameValue)%2 != 0 {
		panic("requireFields: name/value pairs required")
	}
	names := make([]string, 0, len(nameValue)/2)
	empty := false
	for i, field := range nameValue {
		if i%2 == 0 {
			names = append(names, field)
			continue
		}
		if field == "" {
			empty = true
		}
	}
	if empty {
		return requiredArgsResult(names...), true
	}
	return mcp.ToolResult{}, false
}
