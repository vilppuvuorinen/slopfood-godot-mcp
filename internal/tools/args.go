package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// decodeArgs unmarshals raw tool arguments into T and reports the keys that
// were present. An absent or JSON-null payload yields T's zero value and an
// empty presence set, so callers can distinguish an explicit null from an
// omitted key when validating optional arguments.
func decodeArgs[T any](raw json.RawMessage) (T, map[string]json.RawMessage, error) {
	var args T
	if len(raw) == 0 || string(raw) == "null" {
		return args, nil, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, nil, fmt.Errorf("invalid arguments: %w", err)
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil {
		return args, nil, fmt.Errorf("invalid arguments: %w", err)
	}
	return args, present, nil
}

// requireFields returns the required-arguments result naming every field in
// names when any of those struct fields is empty. Callers that need
// first-missing wording name one field at a time.
func requireFields(args any, names ...string) (mcp.ToolResult, bool) {
	value := reflect.ValueOf(args)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return mcp.ToolResult{}, false
	}
	for _, name := range names {
		if jsonFieldEmpty(value, name) {
			return requiredArgsResult(names...), true
		}
	}
	return mcp.ToolResult{}, false
}

func jsonFieldEmpty(value reflect.Value, name string) bool {
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		if tag == "" || strings.SplitN(tag, ",", 2)[0] != name {
			continue
		}
		return value.Field(i).IsZero()
	}
	return true
}
