package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// runScriptTimeout bounds the body's run in the throwaway engine. A synchronous
// GDScript body runs on the engine main thread and cannot be preempted, so on
// expiry the session kills and recycles the engine. 60s clears engine boot and
// legitimately heavy scripts, while sparing an accidental runaway the
// ten-minute queue deadline. The follow-up rescan has its own budget.
const runScriptTimeout = 60 * time.Second

// runScriptRescanTimeout bounds the editor-session rescan that follows the
// throwaway non-editor run. The rescan reconciles imports and UIDs the body
// wrote but the non-editor engine could not register, so it gets its own budget
// rather than sharing the body's.
const runScriptRescanTimeout = 30 * time.Second

var runScriptTimeoutSolutions = []string{
	"The body did not finish within " + runScriptTimeout.String() + " and the engine was stopped",
	"Check for an unbounded loop or a blocking delay and return from the body instead",
	"An await in the body is not resumed because run() is invoked synchronously",
}

func (t *toolset) runScript() mcp.Tool {
	return mcp.Tool{
		Name:  "run_script",
		Title: "Run GDScript",
		Description: `Run a GDScript function body in a throwaway non-editor engine and return its output; without the editor, non-@tool scripts get real instances, and the editor session is rescanned afterward to reconcile imports and UIDs.

Scene edits follow a load, instantiate, mutate, pack, save pattern:

    var scene: PackedScene = load("res://main.tscn")
    var root: Node = scene.instantiate()
    root.get_node("Player").visible = false
    var packed := PackedScene.new()
    packed.pack(root)
    return ResourceSaver.save(packed, "res://main.tscn")

pack records only nodes owned by the packed root, so a change to a node inside an instanced sub-scene is dropped unless that instance is marked editable first:

    var foe: Node = root.get_node("Foe")  # instance of foe.tscn
    foe.get_node("Health").value = 3
    foe.get_owner().set_editable_instance(foe, true)`,
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

	// Bound an accidental runaway or blocking body. Cancellation propagates to
	// Session.Run, which kills and recycles the engine, so the fault costs this
	// budget instead of the ten-minute queue deadline.
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, runScriptTimeout)
	defer cancel()

	// The body always runs in a throwaway non-editor engine: without --editor,
	// nodes loaded from scenes get real script instances, so member state and
	// method calls work instead of hitting an editor placeholder.
	res, failure := runEngineCall(ctx, func(ctx context.Context) (godot.RunResult, error) {
		return t.runIsolated(ctx, "run_script", map[string]any{"body": args.Body})
	}, "Failed to run script", headlessSolutions, headlessSolutions)
	if failure != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return mcp.ErrorResult(
				fmt.Sprintf("Failed to run script: the body did not finish within %s and the engine was stopped", runScriptTimeout),
				runScriptTimeoutSolutions...,
			), nil
		}
		return *failure, nil //nolint:nilerr // the failure is reported through the tool result.
	}

	// The non-editor engine's writes are unreconciled: no EditorFileSystem
	// registered their imports or UIDs. Settle the editor session with a scan
	// before reporting success.
	if failure := t.rescanAfterIsolated(parent); failure != nil {
		return *failure, nil
	}

	// The bridge reports compile and runtime failures inside the payload, so a
	// non-error result must still carry the declared shape.
	if result, ok := enginePayloadResult(res); ok {
		return result, nil
	}
	return mcp.ErrorResult(
		"Failed to run script: run_script returned no structured output: "+res.Stdout,
		headlessSolutions...,
	), nil
}

// rescanAfterIsolated settles the editor session after a throwaway non-editor
// run so its imports and UIDs catch up with what the body wrote. It uses a
// context derived from the caller's (not the body's short deadline) so a body
// that consumed its whole budget can still be reconciled.
func (t *toolset) rescanAfterIsolated(parent context.Context) *mcp.ToolResult {
	rescanCtx, cancel := context.WithTimeout(parent, runScriptRescanTimeout)
	defer cancel()
	res, err := t.runEngine(rescanCtx, "rescan_project", nil)
	if err != nil {
		failure := mcp.ErrorResult("Failed to run script: the run completed but the editor session could not be rescanned: "+err.Error(), headlessSolutions...)
		return &failure
	}
	if scriptFailed(res.ExitCode, res.Stderr) {
		failure := mcp.ErrorResult("Failed to run script: the run completed but the editor session could not be rescanned: "+strings.TrimSpace(res.Stderr), headlessSolutions...)
		return &failure
	}
	return nil
}
