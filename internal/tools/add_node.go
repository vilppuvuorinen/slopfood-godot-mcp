package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func addNode() mcp.Tool {
	return mcp.Tool{
		Name:        "add_node",
		Title:       "Add Node",
		Description: "Add a node to an existing scene",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path to the scene file (relative to project)",
				},
				"parentNodePath": map[string]any{
					"type":        "string",
					"description": `Path to the parent node (e.g., "root" or "root/Player")`,
				},
				"nodeType": map[string]any{
					"type":        "string",
					"description": "Type of node to add (e.g., Sprite2D, CollisionShape2D)",
				},
				"nodeName": map[string]any{
					"type":        "string",
					"description": "Name for the new node",
				},
				"properties": map[string]any{
					"type":        "object",
					"description": "Optional properties to set on the node",
				},
			},
			"required": []string{"projectPath", "scenePath", "nodeType", "nodeName"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":      map[string]any{"type": "string"},
			"nodeName":       map[string]any{"type": "string"},
			"nodeType":       map[string]any{"type": "string"},
			"parentNodePath": map[string]any{"type": "string"},
			"output":         map[string]any{"type": "string"},
		}),
		Handler: handleAddNode,
	}
}

type addNodeArgs struct {
	ProjectPath    string         `json:"projectPath"`
	ScenePath      string         `json:"scenePath"`
	ParentNodePath string         `json:"parentNodePath"`
	NodeType       string         `json:"nodeType"`
	NodeName       string         `json:"nodeName"`
	Properties     map[string]any `json:"properties"`
}

func handleAddNode(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args addNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScenePath == "" || args.NodeType == "" || args.NodeName == "" {
		return mcp.ErrorResult(
			"Missing required parameters",
			"Provide projectPath, scenePath, nodeType, and nodeName",
		), nil
	}
	sceneFull, pathOK := confinePath(args.ProjectPath, args.ScenePath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult(
			"Invalid path",
			`Provide valid paths without ".." or other potentially unsafe characters`,
		), nil
	}
	if !validClassName(args.NodeType) {
		return mcp.ErrorResult(
			"Invalid nodeType",
			"nodeType must be a built-in Godot class name (no paths, no file extensions)",
		), nil
	}

	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath": args.ScenePath,
		"nodeType":  args.NodeType,
		"nodeName":  args.NodeName,
	}
	if args.ParentNodePath != "" {
		params["parentNodePath"] = args.ParentNodePath
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	stdout, stderr, exitCode, err := runOperation(ctx, args.ProjectPath, "add_node", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to add node: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to add node: "+stderr,
			"Check if the node type is valid",
			"Ensure the parent node path exists",
			"Verify the scene file is valid",
		), nil
	}

	message := fmt.Sprintf("Node '%s' of type '%s' added successfully to '%s'.\n\nOutput: %s", args.NodeName, args.NodeType, args.ScenePath, stdout)
	fields := map[string]any{
		"scenePath": args.ScenePath,
		"nodeName":  args.NodeName,
		"nodeType":  args.NodeType,
		"output":    stdout,
	}
	if args.ParentNodePath != "" {
		fields["parentNodePath"] = args.ParentNodePath
	}
	return statusResult(message, fields), nil
}
