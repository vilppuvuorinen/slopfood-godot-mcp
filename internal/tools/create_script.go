package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func createScript() mcp.Tool {
	return mcp.Tool{
		Name:        "create_script",
		Description: "Create a GDScript file from a template",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scriptPath": map[string]any{
					"type":        "string",
					"description": "Script file path (relative to project)",
				},
				"extends": map[string]any{
					"type":        "string",
					"description": "Base class to extend. Default: Node",
				},
				"className": map[string]any{
					"type":        "string",
					"description": "Optional class_name",
				},
				"methods": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Method stubs to include",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Full source code (overrides template)",
				},
			},
			"required": []string{"projectPath", "scriptPath"},
		},
		Handler: handleCreateScript,
	}
}

type createScriptArgs struct {
	ProjectPath string   `json:"projectPath"`
	ScriptPath  string   `json:"scriptPath"`
	Extends     string   `json:"extends"`
	ClassName   string   `json:"className"`
	Methods     []string `json:"methods"`
	Source      string   `json:"source"`
}

func handleCreateScript(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args createScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScriptPath == "" {
		return mcp.ErrorResult("projectPath and scriptPath are required."), nil
	}
	fullPath, pathOK := confinePath(args.ProjectPath, args.ScriptPath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	source := args.Source
	if source == "" {
		source = buildScriptTemplate(args)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil { //nolint:gosec // scriptPath is caller-controlled.
		return mcp.ErrorResult("create_script failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if err := os.WriteFile(fullPath, []byte(source), 0o644); err != nil { //nolint:gosec // user-facing script file.
		return mcp.ErrorResult("create_script failed: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.TextResult("Script created at " + args.ScriptPath), nil
}

// buildScriptTemplate assembles the reference server's GDScript template: an
// "extends" line, an optional "class_name" inserted after it, and an optional
// tab-indented pass body per method, joined by newlines with a trailing one.
func buildScriptTemplate(args createScriptArgs) string {
	ext := args.Extends
	if ext == "" {
		ext = "Node"
	}
	lines := []string{"extends " + ext, ""}
	if args.ClassName != "" {
		lines = slices.Insert(lines, 1, "class_name "+args.ClassName)
	}
	for _, method := range args.Methods {
		lines = append(lines, "", "func "+method+"():", "\tpass")
	}
	return strings.Join(lines, "\n") + "\n"
}
