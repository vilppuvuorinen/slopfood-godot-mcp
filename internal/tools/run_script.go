package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func runScript() mcp.Tool {
	return mcp.Tool{
		Name:        "run_script",
		Title:       "Run GDScript",
		Description: "Run a GDScript function body in the live editor session and return its output; the code runs with full access to the project and editor state",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"body": map[string]any{
					"type":        "string",
					"description": "Body of a synchronous GDScript function, indented as it would appear inside `func run():`. The server wraps it as `@tool\\nextends RefCounted\\n\\nfunc run():\\n<body>` and reports the function's return value",
				},
			},
			"required": []string{"body"},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"compiled": map[string]any{
				"type":        "boolean",
				"description": "Whether the generated run() compiled and instantiated; false means errors holds the parse/runtime diagnostics",
			},
			"result": godotValueSchema("Value returned by the generated run() function, or null when there is none"),
			"logs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Lines the script printed while it ran",
			},
			"errors": map[string]any{
				"type":        "array",
				"description": "Engine errors the script triggered while it ran",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"function":  map[string]any{"type": "string"},
						"file":      map[string]any{"type": "string"},
						"line":      map[string]any{"type": "integer"},
						"code":      map[string]any{"type": "string"},
						"rationale": map[string]any{"type": "string"},
					},
					"required": []string{"function", "file", "line", "code", "rationale"},
				},
			},
		}),
		Handler: withContext(handleRunScript),
	}
}

type runScriptArgs struct {
	Body string `json:"body"`
}

func handleRunScript(ctx Context, raw json.RawMessage) (any, error) {
	var args runScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Body == "" {
		return requiredArgsResult("body"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	stdout, stderr, exitCode, err := ctx.Run(project, "run_script", map[string]any{"body": args.Body})
	if err != nil {
		return mcp.ErrorResult( //nolint:nilerr // the failure is reported through the tool result.
			"Failed to run script: "+err.Error(),
			headlessSolutions...,
		), nil
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(
			"Failed to run script: "+stderr,
			headlessSolutions...,
		), nil
	}

	// The bridge reports compile and runtime failures inside the payload, so a
	// non-error result must still carry the declared shape.
	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}
	return mcp.ErrorResult(
		"Failed to run script: run_script returned no structured output: "+stdout,
		headlessSolutions...,
	), nil
}
