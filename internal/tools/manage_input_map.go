package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageInputMap() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_input_map",
		Title:       "Manage Input Map",
		Description: "Add, remove, or list input actions and bindings",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"list", "add", "remove"},
					"description": `"list", "add", or "remove"`,
				},
				"actionName": map[string]any{
					"type":        "string",
					"description": "Input action name (required for add/remove)",
				},
				"key": map[string]any{
					"type":        "string",
					"description": `Key to bind (for add, e.g. "W", "Space")`,
				},
				"deadzone": map[string]any{
					"type":        "number",
					"description": "Deadzone for the action. Default: 0.5",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"actionName": map[string]any{"type": "string"},
			"key":        map[string]any{"type": "string"},
			"deadzone":   map[string]any{"type": "number"},
		}),
		Handler: handleManageInputMap,
	}
}

type manageInputMapArgs struct {
	ProjectPath string   `json:"projectPath"`
	Action      string   `json:"action"`
	ActionName  string   `json:"actionName"`
	Key         string   `json:"key"`
	Deadzone    *float64 `json:"deadzone"`
}

// readInputAction returns the dictionary of an existing input action. found is
// false when the action is absent; err is non-nil when it exists but is not a
// dictionary, so callers never overwrite it blindly.
func readInputAction(store *settingsStore, name string) (godotconfig.Dictionary, bool, error) {
	value, ok := store.Get("input", name)
	if !ok {
		return godotconfig.Dictionary{}, false, nil
	}
	action, ok := value.(godotconfig.Dictionary)
	if !ok {
		return godotconfig.Dictionary{}, false, fmt.Errorf("action %q is not a dictionary", name)
	}
	return action, true, nil
}

// inputEvent builds the Object(InputEventKey, ...) literal for a scancode in
// the given schema's serialization. Only Godot 4.x is implemented; an
// unsupported schema yields "" so no event is written.
func inputEvent(schema configSchema, scancode int) string {
	switch schema {
	case schemaGodot4:
		return fmt.Sprintf(`Object(InputEventKey,"resource_local_to_scene":false,"resource_name":"","device":-1,"window_id":0,"alt_pressed":false,"shift_pressed":false,"ctrl_pressed":false,"meta_pressed":false,"pressed":false,"keycode":0,"physical_keycode":%d,"key_label":0,"unicode":0,"location":0,"echo":false,"script":null)`, scancode)
	default:
		return ""
	}
}

// mergeInputAction appends a new InputEventKey to an existing action unless
// that scancode is already bound, preserving every other entry (including
// event constructors the parser only captures as Opaque).
func mergeInputAction(action godotconfig.Dictionary, newEvent string, scancode int) godotconfig.Dictionary {
	if newEvent == "" {
		return action
	}
	eventsIndex := -1
	var events godotconfig.Array
	for i, entry := range action.Entries {
		if key, ok := entry.Key.(godotconfig.String); ok && string(key) == "events" {
			eventsIndex = i
			if arr, ok := entry.Value.(godotconfig.Array); ok {
				events = arr
			}
			break
		}
	}
	duplicate := regexp.MustCompile(`"physical_keycode"\s*:\s*` + strconv.Itoa(scancode) + `\b`)
	for _, event := range events {
		if opaque, ok := event.(godotconfig.Opaque); ok && duplicate.MatchString(string(opaque)) {
			return action
		}
	}
	events = append(events, godotconfig.Opaque(newEvent))
	if eventsIndex >= 0 {
		action.Entries[eventsIndex].Value = events
	} else {
		action.Entries = append(action.Entries, godotconfig.DictEntry{
			Key:   godotconfig.String("events"),
			Value: events,
		})
	}
	return action
}

func handleManageInputMap(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args manageInputMapArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath and action are required."), nil
	}

	store, failure := openSettings(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}
	schema := store.schema

	switch args.Action {
	case "list":
		actions, err := store.SectionValues("input")
		if err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		data, err := json.MarshalIndent(actions, "", "  ")
		if err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.JSONResult(string(data)), nil
	case "add":
		if args.ActionName == "" {
			return mcp.ErrorResult("actionName is required for add action."), nil
		}
		deadzone := 0.5
		if args.Deadzone != nil {
			deadzone = *args.Deadzone
		}

		newEvent := ""
		scancode := 0
		if args.Key != "" {
			scancode = keyNameToScancode(schema, args.Key)
			newEvent = inputEvent(schema, scancode)
		}

		existing, found, parseErr := readInputAction(store, args.ActionName)
		if parseErr != nil {
			return mcp.ErrorResult("Failed to manage input map: " + parseErr.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		if found {
			store.Set("input", args.ActionName, mergeInputAction(existing, newEvent, scancode))
			if err := store.Save(); err != nil {
				return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
			}
			return statusResult(fmt.Sprintf(`Input action "%s" updated (event merged into existing action).`, args.ActionName), map[string]any{
				"actionName": args.ActionName,
			}), nil
		}

		action := godotconfig.Dictionary{Entries: []godotconfig.DictEntry{
			{Key: godotconfig.String("deadzone"), Value: godotconfig.Float(deadzone)},
		}}
		if newEvent != "" {
			action.Entries = append(action.Entries, godotconfig.DictEntry{
				Key:   godotconfig.String("events"),
				Value: godotconfig.Array{godotconfig.Opaque(newEvent)},
			})
		}
		store.Set("input", args.ActionName, action)
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult(fmt.Sprintf(`Input action "%s" added.`, args.ActionName), map[string]any{
			"actionName": args.ActionName,
		}), nil
	case "remove":
		if args.ActionName == "" {
			return mcp.ErrorResult("actionName is required for remove action."), nil
		}
		store.Delete("input", args.ActionName)
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return statusResult(fmt.Sprintf(`Input action "%s" removed.`, args.ActionName), map[string]any{
			"actionName": args.ActionName,
		}), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "list", "add", or "remove".`), nil
}

// keySpecialGodot4 is Godot 4's KEY_SPECIAL bit (1<<22). Special keys are the
// bit OR-ed with an offset from core/os/keyboard.h; Godot 3 used 1<<24, so
// reusing its values would write invalid physical_keycodes.
const keySpecialGodot4 = 1 << 22

// keyNameToScancode maps a human-readable key name to a scancode for the given
// schema. Only the Godot 4.x schema is implemented; an unsupported schema
// yields 0 so no binding is written.
func keyNameToScancode(schema configSchema, key string) int {
	switch schema {
	case schemaGodot4:
		return keyNameToScancodeGodot4(key)
	default:
		return 0
	}
}

// keyNameToScancodeGodot4 maps a key name to its Godot 4 Key value. Letters and
// digits are their ASCII code; special keys combine keySpecialGodot4 with the
// offset Godot assigns in keyboard.h.
func keyNameToScancodeGodot4(key string) int {
	scancodes := map[string]int{
		"A": 65, "B": 66, "C": 67, "D": 68, "E": 69, "F": 70, "G": 71, "H": 72,
		"I": 73, "J": 74, "K": 75, "L": 76, "M": 77, "N": 78, "O": 79, "P": 80,
		"Q": 81, "R": 82, "S": 83, "T": 84, "U": 85, "V": 86, "W": 87, "X": 88,
		"Y": 89, "Z": 90, "SPACE": 32,
		"ESCAPE": keySpecialGodot4 | 0x01, "TAB": keySpecialGodot4 | 0x02,
		"BACKSPACE": keySpecialGodot4 | 0x04, "ENTER": keySpecialGodot4 | 0x05,
		"LEFT": keySpecialGodot4 | 0x0F, "UP": keySpecialGodot4 | 0x10,
		"RIGHT": keySpecialGodot4 | 0x11, "DOWN": keySpecialGodot4 | 0x12,
		"SHIFT": keySpecialGodot4 | 0x15, "CTRL": keySpecialGodot4 | 0x16,
		"ALT": keySpecialGodot4 | 0x18,
		"F1":  keySpecialGodot4 | 0x1C, "F2": keySpecialGodot4 | 0x1D,
		"F3": keySpecialGodot4 | 0x1E, "F4": keySpecialGodot4 | 0x1F,
		"F5": keySpecialGodot4 | 0x20, "F6": keySpecialGodot4 | 0x21,
		"F7": keySpecialGodot4 | 0x22, "F8": keySpecialGodot4 | 0x23,
		"F9": keySpecialGodot4 | 0x24, "F10": keySpecialGodot4 | 0x25,
		"F11": keySpecialGodot4 | 0x26, "F12": keySpecialGodot4 | 0x27,
	}
	upper := strings.ToUpper(key)
	if code, ok := scancodes[upper]; ok {
		return code
	}
	if len(key) == 1 {
		return int(key[0])
	}
	return 0
}
