package godot

import "context"

// A non-zero exit is not an error: Godot's editor calls quit(EXIT_FAILURE) when the export fails.
func Export(ctx context.Context, executable, projectPath, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error) {
	exportFlag := "--export-release"
	if debug {
		exportFlag = "--export-debug"
	}
	return runEngine(ctx, executable,
		"--headless",
		"--path", projectPath,
		exportFlag,
		presetName,
		outputPath,
	)
}
