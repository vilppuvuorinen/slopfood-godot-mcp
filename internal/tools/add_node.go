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
		Description: "Add a built-in node or a scene instance to an existing scene (rewrites the .tscn file)",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Path to the scene file (relative to project)",
				},
				"parentNodePath": map[string]any{
					"type":        "string",
					"description": `Path to the parent node, relative to the scene root (e.g., "root" or "root/Player")`,
				},
				"nodeType": map[string]any{
					"type":        "string",
					"description": "Type of node to add (e.g., Sprite2D, CollisionShape2D); provide exactly one of nodeType or instancePath",
				},
				"instancePath": map[string]any{
					"type":        "string",
					"description": "Path to a scene file to instance as the new node (relative to project); provide exactly one of nodeType or instancePath",
				},
				"nodeName": map[string]any{
					"type":        "string",
					"description": "Name for the new node; if a sibling already uses it, Godot renames the new node (e.g. @Node2D@2)",
				},
				"properties": propertiesSchema("Optional properties to set on the node, as name to godotValue pairs"),
			},
			"required": []string{"scenePath", "nodeName"},
			"oneOf": []any{
				map[string]any{"required": []string{"nodeType"}},
				map[string]any{"required": []string{"instancePath"}},
			},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scenePath":      map[string]any{"type": "string"},
			"nodeName":       map[string]any{"type": "string"},
			"nodeType":       map[string]any{"type": "string"},
			"instancePath":   map[string]any{"type": "string"},
			"parentNodePath": map[string]any{"type": "string"},
		}),
		Handler: handleAddNode,
	}
}

type addNodeArgs struct {
	ScenePath      string         `json:"scenePath"`
	ParentNodePath string         `json:"parentNodePath"`
	NodeType       string         `json:"nodeType"`
	InstancePath   string         `json:"instancePath"`
	NodeName       string         `json:"nodeName"`
	Properties     map[string]any `json:"properties"`
}

func handleAddNode(ctx context.Context, raw json.RawMessage) (any, error) {
	var args addNodeArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" || args.NodeName == "" {
		return requiredArgsResult("scenePath", "nodeName"), nil
	}
	if (args.NodeType == "") == (args.InstancePath == "") {
		return mcp.ErrorResult(
			"Provide exactly one of nodeType or instancePath.",
			"Set nodeType to add a built-in node, or instancePath to add an instance of a scene",
		), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return invalidPathResult(), nil
	}
	if args.NodeType != "" && !validClassName(args.NodeType) {
		return mcp.ErrorResult(
			"Invalid nodeType",
			"nodeType must be a built-in Godot class name (no paths, no file extensions)",
		), nil
	}

	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{
		"scenePath": args.ScenePath,
		"nodeName":  args.NodeName,
	}
	fields := map[string]any{
		"scenePath": args.ScenePath,
		"nodeName":  args.NodeName,
	}
	if args.NodeType != "" {
		params["nodeType"] = args.NodeType
		fields["nodeType"] = args.NodeType
	}
	if args.InstancePath != "" {
		instanceFull, instanceOK := confinePath(project, args.InstancePath)
		if !validPath(project) || !instanceOK {
			return invalidPathResult(), nil
		}
		if !pathExists(instanceFull) {
			return instanceMissingResult(args.InstancePath), nil
		}
		params["instancePath"] = args.InstancePath
		fields["instancePath"] = args.InstancePath
	}
	if args.ParentNodePath != "" {
		params["parentNodePath"] = args.ParentNodePath
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}

	stdout, stderr, exitCode, err := runOperation(ctx, project, "add_node", params)
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

	var message string
	if args.InstancePath != "" {
		message = fmt.Sprintf("Scene '%s' instanced as '%s' added successfully to '%s'.\n\nOutput: %s", args.InstancePath, args.NodeName, args.ScenePath, stdout)
	} else {
		message = fmt.Sprintf("Node '%s' of type '%s' added successfully to '%s'.\n\nOutput: %s", args.NodeName, args.NodeType, args.ScenePath, stdout)
	}
	if args.ParentNodePath != "" {
		fields["parentNodePath"] = args.ParentNodePath
	}
	return statusResult(project, message, fields), nil
}
