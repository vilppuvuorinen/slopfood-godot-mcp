package tools

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func (t *toolset) runScriptFile() mcp.Tool {
	return mcp.Tool{
		Name:  "run_script_file",
		Title: "Run GDScript File",
		Description: `Run a project GDScript file as a one-shot --script entrypoint in a throwaway headless engine and return its raw stdout, stderr, and exit code.

Unlike run_script, which compiles an in-memory body against the bridge, this hands the file to Godot's --script, so extends SceneTree/MainLoop entrypoints, _init/_initialize, and command-line arguments work. The engine is non-editor and discarded after the run; the editor session is not touched or rescanned, so this is a test/check runner rather than an edit path.`,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type":        "string",
					"description": "Project-relative path or res:// path of the GDScript file to run",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Arguments forwarded to the script, read with OS.get_cmdline_user_args()",
				},
			},
			"required": []string{"script"},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"script":   map[string]any{"type": "string", "description": "res:// path of the script that ran"},
			"stdout":   map[string]any{"type": "string", "description": "Everything the script wrote to stdout"},
			"stderr":   map[string]any{"type": "string", "description": "Everything the script wrote to stderr"},
			"exitCode": map[string]any{"type": "integer", "description": exitCodeDescription},
			"timedOut": map[string]any{"type": "boolean", "description": timedOutDescription},
		}),
		Handler: t.handleRunScriptFile,
	}
}

type runScriptFileArgs struct {
	Script string   `json:"script"`
	Args   []string `json:"args"`
}

func (t *toolset) handleRunScriptFile(ctx context.Context, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[runScriptFileArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields("script", args.Script); missing {
		return result, nil
	}
	if result, ok := requireProject(project); !ok {
		return result, nil
	}
	hostPath, err := confinePath(project, args.Script)
	if err != nil {
		return invalidPathResultFor(err), nil
	}
	if !pathExists(hostPath) {
		return scriptFileMissingResult(args.Script), nil
	}
	// Godot's --script resolves res:// against the project root; derive it from
	// the validated relative path rather than echoing the caller's form.
	resPath := resScheme + filepath.ToSlash(filepath.Clean(projectRelativePath(args.Script)))

	ctx, cancel := context.WithTimeout(ctx, runScriptFileTimeout)
	defer cancel()

	stdout, stderr, exitCode, runErr := t.runHeadless(ctx, resPath, userArgs(args.Args)...)
	outcome, failure := finishHeadlessRun(ctx, runErr, stdout, stderr, exitCode, "Failed to run script file")
	if failure != nil {
		return *failure, nil
	}
	payload := map[string]any{
		"script":   resPath,
		"stdout":   outcome.stdout,
		"stderr":   outcome.stderr,
		"exitCode": outcome.exitCode,
		"timedOut": outcome.timedOut,
	}
	return mcp.StructuredResult(headlessRunText("Ran "+resPath, outcome), payload), nil
}

func scriptFileMissingResult(scriptPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Script file does not exist: "+scriptPath,
		"Ensure the script path is correct and inside the project root",
		"Create the script first, for example through run_script",
	)
}
