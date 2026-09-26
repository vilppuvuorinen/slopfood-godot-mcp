package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readScene() mcp.Tool {
	return mcp.Tool{
		Name:        "read_scene",
		Title:       "Read Scene",
		Description: "Read scene file as JSON node tree (headless)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenePath": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to project)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": "Optional node path relative to the scene root; only this node's subtree is returned",
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the node; 0 returns the node without children, omitted means no limit",
				},
			},
			"required": []string{"scenePath"},
		},
		OutputSchema: readSceneOutputSchema(),
		Handler:      handleReadScene,
	}
}

// readSceneOutputSchema describes every read_scene result: a readable scene
// yields a node tree (the sceneNode shape), a scene the engine cannot
// instantiate yields its raw text so the caller can still inspect it, and an
// error yields the shared error envelope. children recurses through
// $defs/sceneNode, so any depth of tree is described, and readOutputSchema
// advertises the error branch alongside the two success shapes.
func readSceneOutputSchema() map[string]any {
	properties := sceneNodeProperties()
	properties["error"] = map[string]any{"type": "string"}
	properties["raw"] = map[string]any{"type": "string"}
	return readOutputSchema(map[string]any{
		"type":       "object",
		"properties": properties,
		// A readable scene yields a node tree, while a scene the engine
		// cannot instantiate yields its raw text so the caller can still
		// inspect it. The two shapes are mutually exclusive, so oneOf states
		// that a payload carrying both is invalid rather than valid twice.
		"oneOf": []any{
			map[string]any{"required": []string{"name", "type"}},
			map[string]any{"required": []string{"error", "raw"}},
		},
		"$defs": map[string]any{"sceneNode": sceneNodeSchema()},
	})
}

// sceneNodeSchema describes one node in read_scene's tree. children refers back
// to this definition so nested nodes share the same shape.
func sceneNodeSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": sceneNodeProperties(),
		"required":   []string{"name", "type"},
	}
}

// sceneNodeProperties is the property map of a scene node. children points at
// the sceneNode definition under the output schema's $defs for recursion.
func sceneNodeProperties() map[string]any {
	return map[string]any{
		"name":   map[string]any{"type": "string"},
		"type":   map[string]any{"type": "string"},
		"script": map[string]any{"type": "string", "description": "Script path, relative to the bound project"},
		"properties": map[string]any{
			"type":                 "object",
			"description":          "Node properties in the tool value contract",
			"additionalProperties": godotValueSchema("A node property value in the tool value contract"),
		},
		"groups": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"children": map[string]any{
			"type":        "array",
			"description": "Child nodes, each with the same shape",
			"items":       map[string]any{"$ref": "#/$defs/sceneNode"},
		},
	}
}

type readSceneArgs struct {
	ScenePath string `json:"scenePath"`
	NodePath  string `json:"nodePath"`
	Depth     *int   `json:"depth"`
}

func handleReadScene(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readSceneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ScenePath == "" {
		return requiredArgsResult("scenePath"), nil
	}
	if args.Depth != nil && *args.Depth < 0 {
		return mcp.ErrorResult(
			"depth must be zero or greater.",
			"Omit depth to return the whole subtree",
		), nil
	}
	sceneFull, pathOK := confinePath(project, args.ScenePath)
	if !validPath(project) || !pathOK {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if !pathExists(sceneFull) {
		return sceneMissingResult(args.ScenePath), nil
	}

	params := map[string]any{"scenePath": args.ScenePath}
	if args.NodePath != "" {
		params["nodePath"] = args.NodePath
	}
	if args.Depth != nil {
		params["depth"] = *args.Depth
	}
	stdout, stderr, exitCode, err := runOperation(ctx, project, "read_scene", params)
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to read scene: "+err.Error(),
			headlessSolutions...,
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to read scene: "+stderr,
			headlessSolutions...,
		), nil
	}

	if payload, ok := markerJSON(stdout); ok {
		if json.Valid([]byte(payload)) {
			return mcp.JSONResult(payload), nil
		}
		return readSceneRawResult("read_scene returned invalid JSON", payload, stderr), nil
	}
	return readSceneRawResult("read_scene returned no structured output", stdout, stderr), nil
}

// readSceneRawResult reports a read_scene run whose output did not yield a node
// tree: the raw engine output plus a reason. It fills the error/raw branch of
// readSceneOutputSchema, so a caller that advertised an output schema always
// receives structuredContent instead of a bare text result.
func readSceneRawResult(reason, raw, stderrText string) mcp.ToolResult {
	text := "Scene read output:\n" + raw
	if stderrText != "" {
		text += "\nErrors:\n" + stderrText
	}
	return mcp.StructuredResult(text, map[string]any{
		"error": reason,
		"raw":   raw,
	})
}
