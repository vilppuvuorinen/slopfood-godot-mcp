package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageInputMap() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_input_map",
		Description: "Add, remove, or list input actions and bindings",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
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

// inputMapDeadzonePattern extracts the deadzone out of an existing action block.
var inputMapDeadzonePattern = regexp.MustCompile(`"deadzone"\s*:\s*([\d.eE+-]+)`)

// inputMapEventsPattern extracts the events array out of an existing action block.
var inputMapEventsPattern = regexp.MustCompile(`"events"\s*:\s*\[([\s\S]*)\]\s*\}\s*$`)

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

	content, failure := readProjectGodot(args.ProjectPath)
	if failure != nil {
		return *failure, nil
	}

	switch args.Action {
	case "list":
		actions := map[string]string{}
		if input, ok := parseProjectSections(content)["input"]; ok {
			actions = input
		}
		data, err := json.MarshalIndent(actions, "", "  ")
		if err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(string(data)), nil
	case "add":
		if args.ActionName == "" {
			return mcp.ErrorResult("actionName is required for add action."), nil
		}
		deadzone := 0.5
		if args.Deadzone != nil {
			deadzone = *args.Deadzone
		}
		deadzoneString := strconv.FormatFloat(deadzone, 'g', -1, 64)
		escapedName := regexp.QuoteMeta(args.ActionName)

		newEventObj := ""
		scancode := 0
		if args.Key != "" {
			scancode = keyNameToScancode(args.Key)
			newEventObj = fmt.Sprintf(`Object(InputEventKey,"resource_local_to_scene":false,"resource_name":"","device":-1,"window_id":0,"alt_pressed":false,"shift_pressed":false,"ctrl_pressed":false,"meta_pressed":false,"pressed":false,"keycode":0,"physical_keycode":%d,"key_label":0,"unicode":0,"location":0,"echo":false,"script":null)`, scancode)
		}

		existingActionPattern := regexp.MustCompile(`(?m)^` + escapedName + `\s*=\s*\{[\s\S]*?\}\s*$`)
		if loc := existingActionPattern.FindStringIndex(content); loc != nil {
			block := content[loc[0]:loc[1]]
			existingDeadzone := deadzoneString
			if match := inputMapDeadzonePattern.FindStringSubmatch(block); match != nil {
				existingDeadzone = match[1]
			}
			existingEvents := ""
			if match := inputMapEventsPattern.FindStringSubmatch(block); match != nil {
				existingEvents = match[1]
			}
			mergedEvents := existingEvents
			if newEventObj != "" {
				duplicatePattern := regexp.MustCompile(`"physical_keycode"\s*:\s*` + strconv.Itoa(scancode) + `\b`)
				if !duplicatePattern.MatchString(existingEvents) {
					if strings.TrimSpace(existingEvents) != "" {
						mergedEvents = existingEvents + ", " + newEventObj
					} else {
						mergedEvents = newEventObj
					}
				}
			}
			newBlock := ""
			if strings.TrimSpace(mergedEvents) != "" {
				newBlock = args.ActionName + `={"deadzone": ` + existingDeadzone + `, "events": [` + mergedEvents + `]}`
			} else {
				newBlock = args.ActionName + `={"deadzone": ` + existingDeadzone + `}`
			}
			content = content[:loc[0]] + newBlock + content[loc[1]:]
			if err := writeProjectGodot(args.ProjectPath, content); err != nil {
				return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
			}
			return mcp.TextResult(fmt.Sprintf(`Input action "%s" updated (event merged into existing action).`, args.ActionName)), nil
		}

		events := ""
		if newEventObj != "" {
			events = `, "events": [` + newEventObj + `]`
		}
		inputLine := args.ActionName + `={"deadzone": ` + deadzoneString + events + `}`
		if idx := strings.Index(content, "[input]"); idx != -1 {
			content = content[:idx] + "[input]\n\n" + inputLine + content[idx+len("[input]"):]
		} else {
			content += "\n[input]\n\n" + inputLine + "\n"
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(fmt.Sprintf(`Input action "%s" added.`, args.ActionName)), nil
	case "remove":
		if args.ActionName == "" {
			return mcp.ErrorResult("actionName is required for remove action."), nil
		}
		pattern := regexp.MustCompile(`\n?` + regexp.QuoteMeta(args.ActionName) + `\s*=.*\n?`)
		content = pattern.ReplaceAllString(content, "\n")
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("Failed to manage input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(fmt.Sprintf(`Input action "%s" removed.`, args.ActionName)), nil
	}
	return mcp.ErrorResult(`Invalid action. Use "list", "add", or "remove".`), nil
}

// keyNameToScancode maps a human-readable key name to a Godot scancode,
// mirroring the reference server's keyNameToScancode.
func keyNameToScancode(key string) int {
	scancodes := map[string]int{
		"A": 65, "B": 66, "C": 67, "D": 68, "E": 69, "F": 70, "G": 71, "H": 72,
		"I": 73, "J": 74, "K": 75, "L": 76, "M": 77, "N": 78, "O": 79, "P": 80,
		"Q": 81, "R": 82, "S": 83, "T": 84, "U": 85, "V": 86, "W": 87, "X": 88,
		"Y": 89, "Z": 90, "SPACE": 32, "ENTER": 16777221, "ESCAPE": 16777217,
		"TAB": 16777218, "BACKSPACE": 16777220, "UP": 16777232, "DOWN": 16777234,
		"LEFT": 16777231, "RIGHT": 16777233, "SHIFT": 16777237, "CTRL": 16777238,
		"ALT": 16777240, "F1": 16777244, "F2": 16777245, "F3": 16777246,
		"F4": 16777247, "F5": 16777248, "F6": 16777249, "F7": 16777250,
		"F8": 16777251, "F9": 16777252, "F10": 16777253, "F11": 16777254,
		"F12": 16777255,
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
