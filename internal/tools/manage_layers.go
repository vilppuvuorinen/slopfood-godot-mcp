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

func manageLayers() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_layers",
		Description: "List/set named layer definitions in project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: list or set",
				},
				"layerType": map[string]any{
					"type":        "string",
					"description": "Type: render, physics_2d, physics_3d, navigation",
				},
				"layer": map[string]any{
					"type":        "number",
					"description": "Layer number (1-32)",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Layer name",
				},
			},
			"required": []string{"projectPath", "action"},
		},
		Handler: handleManageLayers,
	}
}

type manageLayersArgs struct {
	ProjectPath string `json:"projectPath"`
	Action      string `json:"action"`
	LayerType   string `json:"layerType"`
	Layer       int    `json:"layer"`
	Name        string `json:"name"`
}

// layerDefinition is one parsed layer_names entry. Field order matches the
// reference server's JSON.stringify output.
type layerDefinition struct {
	Type  string `json:"type"`
	Layer int    `json:"layer"`
	Name  string `json:"name"`
}

// manageLayersListResult is the list action's JSON payload.
type manageLayersListResult struct {
	Layers []layerDefinition `json:"layers"`
}

// layerDefinitionPattern matches a project.godot layer_names setting such as
// layer_names/2d_render/layer_1="Ground". The value group accepts escaped
// quotes so names survive a round trip.
var layerDefinitionPattern = regexp.MustCompile(`layer_names/([\w_]+)/layer_(\d+)="((?:[^"\\]|\\.)*)"`)

func handleManageLayers(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, settingsAliases)
	if err != nil {
		return nil, err
	}
	var args manageLayersArgs
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
		matches := layerDefinitionPattern.FindAllStringSubmatch(content, -1)
		layers := make([]layerDefinition, 0, len(matches))
		for _, match := range matches {
			layer, convErr := strconv.Atoi(match[2])
			if convErr != nil {
				continue
			}
			layers = append(layers, layerDefinition{Type: match[1], Layer: layer, Name: godotconfig.UnescapeString(match[3])})
		}
		encoded, err := json.MarshalIndent(manageLayersListResult{Layers: layers}, "", "  ")
		if err != nil {
			return mcp.ErrorResult("manage_layers failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult(string(encoded)), nil
	case "set":
		if args.LayerType == "" || args.Layer == 0 || args.Name == "" {
			return mcp.ErrorResult("layerType, layer, and name are required for set."), nil
		}
		key := fmt.Sprintf("layer_names/%s/layer_%d", args.LayerType, args.Layer)
		settingLine := godotconfig.EncodePropertyName(key) + "=" + godotconfig.FormatValue(godotconfig.String(args.Name))
		if updated, ok := setSectionValue(content, "layer_names", key, godotconfig.String(args.Name)); ok {
			content = updated
		} else {
			if !strings.Contains(content, "[layer_names]") {
				content += "\n[layer_names]\n"
			}
			content = strings.Replace(content, "[layer_names]", "[layer_names]\n"+settingLine, 1)
		}
		if err := writeProjectGodot(args.ProjectPath, content); err != nil {
			return mcp.ErrorResult("manage_layers failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.TextResult("Layer set: " + settingLine), nil
	}
	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
