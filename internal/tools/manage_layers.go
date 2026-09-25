package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageLayers() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_layers",
		Title:       "Manage Layers",
		Description: "List/set named layer definitions in project",
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
					"enum":        []string{"list", "set"},
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
		OutputSchema: actionOutputSchema(
			map[string]any{
				"layerType": map[string]any{"type": "string"},
				"layer":     map[string]any{"type": "integer"},
				"name":      map[string]any{"type": "string"},
			},
			payloadOutputSchema(map[string]any{
				"layers": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":  map[string]any{"type": "string"},
							"layer": map[string]any{"type": "integer"},
							"name":  map[string]any{"type": "string"},
						},
					},
				},
			}),
		),
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

// layerDefinitionKeyPattern matches a layer_names setting key such as
// layer_names/2d_render/layer_1. The value is read from the typed Config, so
// only the key shape is matched here.
var layerDefinitionKeyPattern = regexp.MustCompile(`^layer_names/([\w_]+)/layer_(\d+)$`)

func handleManageLayers(_ context.Context, raw json.RawMessage) (any, error) {
	var args manageLayersArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
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

	switch args.Action {
	case "list":
		layers := make([]layerDefinition, 0)
		for _, kv := range store.Keys("layer_names") {
			match := layerDefinitionKeyPattern.FindStringSubmatch(kv.Key)
			if match == nil {
				continue
			}
			layer, convErr := strconv.Atoi(match[2])
			if convErr != nil {
				continue
			}
			name, ok := kv.Value.(godotconfig.String)
			if !ok {
				continue
			}
			layers = append(layers, layerDefinition{Type: match[1], Layer: layer, Name: string(name)})
		}
		encoded, err := json.MarshalIndent(manageLayersListResult{Layers: layers}, "", "  ")
		if err != nil {
			return mcp.ErrorResult("manage_layers failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		return mcp.JSONResult(string(encoded)), nil
	case "set":
		if args.LayerType == "" || args.Layer == 0 || args.Name == "" {
			return mcp.ErrorResult("layerType, layer, and name are required for set."), nil
		}
		key := fmt.Sprintf("layer_names/%s/layer_%d", args.LayerType, args.Layer)
		store.Set("layer_names", key, godotconfig.String(args.Name))
		if err := store.Save(); err != nil {
			return mcp.ErrorResult("manage_layers failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
		}
		settingLine := godotconfig.EncodePropertyName(key) + "=" + godotconfig.FormatValue(godotconfig.String(args.Name))
		return statusResult("Layer set: "+settingLine, map[string]any{
			"layerType": args.LayerType,
			"layer":     args.Layer,
			"name":      args.Name,
		}), nil
	}
	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}
