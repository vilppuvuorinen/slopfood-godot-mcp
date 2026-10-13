package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// defaultQuitAfter bounds a project run to a single main-loop iteration unless
// the caller asks for more; headless has no vsync, so even a large budget
// completes quickly and the wall-clock timeout remains the real bound.
const defaultQuitAfter = 1

func (t *toolset) runProject() mcp.Tool {
	return mcp.Tool{
		Name:  "run_project",
		Title: "Run Project",
		Description: `Boot the project's main scene in a throwaway headless engine for a bounded number of frames and return its raw stdout, stderr, and exit code.

Headless binds the dummy display and audio drivers, so this is a boot/logic smoke run: rendering, audio, and real input are unavailable. The engine quits after quitAfter main-loop iterations, or earlier if the game itself quits. Use run_script_file for assertion-style tests.`,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"quitAfter": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Main-loop iterations to run before the engine quits. Default: 1",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Arguments forwarded to the game, read with OS.get_cmdline_user_args()",
				},
			},
			"required": []string{},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"quitAfter": map[string]any{"type": "integer", "description": "The frame budget the run was given"},
			"stdout":    map[string]any{"type": "string", "description": "Everything the project wrote to stdout"},
			"stderr":    map[string]any{"type": "string", "description": "Everything the project wrote to stderr"},
			"exitCode":  map[string]any{"type": "integer", "description": exitCodeDescription},
			"timedOut":  map[string]any{"type": "boolean", "description": timedOutDescription},
		}),
		Handler: t.handleRunProject,
	}
}

type runProjectArgs struct {
	QuitAfter *int     `json:"quitAfter"`
	Args      []string `json:"args"`
}

func (t *toolset) handleRunProject(ctx context.Context, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[runProjectArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, ok := requireProject(project); !ok {
		return result, nil
	}
	quitAfter := defaultQuitAfter
	if args.QuitAfter != nil {
		quitAfter = *args.QuitAfter
	}
	if quitAfter < 1 {
		return mcp.ErrorResult(
			"quitAfter must be at least 1.",
			"Provide the number of main-loop iterations to run before the engine quits",
		), nil
	}

	ctx, cancel := context.WithTimeout(ctx, runProjectTimeout)
	defer cancel()

	stdout, stderr, exitCode, runErr := t.runHeadlessProject(ctx, quitAfter, args.Args...)
	outcome, failure := finishHeadlessRun(ctx, runErr, stdout, stderr, exitCode, "Failed to run project")
	if failure != nil {
		return *failure, nil
	}
	payload := map[string]any{
		"quitAfter": quitAfter,
		"stdout":    outcome.stdout,
		"stderr":    outcome.stderr,
		"exitCode":  outcome.exitCode,
		"timedOut":  outcome.timedOut,
	}
	return mcp.StructuredResult(headlessRunText(fmt.Sprintf("Ran the project for %d frames", quitAfter), outcome), payload), nil
}
