package godot

import "context"

// RunScript runs one engine process that loads scriptPath under projectPath,
// forwarding extraArgs to it. The script checker uses its extra args as the
// target list, bracketing each with a marker so callers can attribute the
// diagnostics on stderr to the script being loaded. Godot can parse
// project.godot during startup and report an error on stderr while still
// exiting 0, so the exit code and stderr are both returned and only a failure
// to start is an error.
func RunScript(ctx context.Context, executable, projectPath, scriptPath string, extraArgs ...string) (stdout, stderr string, exitCode int, err error) {
	args := make([]string, 0, 5+len(extraArgs))
	args = append(args,
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
	)
	args = append(args, extraArgs...)
	return runEngine(ctx, executable, args...)
}
