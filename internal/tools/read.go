package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

var readTypes = []string{"scene", "animation", "tileset", "theme", "resource"}

// The four resource types dispatch straight to the bridge op; only the key is
// remapped (path -> resourcePath).
var readResourceOperations = map[string]string{
	"animation": "read_animation",
	"tileset":   "read_tileset",
	"theme":     "read_theme",
	"resource":  "read_resource",
}

func (t *toolset) read() mcp.Tool {
	return mcp.Tool{
		Name:         "read",
		Title:        "Read",
		Description:  "Read a scene, animation, tileset, theme, or binary resource file; use read_node to read a node subtree. depth applies to scenes, propertyFilter to resources",
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		OutputSchema: readEnvelopeSchema(readTypes, false),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        readTypes,
					"description": "Kind of file to read",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File path (relative to the project root, or a res:// path)",
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the scene root; 0 returns the root without children, omitted means no limit; scene only",
				},
				"propertyFilter": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Limit the returned properties to these names (wildcards * and ? allowed); resource only",
				},
			},
			"required": []string{"type", "path"},
		},
		Handler: t.handleRead,
	}
}

type readArgs struct {
	Type           string   `json:"type"`
	Path           string   `json:"path"`
	Depth          *int     `json:"depth"`
	PropertyFilter []string `json:"propertyFilter"`
}

func (t *toolset) handleRead(ctx context.Context, raw json.RawMessage) (any, error) {
	// Presence, not value: an explicit null and an absent key both nil the
	// typed fields, so mismatched optional args are detected from the raw map.
	args, present, err := decodeArgs[readArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields(args, "type"); missing {
		return result, nil
	}
	if !slices.Contains(readTypes, args.Type) {
		return mcp.ErrorResult(
			"Invalid type: "+args.Type+".",
			"type must be one of "+strings.Join(readTypes, ", "),
		), nil
	}
	if result, missing := requireFields(args, "path"); missing {
		return result, nil
	}
	if _, ok := present["nodePath"]; ok {
		return mcp.ErrorResult(
			"read does not accept nodePath.",
			"Use read_node to read a node subtree",
		), nil
	}
	if _, ok := present["depth"]; ok && args.Type != "scene" {
		return mcp.ErrorResult(
			"depth applies only to type=scene.",
			"Omit depth for this type, or use type=scene",
		), nil
	}
	if _, ok := present["propertyFilter"]; ok && args.Type != "resource" {
		return mcp.ErrorResult(
			"propertyFilter applies only to type=resource.",
			"Omit propertyFilter for this type, or use type=resource",
		), nil
	}
	if args.Type == "scene" {
		if args.Depth != nil && *args.Depth < 0 {
			return mcp.ErrorResult(
				"depth must be zero or greater.",
				"Omit depth to return the whole subtree",
			), nil
		}
		return t.readSceneThrough(ctx, args.Path, "", args.Depth)
	}

	if _, err := normalizeReadPath(project, args.Path); err != nil {
		return invalidPathResultFor(err), nil
	}
	if result, ok := requireProject(project); !ok {
		return result, nil
	}
	if args.Type == "resource" && strings.HasSuffix(strings.ToLower(args.Path), ".tres") {
		return mcp.ErrorResult(
			"read(type=resource) reads binary .res resources; "+args.Path+" is a text .tres file.",
			"Read .tres files directly",
		), nil
	}

	params := map[string]any{"resourcePath": args.Path}
	fields := map[string]any{"kind": args.Type, "path": args.Path}
	if args.Type == "resource" && len(args.PropertyFilter) > 0 {
		params["propertyFilter"] = args.PropertyFilter
	}
	return t.runHeadlessOperation(ctx, readResourceOperations[args.Type], params, fields)
}
