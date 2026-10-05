package godot

import "context"

// CheckScripts runs the checker over every target in one engine process. The
// checker brackets each target with a marker so the caller can attribute the
// diagnostics on stderr to the script being loaded.
func CheckScripts(ctx context.Context, executable, projectPath, checkerPath string, scriptResPaths []string) (stdout, stderr string, exitCode int, err error) {
	args := make([]string, 0, 5+len(scriptResPaths))
	args = append(args,
		"--headless",
		"--path", projectPath,
		"--script", checkerPath,
	)
	args = append(args, scriptResPaths...)
	return runEngine(ctx, executable, args...)
}

// Godot can parse project.godot during startup and report an error on stderr while still exiting 0, so
// the exit code and stderr are both returned and only a failure to start is an error.
func ValidateProject(ctx context.Context, executable, projectPath, scriptPath string) (stdout, stderr string, exitCode int, err error) {
	return runEngine(ctx, executable,
		"--headless",
		"--path", projectPath,
		"--script", scriptPath,
	)
}
