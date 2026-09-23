package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func validateScript() mcp.Tool {
	return mcp.Tool{
		Name:        "validate_script",
		Description: "Check a GDScript file for syntax/type errors (headless, no run)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot project path",
				},
				"scriptPath": map[string]any{
					"type":        "string",
					"description": `GDScript file path relative to project (e.g. "scripts/player.gd")`,
				},
			},
			"required": []string{"projectPath", "scriptPath"},
		},
		Handler: handleValidateScript,
	}
}

type validateScriptArgs struct {
	ProjectPath string `json:"projectPath"`
	ScriptPath  string `json:"scriptPath"`
}

// validateScriptResult is the pretty-printed JSON payload validate_script
// returns, ordered like the reference server's object literal.
type validateScriptResult struct {
	Valid      bool               `json:"valid"`
	ScriptPath string             `json:"scriptPath"`
	ErrorCount int                `json:"errorCount"`
	Errors     []scriptDiagnostic `json:"errors"`
}

func handleValidateScript(ctx context.Context, raw json.RawMessage) (any, error) {
	raw, err := aliasParams(raw, parameterMappings)
	if err != nil {
		return nil, err
	}
	var args validateScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScriptPath == "" {
		return mcp.ErrorResult("projectPath and scriptPath are required."), nil
	}
	scriptFull, pathOK := confinePath(args.ProjectPath, args.ScriptPath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !strings.HasSuffix(strings.ToLower(args.ScriptPath), ".gd") {
		return mcp.ErrorResult("validate_script only checks GDScript (.gd) files."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !pathExists(scriptFull) {
		return mcp.ErrorResult("Script does not exist: " + args.ScriptPath), nil
	}
	diagnostics, err := runGDScriptCheck(ctx, args.ProjectPath, scriptFull)
	if err != nil {
		return mcp.ErrorResult("validate_script could not check the script; " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	data, err := json.MarshalIndent(validateScriptResult{
		Valid:      len(diagnostics) == 0,
		ScriptPath: args.ScriptPath,
		ErrorCount: len(diagnostics),
		Errors:     diagnostics,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode validate_script result: %w", err)
	}
	return mcp.TextResult(string(data)), nil
}
