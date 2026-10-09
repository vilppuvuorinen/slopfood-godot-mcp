package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func (t *toolset) runScript() mcp.Tool {
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
		Handler: t.handleRunScript,
	}
}

type runScriptArgs struct {
	Body string `json:"body"`
}

func (t *toolset) handleRunScript(ctx context.Context, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[runScriptArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields("body", args.Body); missing {
		return result, nil
	}
	if result, ok := requireProject(project); !ok {
		return result, nil
	}

	stdout, payload, hasPayload, failure, failed := runEngineCall(ctx, func(ctx context.Context) (string, string, int, error) {
		return t.runEngine(ctx, "run_script", map[string]any{"body": args.Body})
	}, "Failed to run script", headlessSolutions, headlessSolutions)
	if failed {
		return failure, nil //nolint:nilerr // the failure is reported through the tool result.
	}

	// The bridge reports compile and runtime failures inside the payload, so a
	// non-error result must still carry the declared shape.
	if hasPayload {
		return mcp.JSONResult(payload), nil
	}
	return mcp.ErrorResult(
		"Failed to run script: run_script returned no structured output: "+stdout,
		headlessSolutions...,
	), nil
}
