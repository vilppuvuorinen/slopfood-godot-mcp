package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func attachScript() mcp.Tool {
	return mcp.Tool{
		Name:        "attach_script",
		Title:       "Attach Script",
		Description: "Attach a GDScript to a scene node (headless)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), IdempotentHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node within the scene (e.g., "root/Player")`,
				},
				"scriptPath": map[string]any{
					"type":        "string",
					"description": "Path to the .gd script file (relative to project)",
				},
			},
			"required": []string{"projectPath", "scenePath", "nodePath", "scriptPath"},
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
	ProjectPath string `json:"projectPath"`
	ScenePath   string `json:"scenePath"`
	NodePath    string `json:"nodePath"`
	ScriptPath  string `json:"scriptPath"`
}

func handleAttachScript(ctx context.Context, raw json.RawMessage) (any, error) {
	var args attachScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.NodePath == "" || args.ScriptPath == "" {
		return mcp.ErrorResult("projectPath, scenePath, nodePath, and scriptPath are required."), nil
	}
	sceneFull, pathOK := confinePath(args.ProjectPath, args.ScenePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"scriptPath": args.ScriptPath,
	}
	return runHeadlessOperation(ctx, args.ProjectPath, "attach_script", params, map[string]any{
		"scenePath":  args.ScenePath,
		"nodePath":   args.NodePath,
		"scriptPath": args.ScriptPath,
	})
}
