package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func attachScript() mcp.Tool {
	return mcp.Tool{
		Name:        "attach_script",
		Title:       "Attach Script",
		Description: "Attach a GDScript to a scene node, editing the scene file in place",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to the project root)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node within the scene, relative to the scene root (e.g., "Player" or "Player/Sprite2D")`,
				},
				"scriptPath": map[string]any{
					"type":        "string",
					"description": "Path to the .gd script file (relative to the project root)",
				},
			},
			"required": []string{"scenePath", "nodePath", "scriptPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":  map[string]any{"type": "string"},
			"nodePath":   map[string]any{"type": "string"},
			"scriptPath": map[string]any{"type": "string"},
		}),
		Handler: handleAttachScript,
	}
}

type attachScriptArgs struct {
	ScenePath  string `json:"scenePath"`
	NodePath   string `json:"nodePath"`
	ScriptPath string `json:"scriptPath"`
}

func handleAttachScript(ctx context.Context, raw json.RawMessage) (any, error) {
	var args attachScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodePath == "" || args.ScriptPath == "" {
		return requiredArgsResult("scenePath", "nodePath", "scriptPath"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	sceneFull, sceneErr := confinePath(project, args.ScenePath)
	if sceneErr != nil {
		return invalidPathResultFor(sceneErr), nil
	}
	if _, scriptErr := confinePath(project, args.ScriptPath); scriptErr != nil {
		return invalidPathResultFor(scriptErr), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"scriptPath": args.ScriptPath,
	}
	result, err := runHeadlessOperation(ctx, project, "attach_script", params, map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"scriptPath": args.ScriptPath,
	})
	// The bridge registers the script's class_name synchronously, so a following
	// class-name lookup sees it without a respawn.
	return result, err
}
