package tools

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

var readNodeTypes = []string{"scene", "tilemap"}

func readNode() mcp.Tool {
	return mcp.Tool{
		Name:         "read_node",
		Title:        "Read Node",
		Description:  "Read a node subtree from a scene, or the cells of a TileMapLayer node; use read to read whole files",
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		OutputSchema: readEnvelopeSchema(readNodeTypes, true),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        readNodeTypes,
					"description": "Kind of node read",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to the project root, or a res:// path)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node within the scene, relative to the scene root (e.g., "Player" or "Player/Sprite2D")`,
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the node; 0 returns the node without children, omitted means no limit; scene only",
				},
			},
			"required": []string{"type", "path", "nodePath"},
		},
		Handler: withContext(handleReadNode),
	}
}

type readNodeArgs struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	NodePath string `json:"nodePath"`
	Depth    *int   `json:"depth"`
}

func handleReadNode(ctx Context, raw json.RawMessage) (any, error) {
	var args readNodeArgs
	// Presence, not value: an explicit null and an absent key both nil the
	// typed depth field, so depth-on-tilemap is detected from the raw map.
	var present map[string]json.RawMessage
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if err := json.Unmarshal(raw, &present); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Type == "" {
		return requiredArgsResult("type"), nil
	}
	if !slices.Contains(readNodeTypes, args.Type) {
		return mcp.ErrorResult(
			"Invalid type: "+args.Type+".",
			"type must be one of "+strings.Join(readNodeTypes, ", "),
		), nil
	}
	if args.Path == "" {
		return requiredArgsResult("path"), nil
	}
	if args.NodePath == "" {
		return requiredArgsResult("nodePath"), nil
	}
	if _, ok := present["depth"]; ok && args.Type == "tilemap" {
		return mcp.ErrorResult(
			"depth applies only to type=scene.",
			"Omit depth for this type, or use type=scene",
		), nil
	}
	if args.Type == "scene" {
		if args.Depth != nil && *args.Depth < 0 {
			return mcp.ErrorResult(
				"depth must be zero or greater.",
				"Omit depth to return the whole subtree",
			), nil
		}
		return readSceneThrough(ctx, project, args.Path, args.NodePath, args.Depth)
	}

	if _, result, ok := ensureReadableScene(project, args.Path); !ok {
		return result, nil
	}
	params := map[string]any{
		"scenePath": args.Path,
		"nodePath":  args.NodePath,
	}
	fields := map[string]any{
		"kind":     args.Type,
		"path":     args.Path,
		"nodePath": args.NodePath,
	}
	return runHeadlessOperation(ctx, project, "read_tilemap", params, fields)
}
