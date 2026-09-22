package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func manageShader() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_shader",
		Description: "Create or read .gdshader files",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"shaderPath": map[string]any{
					"type":        "string",
					"description": "Shader file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: create or read",
				},
				"shaderType": map[string]any{
					"type":        "string",
					"description": "Type: spatial, canvas_item, particles, sky",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Shader source code (for create)",
				},
			},
			"required": []string{"projectPath", "shaderPath", "action"},
		},
		Handler: handleManageShader,
	}
}

type manageShaderArgs struct {
	ProjectPath string `json:"projectPath"`
	ShaderPath  string `json:"shaderPath"`
	Action      string `json:"action"`
	ShaderType  string `json:"shaderType"`
	Source      string `json:"source"`
}

func handleManageShader(_ context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args manageShaderArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ShaderPath == "" || args.Action == "" {
		return mcp.ErrorResult("projectPath, shaderPath, and action are required."), nil
	}
	if !validPath(args.ProjectPath) || !validPath(args.ShaderPath) {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}

	fullPath := filepath.Join(args.ProjectPath, args.ShaderPath)
	switch args.Action {
	case "read":
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return mcp.ErrorResult("Shader not found: " + args.ShaderPath), nil
			}
			return manageShaderFailure(err), nil
		}
		return mcp.TextResult(string(data)), nil
	case "create":
		source := args.Source
		if source == "" {
			shaderType := args.ShaderType
			if shaderType == "" {
				shaderType = "spatial"
			}
			source = "shader_type " + shaderType + ";\n\nvoid fragment() {\n\t// Called for every pixel the material is visible on.\n}\n"
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil { //nolint:gosec // shaderPath is caller-controlled.
			return manageShaderFailure(err), nil
		}
		if err := os.WriteFile(fullPath, []byte(source), 0o644); err != nil { //nolint:gosec // user-facing shader file.
			return manageShaderFailure(err), nil
		}
		return mcp.TextResult("Shader created at " + args.ShaderPath), nil
	}
	return mcp.ErrorResult("Unknown action: " + args.Action), nil
}

// manageShaderFailure is the reference server's catch-all message for an
// unreadable or unwritable shader file.
func manageShaderFailure(err error) mcp.ToolResult {
	return mcp.ErrorResult("manage_shader failed: " + err.Error())
}
